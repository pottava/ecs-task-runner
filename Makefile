.PHONY: all help

all: help

help:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			golang:1.10-alpine3.8 \
			go run cmd/ecs-task-runner/main.go --help

dep-init:
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner \
			supinf/go-dep:0.5 init

dep-ensure:
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner \
			supinf/go-dep:0.5 ensure

build:
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner \
			pottava/gox:go1.9 --osarch "linux/amd64 darwin/amd64 windows/amd64" \
			-ldflags "-s -w" -output "dist/{{.OS}}_{{.Arch}}"
