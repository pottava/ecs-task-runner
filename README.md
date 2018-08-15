# A synchronous task runner for AWS Fargate on Amazon ECS

[![CircleCI](https://circleci.com/gh/pottava/ecs-task-runner.svg?style=svg)](https://circleci.com/gh/pottava/ecs-task-runner)

[![pottava/ecs-task-runner](http://dockeri.co/image/pottava/ecs-task-runner)](https://hub.docker.com/r/pottava/ecs-task-runner/)

Supported tags and respective `Dockerfile` links:  
・latest ([Dockerfile](https://github.com/pottava/ecs-task-runner/blob/master/Dockerfile))  
・1 ([Dockerfile](https://github.com/pottava/ecs-task-runner/blob/master/Dockerfile))  


## Description

This is a synchronous task runner for AWS Fargate. It runs a docker container on Fargate and waits for its done. Then it returns its standard output logs from CloudWatch Logs. All resources we need are created temporarily and remove them after the task finished.


## Installation

curl:

```
$ curl -Lo ecs-task-runner https://github.com/pottava/ecs-task-runner/releases/download/1.1/ecs-task-runner_darwin_amd64 && chmod +x ecs-task-runner
```

go:

```
$ go get github.com/pottava/ecs-task-runner/...
```

docker:

```
$ docker pull pottava/ecs-task-runner:1
```


## Parameters

Environment Variables     | Argument        | Description                     | Required | Default 
------------------------- | --------------- | ------------------------------- | -------- | ---------
DOCKER_IMAGE              | image, i        | Docker image to be run on ECS   | *        |
FORCE_ECR                 | force_ecr, f    | True: you can use shortened name |         | false
ENTRYPOINT                | entrypoint      | Override `ENTRYPOINT` of the image |       |
COMMAND                   | command         | Override `CMD` of the image     |          |
AWS_ACCESS_KEY_ID         | access_key, a   | AWS `access key` for API access | *        |
AWS_SECRET_ACCESS_KEY     | secret_key, s   | AWS `secret key` for API access | *        |
AWS_DEFAULT_REGION        | region, r       | AWS `region` for API access     |          | us-east-1
ECS_CLUSTER               | cluster, c      | Amazon ECS cluster name         |          | 
SUBNETS                   | subnets         | Fargate's Subnets               |          |
SECURITY_GROUPS           | security_groups | Fargate's SecurityGroups        |          |
TASK_ROLE                 | role            | ARN of an IAM Role for the task |          |
CPU                       | cpu             | Requested vCPU to run Fargate   |          | 256
MEMORY                    | memory          | Requested memory to run Fargate |          | 512
NUMBER                    | number, n       | Number of tasks                 |          | 1 
TASK_TIMEOUT              | timeout, t      | Timeout minutes for the task    |          | 30


## Samples

With arguments:

```console
$ ecs-task-runner -a AKIAIOSFODNN7EXAMPLE -s wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY run -i sample/image
{
  "container-1": [
    "2018-08-15T12:01:26+09:00: Hello world!",
    "2018-08-15T12:05:01+09:00: message 1",
    "2018-08-15T12:07:01+09:00: message 2"
  ]
}
```

With environment variables:

```console
$ export DOCKER_IMAGE=sample/image
$ export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
$ export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
$ ecs-task-runner run
```

With docker container:

```console
$ export DOCKER_IMAGE=sample/image
$ export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
$ export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
$ docker run --rm -e DOCKER_IMAGE -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY pottava/ecs-task-runner:1 run
```
