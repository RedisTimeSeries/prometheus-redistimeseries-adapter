package redis_ts

import (
	"fmt"
	"math"
	"strings"

	"github.com/RedisLabs/redis-timeseries-go"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
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
		logger: logger,
		client: *c,
	}
}

// Write sends a batch of samples to RedisTS via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	for _, s := range samples {
		_, exists := s.Metric[model.MetricNameLabel]
		if !exists {
			_ = level.Debug(c.logger).Log("msg", "cannot send unnamed sample to RedisTS, skipping", "sample", s)
		}

		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			_ = level.Debug(c.logger).Log("msg", "cannot send to RedisTS, skipping sample", "value", v, "sample", s)
			continue
		}

		err := c.client.Add(metricToKeyName(s.Metric), s.Timestamp.Unix(), v)
		if err != nil {
			return err
		}
	}
	return nil
}

// Until Redis TSDB supports tagging, we handle labels by making them part of the TS key.
// The form is: <metric_name>[:<tag>=<value>][:<tag>=<value>]...
func metricToKeyName(m model.Metric) string {
	labels := make([]string, 0, len(m))

	labels = append(labels, string(m[model.MetricNameLabel]))

	for label, value := range m {
		if label != model.MetricNameLabel {
			labels = append(labels, fmt.Sprintf("%s=%s", label, value))
		}
	}

	return strings.Join(labels, ":")
}

// Name identifies the client as an RedisTS client.
func (c Client) Name() string {
	return "RedisTS"
}
