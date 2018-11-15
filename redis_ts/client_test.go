package redis_ts

import (
	"github.com/RedisLabs/redis-timeseries-go"
	"github.com/garyburd/redigo/redis"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"testing"
)

const redisAddress = "127.0.0.1:6379"

var redisTS = redis_timeseries_go.NewClient(redisAddress, "noname")

func TestWriteSingleSample(t *testing.T) {
	conn, err := redis.Dial("tcp", redisAddress)
	require.Nil(t, err, "Could not connect to Redis")
	defer conn.Close()
	defer conn.Do("FLUSHALL")

	now := model.Now()
	answerToLifeTheUniverse := 42.0

	var seriesName model.LabelValue = "test_series"

	samples := model.Samples{
		&model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: seriesName,
				"label_1":             "value_1",
				"label_2":             "value_2",
			},
			Value:     model.SampleValue(answerToLifeTheUniverse),
			Timestamp: now,
		},
	}

	var remoteClient = NewClient(nil, redisAddress, "")

	err = remoteClient.Write(samples)
	require.Nil(t, err, "Write of samples failed")

	dataPoints, err := redisTS.Range(string(seriesName), 0, now.Unix())
	require.Nil(t, err, "Failed getting samples from Redis")
	require.Len(t, dataPoints, 1, "Incorrect number of samples in Redis")
	dp := dataPoints[0]
	require.Equal(t,
		redis_timeseries_go.DataPoint{
			Timestamp: now.Unix(),
			Value: answerToLifeTheUniverse,
		},
		dp,"Unexpected sample in Redis",
	)
}
