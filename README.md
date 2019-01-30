[![license](https://img.shields.io/github/license/RedisTimeSeries/redis-ts-adapter.svg)](https://github.com/RedisTimeSeries/redis-ts-adapter)
[![CircleCI](https://circleci.com/gh/RedisTimeSeries/redis-ts-adapter/tree/master.svg?style=svg&circle-token=508830e660830ee06491b3e566337b6ce50c3355)](https://circleci.com/gh/RedisTimeSeries/redis-ts-adapter/tree/master)
[![GitHub issues](https://img.shields.io/github/release/RedisTimeSeries/redis-ts-adapter.svg)](https://github.com/RedisTimeSeries/redis-ts-adapter/releases/latest)

# Redis TimeSeries Adapter
Redis TimeSeries Adapter receives [Prometheus][prometheus] metrics via the 
[remote write][prometheus_remote_write], 
and writes to [Redis with TimeSeries module][redis_time_series].

## Getting Started
to build the project:
```bash
make build
cd bin
```

To send metrics to Redis, provide address in host:port format. 
```bash
redis-ts-adapter --redis-address localhost:6379
```

To receive metrics from Prometheus, Add [remote write][prometheus_remote_write_config] 
section to prometheus configuration:
```yaml
remote_write:
  - url: 'http://127.0.0.1:9201/write'
```  

## Makefile commands
run tests:
```bash
make test
```
go linting:
```bash
make lint
```
### Redis Sentinel
If you have Redis Sentinel set up for high availability redis, use the `redis-sentinel` flags:
```bash
redis-ts-adapter --redis-sentinel-address localhost:26379 --redis-sentinel-master mydb
```

## Additional flags

Print help:
```bash
redis-ts-adapter --help
```

Set log level:
```bash
redis-ts-adapter --log.level debug
```

Set the timeout to use when sending samples to the remote storage:
```bash
redis-ts-adapter --send-timeout 60s
```
Set the listening port for prometheus to send metrics: 
```bash
redis-ts-adapter --web.listen-address 127.0.0.1:9201
```

## Contributing
[Contribution guidelines for this project](CONTRIBUTING.md)

## Releases
See the [releases on this repository](https://github.com/RedisLabs/redis-ts-adapter/releases).

## Contributors
See also the list of [contributors](https://github.com/RedisLabs/redis-ts-adapter/contributors) who participated in this project.

## License

See the [LICENSE](LICENSE) file for details.

## Acknowledgments

* Thanks to the prometheus community for the awesome [prometheus project][prometheus], and for [the example adapter](https://github.com/prometheus/prometheus/tree/master/documentation/examples/remote_storage/remote_storage_adapter), that was adapted to this project.
* Thanks to the [go-redis](https://github.com/go-redis/redis) community, for the golang redis client used here.
* Thanks to [PurpleBooth](https://github.com/PurpleBooth) for [readme template](https://gist.github.com/PurpleBooth/109311bb0361f32d87a2)

[prometheus]: https://prometheus.io
[prometheus_remote_write]: https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations
[prometheus_remote_write_config]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#%3Cremote_write%3E
[redis_time_series]: https://github.com/RedisLabsModules/redis-timeseries
[project_github_url]: https://github.com/RedisLabs/redis-ts-adapter
