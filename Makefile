BINDIR  ?= bin
SRC_PKG = github.com/RedisTimeSeries/prometheus-redistimeseries-adapter
BIN_PATH = $(BINDIR)/redis_ts_adapter

build:
	CGO_ENABLED=0 go build -o $(BIN_PATH) ./cmd/redis-ts-adapter

$(BIN_PATH):
	$(MAKE) build

clean:
	rm -f bin/*

test:
	go test -v -cover ./...

.PHONY: build clean tests
