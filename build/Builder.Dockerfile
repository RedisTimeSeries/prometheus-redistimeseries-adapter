FROM golang:1.11.1

WORKDIR /go/src/github.com/RedisLabs/redis-ts-adapter
RUN mkdir -p /go/src/github.com/RedisLabs/redis-ts-adapter


# Build and install redis-server
RUN git clone -b 5.0 --depth 1 https://github.com/antirez/redis.git /redis/redis  && \
    cd /redis/redis  && \
    make -j install

# Build and install Redis Timeseries
RUN git clone -b labels https://github.com/RedisLabsModules/redis-timeseries.git /redis/redis-timeseries && \
    cd /redis/redis-timeseries && \
    git submodule init && \
    git submodule update && \
    cd src && \
    make -j all

# install linter
RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | bash -s -- -b $GOPATH/bin v1.11.1

RUN redis-server --daemonize yes --loadmodule /redis/redis-timeseries/src/redis-tsdb-module.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360


ENTRYPOINT ["bash", "-c"]
