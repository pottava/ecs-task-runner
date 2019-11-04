.PHONY: all deps test build

all: build

run:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY \
			golang:1.13.4-alpine3.10 \
			go run cmd/ecs-task-runner/main.go \
			run alpine --entrypoint env --extended-output

deps:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			golang:1.13.4-alpine3.10 go mod vendor

test:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			supinf/golangci-lint:1.19 \
			run --config .golangci.yml
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			--entrypoint go golang:1.13.4-alpine3.10 \
			test -vet off $(go list ./...)

build:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner \
			supinf/go-gox:1.11 --osarch "linux/amd64 darwin/amd64 windows/amd64" \
			-ldflags "-s -w" -output "dist/{{.OS}}_{{.Arch}}"
