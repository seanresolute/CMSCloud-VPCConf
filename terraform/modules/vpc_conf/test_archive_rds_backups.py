import collections
import datetime
import random
import unittest

import archive_rds_backups

archive_rds_backups.NOW = datetime.datetime(2019, 8, 16, 16, 2, 33, tzinfo=datetime.timezone.utc)
archive_rds_backups.BACKUP_FREQUENCIES = [
    (7, 1),
    (30, 7),
    (365, 30)
]
archive_rds_backups.DEFAULT_BACKUP_FREQUENCY = 90

RetainTestCase = collections.namedtuple('RetainTestCase', ['ids', 'retained_ids'])
TEST_CASES = [
    RetainTestCase(
        [
            'xyz-2019-08-16',
            'xyz-2019-08-15',
            'xyz-2019-08-14',
            'xyz-2019-08-13',
            'xyz-2019-08-12',
            'xyz-2019-08-11',
            'xyz-2019-08-10',
            'xyz-2019-08-09',
            'xyz-2019-08-08',
            'xyz-2019-08-07',
            'xyz-2019-08-06',
            'xyz-2019-08-05',
            'xyz-2019-08-04',
            'xyz-2019-08-03',
            'xyz-2019-08-02',
            'xyz-2019-08-01',
            'xyz-2019-07-15',
            'xyz-2019-07-01',
            'xyz-2019-06-15',
            'xyz-2019-06-01',
            'xyz-2019-05-15',
            'xyz-2019-05-01',
            'xyz-2019-04-15',
            'xyz-2019-04-01',
            'xyz-2018-12-01',
            'xyz-2018-10-01',
            'xyz-2018-08-01',
            'xyz-2018-06-01',
            'xyz-2018-04-01',
            'xyz-2018-02-01',
            'xyz-2018-01-01',
            'xyz-2017-12-01',
            'xyz-2017-11-01',
            'xyz-2017-10-01',
        ],
        [
            'xyz-2019-08-16',
            'xyz-2019-08-15',
            'xyz-2019-08-14',
            'xyz-2019-08-13',
            'xyz-2019-08-12',
            'xyz-2019-08-11',
            'xyz-2019-08-10',
            'xyz-2019-08-03',
            'xyz-2019-08-01',
            'xyz-2019-07-15',
            'xyz-2019-06-15',
            'xyz-2019-06-01',
            'xyz-2019-05-15',
            'xyz-2019-04-15',
            'xyz-2019-04-01',
            'xyz-2018-12-01',
            'xyz-2018-10-01',
            'xyz-2018-08-01',
            'xyz-2018-06-01',
            'xyz-2018-04-01',
            'xyz-2018-01-01',
            'xyz-2017-11-01',
            'xyz-2017-10-01',
        ]
    ),
    RetainTestCase(
        [
            'xyz-2019-08-16',
            'xyz-2019-08-15',
            'xyz-2019-08-14',
            'xyz-2019-08-13',
            'xyz-2019-08-12',
            'xyz-2019-08-11',
            'xyz-2019-08-10',
            'xyz-2019-08-09',
            'xyz-2019-08-08',
            'xyz-2019-08-07',
            'xyz-2019-08-06',
            'xyz-2019-08-05',
            'xyz-2019-08-04',
            'xyz-2019-08-03',
            'xyz-2019-08-02',
            'xyz-2019-08-01',
            'xyz-2019-07-31',
            'xyz-2019-07-30',
            'xyz-2019-07-29',
            'xyz-2019-07-28',
            'xyz-2019-07-27',
            'xyz-2019-07-26',
            'xyz-2019-07-25',
            'xyz-2019-07-24',
            'xyz-2019-07-23',
            'xyz-2019-07-22',
            'xyz-2019-07-21',
            'xyz-2019-07-20',
            'xyz-2019-07-19',
            'xyz-2019-07-18',
            'xyz-2019-07-17',
            'xyz-2019-07-16',
            'xyz-2019-07-15',
            'xyz-2019-07-14',
            'xyz-2019-07-13',
            'xyz-2019-07-12',
            'xyz-2019-07-11',
            'xyz-2019-07-10',
            'xyz-2019-07-09',
            'xyz-2019-07-08',
            'xyz-2019-07-07',
            'xyz-2019-07-06',
            'xyz-2019-07-05',
            'xyz-2019-07-04',
            'xyz-2019-07-03',
            'xyz-2019-07-02',
            'xyz-2019-07-01',
            'xyz-2019-06-30',
            'xyz-2019-06-29',
            'xyz-2019-06-28',
            'xyz-2019-06-27',
            'xyz-2019-06-26',
            'xyz-2019-06-25',
            'xyz-2019-06-24',
            'xyz-2019-06-23',
            'xyz-2019-06-22',
            'xyz-2019-06-21',
            'xyz-2019-06-20',
            'xyz-2019-06-19',
            'xyz-2019-06-18',
            'xyz-2019-06-17',
            'xyz-2019-06-16',
            'xyz-2019-06-15',
        ],
        [
            'xyz-2019-08-16',
            'xyz-2019-08-15',
            'xyz-2019-08-14',
            'xyz-2019-08-13',
            'xyz-2019-08-12',
            'xyz-2019-08-11',
            'xyz-2019-08-10',
            'xyz-2019-08-03',
            'xyz-2019-07-27',
            'xyz-2019-07-20',
            'xyz-2019-06-20',
            'xyz-2019-06-15',
        ]
    ),
    RetainTestCase(
        [
            'xyz-2019-08-16',
            'xyz-2018-08-15',
            'xyz-2017-08-15',
            'xyz-2016-08-15',
            'xyz-2010-08-15',
            'xyz-2009-08-15',
        ],
        [
            'xyz-2019-08-16',
            'xyz-2018-08-15',
            'xyz-2017-08-15',
            'xyz-2016-08-15',
            'xyz-2010-08-15',
            'xyz-2009-08-15',
        ]
    ),
]
FMT = 'xyz-%Y-%m-%d'

class TestBackupRetention(unittest.TestCase):
    def test_retain_snapshot_ids(self):
        random.seed(123)
        for tc in TEST_CASES:
            random.shuffle(tc.ids)
            retained_ids = archive_rds_backups.retain_snapshot_ids(tc.ids, FMT)
            self.assertEqual(
                tc.retained_ids,
                retained_ids,
                'Retained IDs do not match'
            )
            shuffled_ids = list(retained_ids)
            random.shuffle(shuffled_ids)
            self.assertEqual(
                retained_ids,
                archive_rds_backups.retain_snapshot_ids(shuffled_ids, FMT),
                'retain_snapshot_ids not idempotent'
            )

if __name__ == '__main__':
    unittest.main()
