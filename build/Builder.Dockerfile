FROM golang:1.15.13

WORKDIR /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
RUN mkdir -p /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

RUN git clone --recursive https://github.com/RedisTimeSeries/RedisTimeSeries.git /redis/redis-timeseries

RUN set -e ;\
    cd /redis/redis-timeseries ;\
    sudo make setup ;\
    sudo ./deps/readies/bin/getredis -v5 --force ;\
    make build

# install linter
RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | bash -s -- -b $GOPATH/bin v1.26.0

RUN redis-server --daemonize yes --loadmodule /redis/redis-timeseries/src/redis-tsdb-module.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360

ENTRYPOINT ["bash", "-c"]
