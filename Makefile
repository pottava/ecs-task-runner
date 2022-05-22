.PHONY: all run start stop deps test build

all: build

run:
	@docker run --rm -it -v "${GOPATH}":/go \
			-v "${HOME}/.aws":/root/.aws \
			-w /go/src/github.com/pottava/ecs-task-runner \
			-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY \
			-e AWS_MFA_SERIAL_NUMBER -e AWS_MFA_TOKEN \
			-e AWS_PROFILE -e AWS_ASSUME_ROLE \
			-e AWS_DEFAULT_REGION=us-west-2 \
			-e APP_DEBUG=0 \
			golang:1.17.5-alpine3.15 \
			go run cmd/ecs-task-runner/main.go \
			run alpine --entrypoint env --extended-output

start:
	@docker run --rm -it -v "${GOPATH}":/go \
			-v "${HOME}/.aws":/root/.aws \
			-w /go/src/github.com/pottava/ecs-task-runner \
			-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY \
			-e AWS_MFA_SERIAL_NUMBER -e AWS_MFA_TOKEN \
			-e AWS_PROFILE -e AWS_ASSUME_ROLE \
			-e AWS_DEFAULT_REGION=us-west-2 \
			-e APP_DEBUG=0 \
			golang:1.17.5-alpine3.15 \
			go run cmd/ecs-task-runner/main.go \
			run dockercloud/hello-world \
			--async -p 80 --security-groups "${SECURITY_GROUP_ID}" --spot

stop:
	@docker run --rm -it -v "${GOPATH}":/go \
			-v "${HOME}/.aws":/root/.aws \
			-w /go/src/github.com/pottava/ecs-task-runner \
			-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY \
			-e AWS_MFA_SERIAL_NUMBER -e AWS_MFA_TOKEN \
			-e AWS_PROFILE -e AWS_ASSUME_ROLE \
			-e AWS_DEFAULT_REGION=us-west-2 \
			-e APP_DEBUG=0 \
			golang:1.17.5-alpine3.15 \
			go run cmd/ecs-task-runner/main.go \
			stop "${REQUEST_ID}"

deps:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			golang:1.17.5-alpine3.15 go mod vendor

test:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			supinf/golangci-lint:1.19 \
			run --config .golangci.yml
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner \
			--entrypoint go golang:1.17.5-alpine3.15 \
			test -vet off $(go list ./...) -mod=readonly

build:
	@docker run --rm -it -v "${GOPATH}":/go \
			-w /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner \
			supinf/go-gox:1.11 --osarch "linux/amd64 darwin/amd64 windows/amd64" \
			-ldflags "-s -w" -output "dist/{{.OS}}_{{.Arch}}"
