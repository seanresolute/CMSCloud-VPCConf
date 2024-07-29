#!/usr/bin/env python3

"""
Automatically makes a copy (which will not be automatically
deleted) of the latest automated snapshot (which will be)
for the RDS instance identified as 'postgres'.

Then applies a retention policy to the previously copied snaphots.

IAM Policy for role this will run under:
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "",
            "Effect": "Allow",
            "Action": [
                "rds:CopyDBSnapshot",
                "rds:DeleteDBSnapshot",
                "rds:DescribeDBSnapshots"
            ],
            "Resource": "*"
        }
    ]
}
"""

import collections
import datetime
import os
import re
import sys
import time
import traceback

import boto3

NOW = datetime.datetime.now(datetime.timezone.utc)
# (X, Y) means "snaphsots <= Y days old should have gaps
# of <= X days between them"
BACKUP_FREQUENCIES = [
    (7, 1),
    (30, 7),
    (365, 30)
]
DEFAULT_BACKUP_FREQUENCY = 90

def error(msg):
    raise Exception(msg)

class InvalidTimestampException(Exception):
    pass

def backup_frequency(d):
    age = NOW - d
    if age < datetime.timedelta(0):
        raise InvalidTimestampException('Invalid snapshot time %s' % d)
    for age_days, freq_days in BACKUP_FREQUENCIES:
        if age <= datetime.timedelta(days=age_days):
            return datetime.timedelta(days=freq_days)
    return datetime.timedelta(days=DEFAULT_BACKUP_FREQUENCY)

def retain_snapshot_ids(snapshot_ids, fmt):
    snapshot_ids = sorted(snapshot_ids, reverse=True)
    Snapshot = collections.namedtuple('Snapshot', ['id', 'date'])
    snapshots = [
        Snapshot(id, datetime.datetime(*time.strptime(id, fmt)[0:6], tzinfo=datetime.timezone.utc))
        for id in snapshot_ids
    ]
    last_date = None
    retained_ids = []
    for idx, snapshot in enumerate(snapshots):
        max_age = backup_frequency(snapshot.date)
        if (
                # first backup
                last_date is None
                # last backup
                or idx == len(snapshots) - 1
                # next backup would be too old
                or last_date - snapshots[idx+1].date > max_age):
            last_date = snapshot.date
            retained_ids.append(snapshot.id)
    return retained_ids

def lambda_handler(event, context):
    region_name = os.environ['region']
    db_identifier = os.environ['db']

    # STEP 1: make a permanent copy of the most recent snapshot
    rds = boto3.client('rds', region_name=region_name)
    snapshots = rds.describe_db_snapshots(
        DBInstanceIdentifier=db_identifier, SnapshotType='automated'
    )['DBSnapshots']
    if not snapshots:
        error('No automated snapshots found')

    latest_snapshot = sorted(
        snapshots, key=lambda s: s['SnapshotCreateTime'], reverse=True)[0]

    status = latest_snapshot['Status']
    if status != 'available':
        error('Latest snapshot status was "%s"' % status)

    created_at = latest_snapshot['SnapshotCreateTime']
    diff = NOW - created_at

    if diff.total_seconds() < 0:
        error('Latest snapshot created after current time: %s vs %s' % (
            created_at, NOW))

    if diff.total_seconds() / 60 / 60 > 24:
        error('Latest snapshot created more than 24 hours ago: %s vs %s' % (
            created_at, NOW))

    identifier = latest_snapshot['DBSnapshotIdentifier']
    fmt = '%s-archived-%%Y-%%m-%%d' % db_identifier
    new_identifier = latest_snapshot['SnapshotCreateTime'].strftime(fmt)

    snapshots = rds.describe_db_snapshots(
        DBInstanceIdentifier=db_identifier, SnapshotType='manual'
    )['DBSnapshots']
    if any(s['DBSnapshotIdentifier'] == new_identifier for s in snapshots):
        print('There is already a snapshot called %s' % new_identifier)
    else:
        print('Copying %s to %s' % (identifier, new_identifier))

        rds.copy_db_snapshot(
            SourceDBSnapshotIdentifier=identifier,
            TargetDBSnapshotIdentifier=new_identifier)

    # STEP 2: enforce retention policy against all archived copies
    pattern = re.compile(r'%s-archived-\d{4}-\d{2}-\d{2}$' % re.escape(db_identifier))
    archive_snapshot_ids = [
        s['DBSnapshotIdentifier'] for s in snapshots
        if pattern.match(s['DBSnapshotIdentifier'])
    ]

    keep_ids = retain_snapshot_ids(archive_snapshot_ids, fmt)
    
    for id in sorted(set(archive_snapshot_ids) - set(keep_ids)):
        print('Deleting archived snapshot %s' % id)
        rds.delete_db_snapshot(DBSnapshotIdentifier=id)

if __name__ == '__main__':
    # for testing
    os.environ['region'] = sys.argv[1]
    os.environ['db'] = sys.argv[2]
    lambda_handler(None, None)
