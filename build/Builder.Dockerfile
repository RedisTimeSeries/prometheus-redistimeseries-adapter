FROM golang:1.14

WORKDIR /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
RUN mkdir -p /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter


# Build and install redis-server
RUN git clone -b 5.0 --depth 1 https://github.com/antirez/redis.git /redis/redis  && \
    cd /redis/redis  && \
    make -j install

# Build and install Redis Timeseries
RUN git clone https://github.com/RedisTimeSeries/RedisTimeSeries.git /redis/redis-timeseries && \
    cd /redis/redis-timeseries && \
    git submodule init && \
    git submodule update && \
    cd src && \
    make -j all

# install linter
RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | bash -s -- -b $GOPATH/bin v1.26.0

RUN redis-server --daemonize yes --loadmodule /redis/redis-timeseries/src/redis-tsdb-module.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360


ENTRYPOINT ["bash", "-c"]
