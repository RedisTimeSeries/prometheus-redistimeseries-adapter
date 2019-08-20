FROM circleci/golang:1.11
RUN echo /go && git clone -b 5.0 --depth 1 https://github.com/antirez/redis.git
RUN cd redis && sudo make -j install

RUN echo /go && git clone https://github.com/RedisLabsModules/redis-timeseries.git
RUN cd redis-timeseries && \
            git submodule init && \
            git submodule update && \
            cd src && \
            make -j all

RUN echo daemonize yes > /tmp/sentinel.conf
RUN echo sentinel monitor mymaster 127.0.0.1 6379 1 >> /tmp/sentinel.conf

WORKDIR /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
RUN mkdir -p /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter

# This is not nessecerly the right way to do it, but it makes circleci works because it uses a remote docker host
COPY . /go/src/github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
#ENTRYPOINT /bin/bash
CMD redis-sentinel /tmp/sentinel.conf && \
        redis-server --daemonize yes --loadmodule /go/redis-timeseries/bin/redistimeseries.so RETENTION_POLICY 0 MAX_SAMPLE_PER_CHUNK 360 && \
                                                     sleep 1 && redis-cli ping && make test