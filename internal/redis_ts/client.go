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

type Tag struct {
	label, value string
}

func (t Tag) String() string {
	return strings.Join([]string{t.label, t.value}, "=")
}

// NewClient creates a new Client.
func NewClient(address string, auth string) *Client {
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: auth,
		DB:       0, // use default DB
	})
	return (*Client)(client)
}

func NewFailoverClient(failoverOpt *redis.FailoverOptions) *Client {
	client := redis.NewFailoverClient(failoverOpt)
	return (*Client)(client)
}

func (c *Client) Add(key string, tags []Tag, timestamp int64, value float64) *redis.StatusCmd {
	args := []interface{}{"TS.ADD", key}
	for _, tag := range tags {
		args = append(args, tag.String())
	}
	args = append(args, timestamp)
	args = append(args, value)
	cmd := redis.NewStatusCmd(args...)
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
		err := c.Add(metricToKeyName(s.Metric), metricToTags(s.Metric), s.Timestamp.Unix(), v).Err()
		if err != nil {
			return err
		}
	}
	return nil
}

func metricToTags(m model.Metric) (tags []Tag) {
	for label, value := range m {
		tags = append(tags, Tag{string(label), string(value)})
	}
	return tags
}

// We add labels to TS key, to keep key unique per labelSet.
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
