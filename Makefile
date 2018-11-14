build:
	curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
	dep ensure -v
	mkdir -p bin
	go build -i -o bin/prometheus_redis_ts_adapter main.go
	chmod +x bin/prometheus_redis_ts_adapter