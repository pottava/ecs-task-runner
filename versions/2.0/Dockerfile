FROM alpine:3.8

ENV DEP_VERSION=v0.5.0 \
    APP_VERSION=2.0 \
    AWS_DEFAULT_REGION=us-east-1

RUN apk add --no-cache ca-certificates

RUN apk --no-cache add --virtual build-deps gcc musl-dev go git \
    && export GOPATH=/go \
    && export PATH=$GOPATH/bin:$PATH \
    && mkdir $GOPATH \
    && chmod -R 777 $GOPATH \
    && go get -u github.com/golang/dep/... \
    && cd /go/src/github.com/golang/dep \
    && git checkout ${DEP_VERSION} \
    && cd cmd/dep \
    && go build \
    && go get github.com/pottava/ecs-task-runner \
    && cd /go/src/github.com/pottava/ecs-task-runner \
    && git checkout ${APP_VERSION} \
    && githash=$(git rev-parse --short HEAD 2>/dev/null) \
    && /go/src/github.com/golang/dep/cmd/dep/dep ensure \
    && cd cmd/ecs-task-runner \
    && go build -ldflags "-s -w -X main.date=$(date +%Y-%m-%d --utc) -X main.version=${APP_VERSION} -X main.commit=${githash}" \
    && mv ecs-task-runner /usr/bin/ \
    && apk del --purge -r build-deps \
    && cd / \
    && rm -rf /go /root/.cache

ENTRYPOINT ["ecs-task-runner", "run"]
CMD ["--help"]
