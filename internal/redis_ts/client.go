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

func add(key string, labels []interface{}, timestamp int64, value float64) redis.Cmder {
	args := []interface{}{"TS.ADD", key}
	args = append(args, labels...)
	args = append(args, timestamp)
	args = append(args, value)
	cmd := redis.NewStatusCmd(args...)
	return cmd
}

// Write sends a batch of samples to RedisTS via its HTTP API.
func (c *Client) Write(samples model.Samples) error {
	pipe := (*redis.Client)(c).Pipeline()

	for _, s := range samples {
		_, exists := s.Metric[model.MetricNameLabel]
		if !exists {
			log.WithFields(log.Fields{"sample": s}).Info("Cannot send unnamed sample to RedisTS, skipping")
			continue
		}

		v := float64(s.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.WithFields(log.Fields{"sample": s, "value": v}).Info("Cannot send to RedisTS, skipping")
			continue
		}
		cmd := add(metricToKeyName(s.Metric), metricToLabels(s.Metric), s.Timestamp.Unix(), v)
		err := pipe.Process(cmd)
		if err != nil {
			return err
		}
	}

	_, err := pipe.Exec()
	return err
}

// Returns labels in string format (key=value), but as slice of interfaces.
func metricToLabels(m model.Metric) (labels []interface{}) {
	labels = make([]interface{}, 0, len(m))
	for label, value := range m {
		labels = append(labels, fmt.Sprintf("%s=%s", label, value))
	}
	return labels
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

	for _, cmd := range commands {
		err := cmd.Err()
		if err != nil {
			return nil, err
		}

		for _, ts := range cmd.Val() {
			tsSlice := ts.([]interface{})
			labels := tsSlice[1].([]interface{})
			tsLabels := make([]*prompb.Label, 0, len(labels))
			for _, label := range labels {
				parsedLabel := label.([]interface{})
				tsLabels = append(tsLabels, &prompb.Label{Name: parsedLabel[0].(string), Value: parsedLabel[1].(string)})
			}

			samples := tsSlice[2].([]interface{})
			tsSamples := make([]prompb.Sample, 0, len(samples))
			for _, sample := range samples {
				parsedSample := sample.([]interface{})
				value, err := strconv.ParseFloat(parsedSample[1].(string), 64)
				if err != nil {
					return nil, err
				}
				tsSamples = append(tsSamples, prompb.Sample{Timestamp: parsedSample[0].(int64) * 1000, Value: value})
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
