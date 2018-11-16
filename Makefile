BINDIR  ?= bin
SRC_PKG = github.com/RedisLabs/prometheus-redis-ts-adapter
DOCKER_IMAGE ?= redislabs/prometheus-redis-ts-adapter
DOCKER_IMAGE_TAG ?= beta
DOCKER_BUILDER = redislabs/adapter-builder
BIN_PATH = $(BINDIR)/prometheus_redis_ts_adapter

build:
	CGO_ENABLED=0 go build -o $(BIN_PATH)

docker_build_image:
	docker build -t $(DOCKER_BUILDER) -f build/Builder.Dockerfile build/

dockerized_make: docker_build_image
	docker run --rm -v `pwd`:/go/src/$(SRC_PKG) $(DOCKER_BUILDER) "make $(CMD)"

$(BIN_PATH):
	$(MAKE) build

image: $(BIN_PATH)
	cp $(BIN_PATH) build/
	docker build -t $(DOCKER_IMAGE):$(DOCKER_IMAGE_TAG) -f build/adapter.Dockerfile build/

clean:
	rm bin/*

test:
	go test -v -cover ./...

lint:
	golangci-lint run -E gofmt

push:
	docker push $(DOCKER_IMAGE):$(DOCKER_IMAGE_TAG)

.PHONY: build push clean image tests dockerized_make lint rhel_image