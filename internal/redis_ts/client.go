package redis_ts

import (
	"bytes"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/prometheus/prometheus/prompb"
	lru "github.com/hashicorp/golang-lru"
	log "github.com/sirupsen/logrus"
	radix "github.com/mediocregopher/radix/v3"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	redis.Client

	Cache lru.Cache
	rpool *radix.Pool
	objPool sync.Pool
	bufferPool sync.Pool
	cmdActionSlicePool sync.Pool
}
type StatusCmd redis.StatusCmd

const nameLabel = "__name__"
const TS_ADD = "TS.ADD"
const LABELS = "LABELS"

// NewClient creates a new Client.
func NewClient(address string, auth string) *Client {
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: auth,
		DB:       0, // use default DB
	})
	cache, _ := lru.New(1024)
	customConnFunc := func(network, addr string) (radix.Conn, error) {
		return radix.Dial(network, addr,
			radix.DialTimeout(1 * time.Minute),
			radix.DialAuthPass(auth),
		)
	}
	rpool, err := radix.NewPool("tcp", address, 10, radix.PoolConnFunc(customConnFunc))
	if err != nil {
		panic(err)
	}
	return &Client{
		Client: *client,
		Cache:  *cache,
		rpool: rpool,
	}
}

func NewFailoverClient(failoverOpt *redis.FailoverOptions) *Client {
	client := redis.NewFailoverClient(failoverOpt)
	cache, _ := lru.New(1024)
	return &Client{
		Client: *client,
		Cache:  *cache,
	}
}
//
func Testadd(c *Client, args []string, key string, labels []prompb.Label, metric string, timestamp *int64, value *float64) radix.CmdAction {
	return c.add(args, key, labels, metric, timestamp, value)
}


func (c *Client) add(args []string, key string, labels []prompb.Label, metric string, timestamp *int64, value *float64) radix.CmdAction {
	// TODO: make TS_ADD, LABELS, key and actual labels interface{} cached
	//args := make([]string, 0, len(labels)*2+7)
	args = append(args, key)
	args = append(args, strconv.FormatInt(*timestamp, 10))
	args = append(args, strconv.FormatFloat(*value, 'f', 6, 64))
	args = append(args, LABELS)
	hasNameLabel := false
	for i := range labels {
		args = append(args, labels[i].Name, labels[i].Value)
		if labels[i].Name == nameLabel {
			hasNameLabel = true
		}
	}
	if !hasNameLabel {
		args = append(args, nameLabel, metric)
	}
	cmd := radix.Cmd(nil, TS_ADD, args...)
	return cmd
}

func (c *Client) getArgs() []string {
	if strSlice := c.objPool.Get(); strSlice != nil {
		return strSlice.([]string)
	}
	return make([]string, 0, 100)
}

func (c *Client) getBuffer() *bytes.Buffer {
	if bb := c.bufferPool.Get(); bb != nil {
		return bb.(*bytes.Buffer)
	}
	return bytes.NewBuffer(make([]byte, 1024))
}

func (c *Client) getCmdActionSlice() []radix.CmdAction {
	if ca := c.cmdActionSlicePool.Get(); ca != nil {
		return ca.([]radix.CmdAction)
	}
	return make([]radix.CmdAction, 0, 100)
}

// Write sends a batch of samples to RedisTS via its HTTP API.
func (c *Client) Write(timeseries []prompb.TimeSeries) (returnErr error) {
	//pipe := c.Pipeline()
	//defer func() {
	//	err := pipe.Close()
	//	if err != nil {
	//		returnErr = err
	//	}
	//}()

	cmds := make([]radix.CmdAction, 0, len(timeseries))
	args := make([]string, 0, 100)
	buf := bytes.NewBuffer(make([]byte, 1024))
	for i := range timeseries {
		samples := timeseries[i].Samples
		cmds = cmds[:0]
		buf.Reset()
		labels, metric := metricToLabels(timeseries[i].Labels, buf)
		buf.Reset()
		key := metricToKeyName(metric, labels, buf)
		if metric == "" {
			log.WithFields(log.Fields{"Metric": timeseries[i].Labels}).Info("Cannot send unnamed sample to RedisTS, skipping")
			continue
		}
		for j := range samples {
			if math.IsNaN(samples[j].Value) || math.IsInf(samples[j].Value, 0) {
				log.WithFields(log.Fields{"sample": samples[j], "value": samples[j].Value}).Debug("Cannot send to RedisTS, skipping")
				continue
			}

			cmd := c.add(args, key, timeseries[i].Labels, metric, &samples[j].Timestamp, &samples[j].Value)
			cmds = append(cmds, cmd)
		}
	}

	// TODO: ignore errors for debugging
	err := c.rpool.Do(radix.Pipeline(cmds...))
	//c.objPool.Put(args)
	//c.bufferPool.Put(buf)
	//c.cmdActionSlicePool.Put(cmds)

	return err
}

// Returns labels in string format (key=value), but as slice of interfaces.
func metricToLabels(l []prompb.Label, buf *bytes.Buffer) ([]string, string) {
	var labels = make([]string, 0, len(l))
	var metric string
	for i := range l {
		if l[i].Name == nameLabel {
			metric = l[i].Value
		} else {
			buf.Reset()
			buf.WriteString(l[i].Name)
			buf.WriteString("=")
			buf.WriteString(l[i].Value)
			labels = append(labels, buf.String())
		}
	}
	sort.Strings(labels)
	return labels, metric
}

// We add labels to TS key, to keep key unique per labelSet.
// The form is: <metric_name>{[<tag>="<value>"][,<tag>="<value>"…]}
func metricToKeyName(metric string, labels []string, buf *bytes.Buffer) (keyName string) {
	buf.WriteString(metric)
	buf.WriteString("{")
	buf.WriteString(strings.Join(labels, ","))
	buf.WriteString("}")
	return buf.String()
}

func (c *Client) Read(req *prompb.ReadRequest) (returnVal *prompb.ReadResponse, returnErr error) {
	var timeSeries []*prompb.TimeSeries
	results := make([]*prompb.QueryResult, 0, len(req.Queries))
	pipe := c.Pipeline()
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
			tsLabels := make([]prompb.Label, 0, len(labels))
			for _, label := range labels {
				parsedLabel := label.([]interface{})
				tsLabels = append(tsLabels, prompb.Label{Name: parsedLabel[0].(string), Value: parsedLabel[1].(string)})
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
