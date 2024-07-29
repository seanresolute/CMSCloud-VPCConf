import boto3
import collections
import datetime
import http.client
import json
import os
import sys


def lambda_handler(event, context):
    cluster = os.environ['cluster']
    service = os.environ['service']
    metric_namespace = os.environ['metric_namespace']
    ecs_client = boto3.client('ecs', region_name=os.environ['region'])
    now = datetime.datetime.now(datetime.timezone.utc)

    # Determine container port
    services = ecs_client.describe_services(
        cluster=os.environ['cluster'],
        services=[os.environ['service']]
    )['services']
    if len(services) != 1:
        raise Exception("Got %s matching services" % len(services))
    if len(services[0]['loadBalancers']) != 1:
        raise Exception("Got %s load balancers" % len(services[0]['loadBalancers']))
    containerPort = services[0]['loadBalancers'][0]['containerPort']

    # Get tasks
    taskArns = ecs_client.list_tasks(
        cluster=cluster,
        serviceName=service,
        desiredStatus='RUNNING'
    )['taskArns']
    tasks = ecs_client.describe_tasks(
        cluster=cluster,
        tasks=taskArns
    )['tasks']

    # Check health on each task
    connect_counts = collections.defaultdict(int)
    oldest_age = 0
    num_queued = 0
    num_in_progress = 0
    num_healthy = 0
    jira_num_errors = 0
    jira_num_retries = 0
    jira_oldest_age = 0

    for task in tasks:
        if task['lastStatus'] == 'RUNNING':
            containers = task['containers']
            if len(containers) != 1:
                raise Exception("Got %s containers in task %s" % (len(containers), task['taskArn']))
            container = containers[0]
            if len(container['networkInterfaces']) != 1:
                raise Exception("Got %s network interfaces in task %s" % (
                    len(container['networkInterfaces']), task['taskArn']))
            ip = container['networkInterfaces'][0]['privateIpv4Address']
            try:
                conn = http.client.HTTPConnection(ip, containerPort, timeout=30)
                conn.request('GET', '/health')
                health = json.load(conn.getresponse())
            except Exception as e:
                print('ERROR connecting to %s:%s: %s' % (ip, containerPort, e))
                continue
            num_healthy += 1
            for k, v in health.get('CanConnect', {}).items():
                if v:
                    connect_counts[k] += 1
            if health.get('TaskStats'):
                task_stats = health['TaskStats']
                if task_stats.get('OldestNotDoneAddedAt'):
                    d = datetime.datetime.strptime(task_stats['OldestNotDoneAddedAt'], '%Y-%m-%dT%H:%M:%S.%fZ')
                    d = d.replace(tzinfo=datetime.timezone.utc)
                    oldest_age = (now - d).total_seconds()
                num_queued = task_stats.get('NumQueued', 0)
                num_in_progress = task_stats.get('NumInProgress', 0)
            if health.get('JiraIssueErrors'):
                jira = health['JiraIssueErrors']
                jira_num_errors = jira.get('NumErrors')
                if jira_num_errors > 0:
                    d = datetime.datetime.strptime(jira['OldestAddedAt'], '%Y-%m-%dT%H:%M:%S.%fZ')
                    d = d.replace(tzinfo=datetime.timezone.utc)
                    jira_oldest_age = (now - d).total_seconds()
                    jira_num_retries = jira.get('OldestNumRetries')

    cw_client = boto3.client('cloudwatch', region_name=os.environ['region'])
    metric_data = [
        {
            'MetricName': 'OldestTaskAge',
            'Timestamp': now,
            'Value': oldest_age,
        },
        {
            'MetricName': 'NumUnfinishedTasks',
            'Timestamp': now,
            'Value': num_queued + num_in_progress,
        },
        {
            'MetricName': 'Backends.Healthy',
            'Timestamp': now,
            'Value': num_healthy,
        },
        {
            'MetricName': 'JiraOldestAge',
            'Timestamp': now,
            'Value': jira_oldest_age,
        },
        {
            'MetricName': 'JiraNumRetries',
            'Timestamp': now,
            'Value': jira_num_retries,
        },
        {
            'MetricName': 'JiraNumErrors',
            'Timestamp': now,
            'Value': jira_num_errors,
        },
    ]
    for k, v in connect_counts.items():
        metric_data.append(
            {
                'MetricName': 'Backends.Connect.' + k,
                'Timestamp': now,
                'Value': v,
            })
    cw_client.put_metric_data(
        Namespace=metric_namespace,
        MetricData=metric_data
    )


if __name__ == "__main__":
    # for testing
    os.environ['region'] = 'us-west-2'
    os.environ['cluster'] = sys.argv[1]
    os.environ['service'] = sys.argv[2]
    os.environ['metric_namespace'] = sys.argv[3]
    lambda_handler(None, None)
