@Library('corbalt') _

def podParams = corbalt.getPodParams([aws: 'golang:1.13-alpine'])
podParams.serviceAccount = 'itops-ia'
corbalt.inPod(podParams) {
    stage ('Build') {
        checkout scm
        def sha = sh(returnStdout: true, script: 'git rev-parse --short=8 HEAD').trim()

        // Name container "aws" so that getIamRoleEnv can find it
        container('aws') {
            sh 'apk add aws-cli'

            sh 'CGO_ENABLED=0 GOBIN=/ go install'

            withEnv(corbalt.getIamRoleEnv('arn:aws-us-gov:iam::350521122370:role/delegatedadmin/developer/cbc-jenkins')) {
                sh "aws --region=us-gov-west-1 s3 cp /artifact-db s3://vpc-automation-artifact-db/${sha} --sse"
                sh 'aws --region=us-gov-west-1 s3 cp /artifact-db s3://vpc-automation-artifact-db/latest --sse'
            }
        }
    }
}
