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

            withEnv(corbalt.getIamRoleEnv('arn:aws:iam::546085968493:role/cbc-jenkins')) {
                sh "aws --region=us-east-1 s3 cp /artifact-db s3://artifact-db-546085968493-us-east-1/${sha} --sse"
                sh 'aws --region=us-east-1 s3 cp /artifact-db s3://artifact-db-546085968493-us-east-1/latest --sse'
            }
        }
    }
}
