import boto3
import datetime
import os
import sys

def lambda_handler(event, context):
    redeploy_seconds = int(os.environ['redeploy_seconds'])
    client = boto3.client('ecs', region_name=os.environ['region'])
    services = client.describe_services(
        cluster=os.environ['cluster'],
        services=[os.environ['service']]
    )['services']
    if len(services) != 1:
        raise Exception("Got %s matching services" % len(services))
    service = services[0]
    deployment_dates = [
        d['createdAt'] for d in service['deployments'] if d['status'] == 'PRIMARY'
    ]
    if len(deployment_dates) != 1:
        raise Exception("%s primary deployments" % len(deployment_dates))
    primary_deployment_date = deployment_dates[0]
    since_deploy = datetime.datetime.now(datetime.timezone.utc) - primary_deployment_date
    print('%s since primary deploy' % since_deploy)
    if since_deploy.total_seconds() > redeploy_seconds:
        print('Redeploying')
        client.update_service(
            cluster=os.environ['cluster'],
            service=os.environ['service'],
            forceNewDeployment=True
        )
    else:
        print('Not redeploying')

if __name__ == "__main__":
    # for testing
    os.environ['region'] = 'us-west-2'
    os.environ['cluster'] = sys.argv[1]
    os.environ['service'] = sys.argv[2]
    os.environ['redeploy_seconds'] = sys.argv[3]
    lambda_handler(None, None)
