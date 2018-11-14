FROM golang:1.11.1

WORKDIR /go/src/github.com/RedisLabs/prometheus-redis-ts-adapter
COPY * ./

RUN make build
RUN go test
CMD ls -l