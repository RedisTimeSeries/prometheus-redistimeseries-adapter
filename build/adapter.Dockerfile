FROM alpine:3.6

WORKDIR /adapter
RUN adduser -D redis-adapter
USER redis-adapter

COPY redis_ts_adapter /usr/local/bin/redis_ts_adapter

ENTRYPOINT /usr/local/bin/redis_ts_adapter