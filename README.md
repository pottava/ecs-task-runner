# A synchronous task runner for AWS Fargate on Amazon ECS

[![CircleCI](https://circleci.com/gh/pottava/ecs-task-runner.svg?style=svg)](https://circleci.com/gh/pottava/ecs-task-runner)

[![pottava/ecs-task-runner](http://dockeri.co/image/pottava/ecs-task-runner)](https://hub.docker.com/r/pottava/ecs-task-runner/)

Supported tags and respective `Dockerfile` links:  
・latest ([versions/2.3/Dockerfile](https://github.com/pottava/ecs-task-runner/blob/master/versions/2.3/Dockerfile))  
・2.3 ([versions/2.3/Dockerfile](https://github.com/pottava/ecs-task-runner/blob/master/versions/2.3/Dockerfile))  
・1 ([versions/1.2/Dockerfile](https://github.com/pottava/ecs-task-runner/blob/master/versions/1.2/Dockerfile))  

## Description

This is a synchronous task runner for AWS Fargate. It runs a docker container on Fargate and waits for its done. Then it returns its standard output logs from CloudWatch Logs. All resources we need are created temporarily and remove them after the task finished.

## Installation

curl (macOS):

```sh
$ curl -Lo ecs-task-runner https://github.com/pottava/ecs-task-runner/releases/download/2.3/ecs-task-runner_darwin_amd64 \
    && chmod +x ecs-task-runner
```

curl (Linux):

```sh
$ curl -Lo ecs-task-runner https://github.com/pottava/ecs-task-runner/releases/download/2.3/ecs-task-runner_linux_amd64 \
    && chmod +x ecs-task-runner
```

go:

```console
go get github.com/pottava/ecs-task-runner/...
```

docker:

```console
docker pull pottava/ecs-task-runner
```

## Parameters

Common parameters:

Environment Variables     | Argument        | Description                     | Required | Default
------------------------- | --------------- | ------------------------------- | -------- | ---------
AWS_ACCESS_KEY_ID         | access-key, a   | AWS `access key` for API access | *        |
AWS_SECRET_ACCESS_KEY     | secret-key, s   | AWS `secret key` for API access | *        |
AWS_DEFAULT_REGION        | region, r       | AWS `region` for API access     |          | us-east-1
ECS_CLUSTER               | cluster, c      | Amazon ECS cluster name         |          |
EXEC_ROLE_NAME            | exec-role-name  | Name of an execution role       |          | ecs-task-runner
TASK_TIMEOUT              | timeout, t      | Timeout minutes for the task    |          | 30
EXTENDED_OUTPUT           | extended-output | True: meta data also returns    |          | false

for the `run` command:

Environment Variables     | Argument        | Description                     | Required | Default
------------------------- | --------------- | ------------------------------- | -------- | ---------
DOCKER_IMAGE              |                 | Docker image to be run on ECS   | *        |
FARGATE_SPOT              | spot            | True: fargate spot will be used |          | false
FORCE_ECR                 | force-ecr, f    | True: you can use shortened name |         | false
ENTRYPOINT                | entrypoint      | Override `ENTRYPOINT` of the image |       |
COMMAND                   | command         | Override `CMD` of the image     |          |
PORT                      | port, p         | Publish ports                   |          |
ENVIRONMENT               | environment, e  | Add `ENV` to the container      |          |
USER                      | docker-user     | The user inside the container   |          |
LABEL                     | label, l        | Add `LABEL` to the container    |          |
SUBNETS                   | subnets         | Fargate's Subnets               |          |
SECURITY_GROUPS           | security-groups | Fargate's SecurityGroups        |          |
TASKDEF_FAMILY            | taskdef-family  | ECS Task Definition family name |          | ecs-task-runner
TASK_ROLE                 | task-role-arn   | ARN of an IAM Role for the task |          |
CPU                       | cpu             | Requested vCPU to run Fargate   |          | 256
MEMORY                    | memory          | Requested memory to run Fargate |          | 512
NUMBER                    | number, n       | Number of tasks                 |          | 1
PRIVATE_REGISTRY_USER     | user            | PrivateRegistry Username        |          |
PRIVATE_REGISTRY_PASSWORD | password        | PrivateRegistry Password        |          |
KMS_CUSTOMKEY_ID          | kms-key-id      | KMS custom key ID for SecretsManager |     |
ASSIGN_PUBLIC_IP          | assign-pub-ip   | True: Assigns public IP         |          | true
READONLY_ROOOTFS          | readonly-rootfs | Make the root file system read-only |      | false
ASYNC                     | async           | True: Does not wait for the job done |     | false

for the `stop` command:

Environment Variables     | Argument        | Description                     | Required | Default
------------------------- | --------------- | ------------------------------- | -------- | ---------
REQUEST_ID                |                 | Resources ID to be stopped      | *        |
TASK_ARN                  | task-arn        | Task ARNs to be stopped         |          |

## Samples

```console
$ export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
$ export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
$ ecs-task-runner run alpine --entrypoint env
{
  "container-1": [
    "2018-09-23T11:42:01+09:00: PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
    "2018-09-23T11:42:01+09:00: HOSTNAME=ip-172-31-40-206.us-east-1.compute.internal",
    "2018-09-23T11:42:01+09:00: AWS_DEFAULT_REGION=ap-northeast-1",
    "2018-09-23T11:42:01+09:00: AWS_REGION=ap-northeast-1",
    "2018-09-23T11:42:01+09:00: HOME=/root"
  ]
}
$ echo $?
0
```

This app will return with an exit code of the containers:

```console
$ ecs-task-runner run alpine --entrypoint sh,-c --command "exit 255"
{
  "container-1": []
}
$ echo $?
255
```

Run a container asynchronously with `--async` flag:

```console
$ ecs-task-runner run nginx --async -p 80 --security-groups public-80
{
  "RequestID": "ecs-task-runner-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "Tasks": [
    {
      "PublicIP": "xx.xxx.xxx.xx",
      "TaskARN": "arn:aws:ecs:us-east-1:xxxxxxxxxxxx:task/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    }
  ]
}
```

To stop the asynchronous tasks:

```console
$ ecs-task-runner stop ecs-task-runner-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
{
  "container-1": [
    "2018-09-23T22:34:37+09:00: zzz.zz.z.zzz - - [23/Sep/2018:13:34:37 +0000] \"GET / HTTP/1.1\" 200 612 \"-\" \"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/52.0 Safari/537.36\" \"-\""
  ]
}
```

## Usage

With arguments:

```console
ecs-task-runner -a AKIAIOSFODNN7EXAMPLE -s wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY run sample/image
```

With environment variables:

```console
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
ecs-task-runner run sample/image
```

With ECR shortened image name:

```console
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
ecs-task-runner run --force-ecr my-ecr/image
```

With a private registory:

```console
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
export PRIVATE_REGISTRY_USER=user
export PRIVATE_REGISTRY_PASSWORD=password
ecs-task-runner run sample/secret
```

With the docker container:

```console
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
docker run --rm -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY pottava/ecs-task-runner sample/image
```

## Troubleshooting

If the command returns non-zero exit code, you can try `--extended-output` to analyze the cause of failure.

```console
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
ecs-task-runner run sample/image --extended-output
```
