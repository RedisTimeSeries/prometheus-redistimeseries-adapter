FROM golang:1.11.1 as builder

RUN mkdir -p /go/src/github.com/RedisLabs/redis-ts-adapter
WORKDIR /go/src/github.com/RedisLabs/redis-ts-adapter
COPY . .
RUN CGO_ENABLED=0 go build -o redis-ts-adapter cmd/redis-ts-adapter/main.go


FROM alpine:3.6
WORKDIR /adapter
RUN adduser -D redis-adapter
USER redis-adapter

COPY --from=builder /go/src/github.com/RedisLabs/redis-ts-adapter/redis-ts-adapter /adapter/redis-ts-adapter

ENTRYPOINT ["/adapter/redis-ts-adapter"]