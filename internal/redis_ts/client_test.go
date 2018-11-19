package redis_ts

import (
	"testing"

	"github.com/RedisLabs/redis-timeseries-go"
	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

const redisAddress = "127.0.0.1:6379"

var redisTS = redis_timeseries_go.NewClient(redisAddress, "noname")

func cleanup(conn redis.Conn) {
	_, _ = conn.Do("FLUSHALL")
}

func TestWriteSingleSample(t *testing.T) {
	conn, err := redis.Dial("tcp", redisAddress)
	assert.Nil(t, err, "Could not connect to Redis")
	defer conn.Close()
	defer cleanup(conn)

	now := model.Now()
	answerToLifeTheUniverse := 42.1

	samples := model.Samples{
		&model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: "test_series",
				"label_1":             "value_1",
				"label_2":             "value_2",
			},
			Value:     model.SampleValue(answerToLifeTheUniverse),
			Timestamp: now,
		},
	}

	var remoteClient = NewClient(nil, redisAddress, "")

	err = remoteClient.Write(samples)
	assert.Nil(t, err, "Write of samples failed")

	keyName := metricToKeyName(samples[0].Metric)

	dataPoints, err := redisTS.Range(keyName, 0, now.Unix())
	assert.Nil(t, err, "Failed getting samples from Redis")
	assert.Len(t, dataPoints, 1, "Incorrect number of samples in Redis")
	dp := dataPoints[0]
	assert.Equal(t,
		redis_timeseries_go.DataPoint{
			Timestamp: now.Unix(),
			Value:     answerToLifeTheUniverse,
		},
		dp, "Unexpected sample in Redis",
	)
}
