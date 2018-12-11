package redis_ts

import (
	"fmt"
	"github.com/go-redis/redis"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	log "github.com/sirupsen/logrus"
	"math"
	"sort"
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

func (c *Client) Add(key string, labels []interface{}, timestamp int64, value float64) *redis.StatusCmd {
	args := []interface{}{"TS.ADD", key}
	args = append(args, labels...)
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
		err := c.Add(metricToKeyName(s.Metric), metricToLabels(s.Metric), s.Timestamp.Unix(), v).Err()
		if err != nil {
			return err
		}
	}
	return nil
}

// Returns labels in string format (key=value), but as slice of interfaces.
func metricToLabels(m model.Metric) (labels []interface{}) {
	labels = make([]interface{}, 0, len(m))
	for label, value := range m {
		labels = append(labels, strings.Join([]string{string(label), string(value)}, "="))
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
	for _, q := range req.Queries {
		labelPairs, err := buildLabelPairs(q)
		if err != nil {
			return nil, err
		}
		cmd := c.RangeByLabels(labelPairs, q.StartTimestampMs/1000, q.EndTimestampMs/1000)
		err = cmd.Err()
		if err != nil {
			return nil, err
		}
		for _, ts := range cmd.Val() {
			tsSlice := ts.([]interface{})
			labels := tsSlice[1].([][]string)
			var tsLabels []*prompb.Label
			for _, label := range labels {
				tsLabels = append(tsLabels, &prompb.Label{Name: label[0], Value: label[1]})
			}

			samples := tsSlice[2].([][]interface{})
			var tsSamples []prompb.Sample
			for _, sample := range samples {
				tsSamples = append(tsSamples, prompb.Sample{Timestamp: sample[0].(int64), Value: sample[1].(float64)})
			}

			timeSerie := &prompb.TimeSeries{
				Labels:  tsLabels,
				Samples: tsSamples,
			}
			timeSeries = append(timeSeries, timeSerie)
		}

	}
	queryResult := prompb.QueryResult{Timeseries: timeSeries}
	resp := prompb.ReadResponse{Results: []*prompb.QueryResult{&queryResult}}
	return &resp, nil
}

func (c *Client) RangeByLabels(labelPairs []string, start int64, end int64) *redis.SliceCmd {
	// todo: find a way to check labelPairs is dividable by two, that matches style of go-redis
	args := []interface{}{"TS.RANGEBYLABELS"}
	numPairs := len(labelPairs) / 2
	for i := 0; i < numPairs; i++ {
		args = append(args, strings.Join([]string{labelPairs[2*i], labelPairs[2*i+1]}, "="))
	}
	args = append(args, start)
	args = append(args, end)
	cmd := redis.NewSliceCmd(args...)
	_ = c.Process(cmd)
	return cmd
}

func buildLabelPairs(q *prompb.Query) (labelPairs []string, err error) {
	for _, m := range q.Matchers {
		switch m.Type {
		case prompb.LabelMatcher_EQ:
			labelPairs = append(labelPairs, fmt.Sprintf("%q=%s", m.Name, m.Value))
		case prompb.LabelMatcher_NEQ:
			labelPairs = append(labelPairs, fmt.Sprintf("%q!=%s", m.Name, m.Value))
		case prompb.LabelMatcher_RE:
			return labelPairs, fmt.Errorf("regex-equal matcher is not supported yet. type: %v", m.Type)
		case prompb.LabelMatcher_NRE:
			return labelPairs, fmt.Errorf("regex-non-equal matcher is not supported yet. type: %v", m.Type)
		default:
			return labelPairs, fmt.Errorf("unknown match type %v", m.Type)
		}
	}
	return labelPairs, nil
}

// Name identifies the client as an RedisTS client.
func (c Client) Name() string {
	return "RedisTS"
}
