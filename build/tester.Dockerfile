FROM circleci/golang:1.16 as go
FROM ubuntu:bionic

COPY --from=go /usr/local/go /usr/local/go
RUN ln -s /usr/local/go/bin/* /usr/local/bin
RUN go version
ENV GOPATH /go
RUN apt update
RUN apt install -y git
RUN git clone --recursive https://github.com/RedisTimeSeries/RedisTimeSeries.git /go/redis-timeseries
WORKDIR /go/redis-timeseries
RUN ./deps/readies/bin/getpy3
RUN ./sbin/system-setup.py
RUN python3 ./deps/readies/bin/getredis -v6 --force
RUN make build

RUN set -e ;\
    echo daemonize yes > /tmp/sentinel.conf ;\
    echo sentinel monitor mymaster 127.0.0.1 6379 1 >> /tmp/sentinel.conf

RUN mkdir -p /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
WORKDIR /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

# This is not nessecerly the right way to do it, but it makes circleci works because it uses a remote docker host
COPY . /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

#ENTRYPOINT /bin/bash
CMD set -e ;\
    redis-sentinel /tmp/sentinel.conf ;\
    redis-server --daemonize yes --loadmodule /go/redis-timeseries/bin/redistimeseries.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360 ;\
    sleep 1 ;\
    redis-cli ping ;\
    make test
