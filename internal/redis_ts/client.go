package redis_ts

import (
	"bytes"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/prometheus/prometheus/prompb"
	log "github.com/sirupsen/logrus"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Client redis.Client
type StatusCmd redis.StatusCmd

const nameLabel = "__name__"

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

func add(key *string, labels []*prompb.Label, metric *string, timestamp *int64, value *float64) redis.Cmder {
	args := make([]interface{}, 0, len(labels)+3)
	args = append(args, "TS.ADD", *key)
	args = append(args, strconv.FormatInt(*timestamp, 10))
	args = append(args, strconv.FormatFloat(*value, 'f', 6, 64))
	args = append(args, "LABELS")
	hasNameLabel := false
	for i := range labels {
		args = append(args, labels[i].Name, labels[i].Value)
		if labels[i].Name == nameLabel {
			hasNameLabel = true
		}
	}
	if !hasNameLabel {
		args = append(args, nameLabel, *metric)
	}
	cmd := redis.NewStatusCmd(args...)
	return cmd
}

// Write sends a batch of samples to RedisTS via its HTTP API.
func (c *Client) Write(timeseries []*prompb.TimeSeries) (returnErr error) {
	pipe := (*redis.Client)(c).Pipeline()
	defer func() {
		err := pipe.Close()
		if err != nil {
			returnErr = err
		}
	}()

	for i := range timeseries {
		samples := timeseries[i].Samples
		labels, metric := metricToLabels(timeseries[i].Labels)
		key := metricToKeyName(metric, labels)
		if *metric == "" {
			log.WithFields(log.Fields{"Metric": timeseries[i].Labels}).Info("Cannot send unnamed sample to RedisTS, skipping")
			continue
		}
		for j := range samples {
			sample := &samples[j]
			if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
				log.WithFields(log.Fields{"sample": sample, "value": sample.Value}).Debug("Cannot send to RedisTS, skipping")
				continue
			}

			cmd := add(&key, timeseries[i].Labels, metric, &sample.Timestamp, &sample.Value)
			err := pipe.Process(cmd)
			if err != nil {
				return err
			}
		}
	}

	_, err := pipe.Exec()
	return err
}

// Returns labels in string format (key=value), but as slice of interfaces.
func metricToLabels(l []*prompb.Label) (*[]string, *string) {
	var labels = make([]string, 0, len(l)-1)
	var metric *string
	var buf bytes.Buffer
	for i := range l {
		if l[i].Name == "__name__" {
			metric = &l[i].Value
		} else {
			buf.Reset()
			buf.WriteString(l[i].Name)
			buf.WriteString("=")
			buf.WriteString(l[i].Value)
			labels = append(labels, buf.String())
		}
	}
	sort.Strings(labels)
	return &labels, metric
}

// We add labels to TS key, to keep key unique per labelSet.
// The form is: <metric_name>{[<tag>="<value>"][,<tag>="<value>"…]}
func metricToKeyName(metric *string, labels *[]string) (keyName string) {
	var buf bytes.Buffer
	buf.WriteString(*metric)
	buf.WriteString("{")
	buf.WriteString(strings.Join(*labels, ","))
	buf.WriteString("}")
	return buf.String()
}

func (c *Client) Read(req *prompb.ReadRequest) (returnVal *prompb.ReadResponse, returnErr error) {
	var timeSeries []*prompb.TimeSeries
	results := make([]*prompb.QueryResult, 0, len(req.Queries))
	pipe := (*redis.Client)(c).Pipeline()
	defer func() {
		err := pipe.Close()
		if err != nil {
			returnErr = err
		}
	}()

	commands := make([]*redis.SliceCmd, 0, len(req.Queries))
	for _, q := range req.Queries {
		labelMatchers, err := labelMatchers(q)
		if err != nil {
			return nil, err
		}
		cmd := c.rangeByLabels(labelMatchers, q.StartTimestampMs, q.EndTimestampMs)
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
			labels := tsSlice[1].([]interface{})
			tsLabels := make([]*prompb.Label, 0, len(labels))
			for _, label := range labels {
				parsedLabel := label.([]interface{})
				tsLabels = append(tsLabels, &prompb.Label{Name: parsedLabel[0].(string), Value: parsedLabel[1].(string)})
			}

			samples := tsSlice[2].([]interface{})
			tsSamples := make([]prompb.Sample, 0, len(samples))
			for i := range samples {
				parsedSample := samples[i].([]interface{})
				value, err := strconv.ParseFloat(parsedSample[1].(string), 64)
				if err != nil {
					return nil, err
				}
				tsSamples = append(tsSamples, prompb.Sample{Timestamp: parsedSample[0].(int64), Value: value})
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
	args = append(args, "TS.MRANGE")
	args = append(args, start)
	args = append(args, end)
	args = append(args, "WITHLABELS")
	args = append(args, "FILTER")
	args = append(args, labelMatchers...)
	log.WithFields(log.Fields{"args": args}).Debug("ts.mrange")
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
