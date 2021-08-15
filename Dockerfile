FROM golang:1.16 as builder

RUN mkdir -p /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
WORKDIR /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
COPY . .
RUN CGO_ENABLED=0 go build -o redis-ts-adapter cmd/redis-ts-adapter/main.go


FROM alpine:3
WORKDIR /adapter
RUN adduser -D redis-adapter
USER redis-adapter

COPY --from=builder /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter/redis-ts-adapter /adapter/redis-ts-adapter

ENTRYPOINT ["/adapter/redis-ts-adapter"]
