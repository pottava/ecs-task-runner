# ECS Task Runner v3.0
# docker run --rm -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY pottava/ecs-task-runner alpine --entrypoint env

FROM golang:1.23.3-alpine3.20 AS builder
RUN apk --no-cache add gcc musl-dev git
RUN mkdir -p /go/src/github.com/pottava
WORKDIR /go/src/github.com/pottava
ENV APP_VERSION=3.0.3
RUN git clone -b "${APP_VERSION}" https://github.com/pottava/ecs-task-runner.git
WORKDIR /go/src/github.com/pottava/ecs-task-runner
RUN go mod download
RUN go mod verify
RUN githash=$(git rev-parse --short HEAD 2>/dev/null) \
    && today=$(date +%Y-%m-%d --utc) \
    && cd /go/src/github.com/pottava/ecs-task-runner/cmd/ecs-task-runner \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags '-s -w -X main.ver=${APP_VERSION} -X main.commit=${githash} -X main.date=${today}' \
    -o /app

FROM alpine:3.20 AS libs
RUN apk --no-cache add ca-certificates

FROM scratch
COPY --from=libs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app /ecs-task-runner
ENTRYPOINT ["/ecs-task-runner", "run"]
CMD ["--help"]
