package redis_ts

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/go-redis/redis"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

type Client redis.Client
type StatusCmd redis.StatusCmd

// NewClient creates a new Client.
func NewClient(address string, auth string) *Client {
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: auth,
		DB:       0, // use default DB
	})
	myClient := Client(*client)
	return &myClient
}

func (c *Client) Add(key string, timestamp int64, value float64) *redis.StatusCmd {
	cmd := redis.NewStatusCmd("TS.ADD", key, timestamp, value)
	_ = c.Process(cmd)
	return cmd
}

// Write sends a batch of samples to RedisTS via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	for _, s := range samples {
		_, exists := s.Metric[model.MetricNameLabel]
		if !exists {
			log.WithFields(log.Fields{"sample": s}).Info("Cannot send unnamed sample to RedisTS, skipping")
		}

		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.WithFields(log.Fields{"sample": s, "value": v}).Info("Cannot send to RedisTS, skipping")
			continue
		}

		err := c.Add(metricToKeyName(s.Metric), s.Timestamp.Unix(), v).Err()
		if err != nil {
			return err
		}
	}
	return nil
}

// Until Redis TSDB supports tagging, we handle labels by making them part of the TS key.
// The form is: <metric_name>{[<tag>="<value>"][,<tag>="<value>"…]}
func metricToKeyName(m model.Metric) (keyName string) {
	keyName = string(m[model.MetricNameLabel])
	labels := make([]string, 0, len(m))

	for label, value := range m {
		if label != model.MetricNameLabel {
			labels = append(labels, fmt.Sprintf("%s=\"%s\"", label, value))
		}
	}
	sort.Strings(labels)
	keyName += "{" + strings.Join(labels, ",") + "}"
	return keyName
}

// Name identifies the client as an RedisTS client.
func (c Client) Name() string {
	return "RedisTS"
}
