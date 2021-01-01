FROM circleci/golang:1.14

RUN git clone --recursive https://github.com/RedisLabsModules/redis-timeseries.git

RUN set -e ;\
    cd redis-timeseries ;\
	sudo ./deps/readies/bin/getredis -v5 --force ;\
	sudo make setup ;\
	make build

RUN set -e ;\
    echo daemonize yes > /tmp/sentinel.conf ;\
    echo sentinel monitor mymaster 127.0.0.1 6379 1 >> /tmp/sentinel.conf

WORKDIR /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

RUN mkdir -p /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

# This is not nessecerly the right way to do it, but it makes circleci works because it uses a remote docker host
COPY . /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

#ENTRYPOINT /bin/bash
CMD set -e ;\
    redis-sentinel /tmp/sentinel.conf ;\
    redis-server --daemonize yes --loadmodule /go/redis-timeseries/bin/redistimeseries.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360 ;\
    sleep 1 ;\
    redis-cli ping ;\
    make test
