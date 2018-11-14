FROM golang:1.11.1

ENV GOPATH /go_ts

WORKDIR /go_ts/src/github.com/RedisLabs/prometheus-redis-ts-adapter
COPY . .
RUN mkdir -p $GOPATH/bin $GOPATH/pkg

# Build and install redis-server
RUN git clone -b 5.0 --depth 1 https://github.com/antirez/redis.git /redis
RUN cd /redis && make -j install

# Build and install Redis Timeseries
RUN git clone https://github.com/RedisLabsModules/redis-timeseries.git /redis/redis-timeseries

RUN cd /redis/redis-timeseries && \
    git submodule init && \
    git submodule update && \
    cd src && \
    make -j all

# Satisfy dependencies
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
RUN $GOPATH/bin/dep ensure

# Run Redis with TS module, then run tests
CMD redis-server --daemonize yes --loadmodule /redis/redis-timeseries/src/redis-tsdb-module.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360 && \
    go test -v -coverprofile=coverage.out github.com/RedisLabs/prometheus-redis-ts-adapter/redis_ts
