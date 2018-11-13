package redis_ts

import (
	"github.com/RedisLabs/redis-timeseries-go"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"math"
)

// Client allows sending batches of Prometheus samples to Redis TS.
type Client struct {
	logger log.Logger

	client redis_timeseries_go.Client
}

// NewClient creates a new Client.
func NewClient(logger log.Logger, redisAddress string, redisAuth string) *Client {
	c := redis_timeseries_go.NewClient(redisAddress, "redis_ts_adapter")

	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Client{
		logger:          logger,
		client:          *c,

	}
}

// Write sends a batch of samples to InfluxDB via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	for _, s := range samples {
		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			level.Debug(c.logger).Log("msg", "cannot send to RedisTS, skipping sample", "value", v, "sample", s)
			continue
		}

		err := c.client.Add(string(s.Metric[model.MetricNameLabel]), s.Timestamp.Unix(),v)
		if err != nil {
			return err
		}
	}
	return nil
}


// Name identifies the client as an RedisTS client.
func (c Client) Name() string {
	return "RedisTS"
}
