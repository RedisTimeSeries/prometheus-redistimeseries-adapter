package redis_ts

import (
	"fmt"
	"github.com/go-redis/redis"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	log "github.com/sirupsen/logrus"
	"math"
	"sort"
	"strconv"
	"strings"
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
	return (*Client)(client)
}

func NewFailoverClient(failoverOpt *redis.FailoverOptions) *Client {
	client := redis.NewFailoverClient(failoverOpt)
	return (*Client)(client)
}

func add(key string, labels *[]string, timestamp int64, value float64) redis.Cmder {
	args := []interface{}{"TS.ADD", key}
	for i := range *labels {
		args = append(args, (*labels)[i])
	}
	args = append(args, strconv.FormatInt(timestamp, 10))
	args = append(args, strconv.FormatFloat(value, 'E', -1, 64))
	cmd := redis.NewStatusCmd(args...)
	return cmd
}

// Write sends a batch of samples to RedisTS via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	pipe := (*redis.Client)(c).Pipeline()

	for i := range samples {
		_, exists := samples[i].Metric[model.MetricNameLabel]
		if !exists {
			log.WithFields(log.Fields{"sample": samples[i]}).Info("Cannot send unnamed sample to RedisTS, skipping")
			continue
		}

		v := float64(samples[i].Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.WithFields(log.Fields{"sample": samples[i], "value": v}).Debug("Cannot send to RedisTS, skipping")
			continue
		}
		labels := metricToLabels(samples[i].Metric)
		cmd := add(metricToKeyName(samples[i].Metric, labels), labels, samples[i].Timestamp.Unix(), v)
		err := pipe.Process(cmd)
		if err != nil {
			return err
		}
	}

	_, err := pipe.Exec()
	return err
}

// Returns labels in string format (key=value), but as slice of interfaces.
func metricToLabels(m model.Metric) *[]string {
	var labels = make([]string, 0, len(m))
	for label, value := range m {
		labels = append(labels, fmt.Sprintf("%s=%s", label, value))
	}
	sort.Strings(labels)
	return &labels
}

// We add labels to TS key, to keep key unique per labelSet.
// The form is: <metric_name>{[<tag>="<value>"][,<tag>="<value>"…]}
func metricToKeyName(m model.Metric, labels *[]string) (keyName string) {
	keyName = string(m[model.MetricNameLabel])
	keyName += "{" + strings.Join(*labels, ",") + "}"
	return keyName
}

func (c *Client) Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	var timeSeries []*prompb.TimeSeries
	results := make([]*prompb.QueryResult, 0, len(req.Queries))
	pipe := (*redis.Client)(c).Pipeline()

	commands := make([]*redis.SliceCmd, 0, len(req.Queries))
	for _, q := range req.Queries {
		labelMatchers, err := labelMatchers(q)
		if err != nil {
			return nil, err
		}
		cmd := c.rangeByLabels(labelMatchers, q.StartTimestampMs/1000, q.EndTimestampMs/1000)
		err = pipe.Process(cmd)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}

	_, err := pipe.Exec()
	if err != nil {
		return nil, err
	}

	for i := range commands {
		err := commands[i].Err()
		if err != nil {
			return nil, err
		}

		for _, ts := range commands[i].Val() {
			tsSlice := ts.([]interface{})
			labels := tsSlice[1].([][]string)
			tsLabels := make([]*prompb.Label, 0, len(labels))
			for _, label := range labels {
				tsLabels = append(tsLabels, &prompb.Label{Name: label[0], Value: label[1]})
			}

			samples := tsSlice[2].([][]interface{})
			tsSamples := make([]prompb.Sample, 0, len(samples))
			for i := range samples {
				sample := samples[i]
				tsSamples = append(tsSamples, prompb.Sample{Timestamp: sample[0].(int64), Value: sample[1].(float64)})
			}

			thisSeries := &prompb.TimeSeries{
				Labels:  tsLabels,
				Samples: tsSamples,
			}
			timeSeries = append(timeSeries, thisSeries)
		}
		results = append(results, &prompb.QueryResult{Timeseries: timeSeries})
	}

	resp := prompb.ReadResponse{Results: results}
	return &resp, nil
}

func (c *Client) rangeByLabels(labelMatchers []interface{}, start int64, end int64) *redis.SliceCmd {
	args := make([]interface{}, 0, len(labelMatchers)+3)
	args = append(args, "TS.RANGEBYLABELS")
	args = append(args, labelMatchers...)
	args = append(args, start)
	args = append(args, end)
	cmd := redis.NewSliceCmd(args...)
	return cmd
}

func labelMatchers(q *prompb.Query) (labels []interface{}, err error) {
	labels = make([]interface{}, 0, len(q.Matchers))
	for _, m := range q.Matchers {
		switch m.Type {
		case prompb.LabelMatcher_EQ:
			labels = append(labels, fmt.Sprintf("%s=%s", m.Name, m.Value))
		case prompb.LabelMatcher_NEQ:
			labels = append(labels, fmt.Sprintf("%s!=%s", m.Name, m.Value))
		case prompb.LabelMatcher_RE:
			return labels, fmt.Errorf("regex-equal matcher is not supported yet. type: %v", m.Type)
		case prompb.LabelMatcher_NRE:
			return labels, fmt.Errorf("regex-non-equal matcher is not supported yet. type: %v", m.Type)
		default:
			return labels, fmt.Errorf("unknown match type %v", m.Type)
		}
	}
	return labels, nil
}

// Name identifies the client as an RedisTS client.
func (c Client) Name() string {
	return "RedisTS"
}
