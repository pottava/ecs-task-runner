.PHONY: all help test build

all: build

help:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			golang:1.10-alpine3.8 \
			go run cmd/ecs-task-runner/main.go --help

deps:
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner \
			supinf/go-dep:0.5 ensure

test:
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner \
			supinf/gometalinter:2.0.5 \
			--config=lint-config.json ./...
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner \
			golang:1.10-alpine3.8 \
			go test -cover -bench -benchmem `go list ./...`

build:
	@docker run --rm -it -v "${GOPATH}"/src:/go/src \
			-w /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner \
			pottava/gox:go1.9 --osarch "linux/amd64 darwin/amd64 windows/amd64" \
			-ldflags "-s -w" -output "dist/{{.OS}}_{{.Arch}}"
