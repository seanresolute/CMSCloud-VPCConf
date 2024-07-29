import groovy.json.JsonOutput
import groovy.json.JsonSlurper

@Library('corbalt') _

def appToEnvToConfig = [
    'vpc-conf': [
        dev: [
            region: 'us-east-1',
            role: 'arn:aws:iam::346570397073:role/vpc-conf-dev-cbc-jenkins',
        ],
        prod: [
            region: 'us-east-1',
            role: 'arn:aws:iam::546085968493:role/vpc-conf-prod-cbc-jenkins',
        ],
    ],
    'creds-api': [
        dev: [
            region: 'us-east-1',
            role: 'arn:aws:iam::346570397073:role/cbc-jenkins',
        ],
        prod: [
            region: 'us-east-1',
            role: 'arn:aws:iam::546085968493:role/cbc-jenkins',
        ],
    ],
    'update-aws-accounts': [
        dev: [
            region: 'us-east-1',
            role: 'arn:aws:iam::346570397073:role/cbc-jenkins',
        ],
        prod: [
            region: 'us-east-1',
            role: 'arn:aws:iam::546085968493:role/cbc-jenkins',
        ],
    ],
]

stage ('Build') {
    def appToTask = appToEnvToConfig.collectEntries { app, envToConfig ->
        [(app): {
            corbalt.inCMSCloudBuildPod {
                def (defaultRegistry, dockerConfig) = corbalt.getDockerConfigForDockerHub()
                def ecrHosts = []

                checkout scm
                def sha = sh(returnStdout: true, script: 'git rev-parse --short=8 HEAD').trim()

                envToConfig.each { e, config ->
                    if (e == 'prod' && env.BRANCH_NAME != 'master') {
                        println 'Will not push to prod because this is not a merge to master'
                        return
                    }
                    println "Getting ${e} creds"
                    withEnv(corbalt.getIamRoleEnv(config.role)) {
                        def (envEcrHost, envConfig) = corbalt.getDockerConfigForEcr(config.region)
                        ecrHosts += envEcrHost
                        dockerConfig.auths << envConfig.auths
                    }
                }

                def targets = ecrHosts.collect { "${it}/${app}:${sha}" }
                corbalt.kanikoBuild(targets, dockerConfig, context: './vpc-automation', dockerfile: "./vpc-automation/${app}.Dockerfile")

                withEnv(corbalt.getIamRoleEnv('arn:aws:iam::546085968493:role/cbc-jenkins')) {
                    container('aws') {
                        sh 'aws --region=us-east-1 s3 cp s3://artifact-db-546085968493-us-east-1/latest ./artifact-db'
                        sh 'chmod +x ./artifact-db'
                        def tags = JsonOutput.toJson([
                            SHA: sha,
                            ForAutomatedDeploy: sprintf("%s", env.BRANCH_NAME == 'master'),
                        ])
                        sh "./artifact-db --region us-east-1 create -p ${app} -t '${tags}' -v ${app}:${sha}"
                    }
                }
            }
        }]
    }
    parallel(appToTask)
}
