# ECS Task Runner v2.2
# docker run --rm -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY pottava/ecs-task-runner alpine --entrypoint env

FROM golang:1.11.4-alpine3.8 AS build-env
RUN apk --no-cache add gcc musl-dev git
RUN go get -u github.com/golang/dep/...
WORKDIR /go/src/github.com/golang/dep
RUN git checkout v0.5.0 > /dev/null 2>&1
RUN go install github.com/golang/dep/...
RUN go get -u github.com/pottava/ecs-task-runner/...
WORKDIR /go/src/github.com/pottava/ecs-task-runner
ENV APP_VERSION=2.2
RUN git checkout "${APP_VERSION}" > /dev/null 2>&1
RUN dep ensure
RUN githash=$(git rev-parse --short HEAD 2>/dev/null) \
    && cd /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner \
    && go build -ldflags "-s -w -X main.date=$(date +%Y-%m-%d --utc) -X main.version=${APP_VERSION} -X main.commit=${githash}"
RUN mv /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner/ecs-task-runner /

FROM alpine:3.8
ENV AWS_DEFAULT_REGION=us-east-1
RUN apk add --no-cache ca-certificates
COPY --from=build-env /ecs-task-runner /ecs-task-runner
ENTRYPOINT ["/ecs-task-runner", "run"]
CMD ["--help"]
