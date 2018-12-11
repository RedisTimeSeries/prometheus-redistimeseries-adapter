package redis_ts

import (
	"testing"

	"github.com/go-redis/redis"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

const redisAddress = "127.0.0.1:6379"
const redisAuth = ""
const sentinelAddress = "127.0.0.1:26379"
const sentinelMasterName = "mymaster"

var redisClient = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: redisAuth, // no password set
	DB:       0,         // use default DB
})

func Test_metricToKeyName(t *testing.T) {
	metric := model.Metric{
		model.MetricNameLabel:      "the_twist",
		"Z_should_be_last":         "42",
		"A_should_be_first":        "falafel",
		"U_are_so_beautiful_to_me": "sugar",
	}
	keyName := metricToKeyName(metric)
	expected := "the_twist{A_should_be_first=\"falafel\",U_are_so_beautiful_to_me=\"sugar\",Z_should_be_last=\"42\"}"
	assert.Equal(t, expected, keyName)
}

func TestWriteSingleSample(t *testing.T) {
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

	var redisTsClient = NewClient(redisAddress, redisAuth)

	err := redisTsClient.Write(samples)
	assert.Nil(t, err, "Write of samples failed")

	keys := redisClient.Keys("test_series{label_1=\"value_1\",label_2=\"value_2\"}").Val()
	assert.Len(t, keys, 1)
	labelsMatchers := []interface{}{"label_1=value_1"}
	ranges, err := redisTsClient.RangeByLabels(labelsMatchers, 0, now.Unix()+5).Result()
	assert.Nil(t, err, "RangeByLabels failed")
	assert.Len(t, ranges, 1)
}

func TestNewFailoverClient(t *testing.T) {
	var redisFailoverClient = NewFailoverClient(&redis.FailoverOptions{
		MasterName:    sentinelMasterName,
		SentinelAddrs: []string{sentinelAddress},
	})
	redisFailoverClient.Ping()
}

func Test_metricToLabels(t *testing.T) {
	m := model.Metric{
		"leaving": "jet_plane",
		"don't":   "know_when",
		"i'll":    "be_back_again",
	}
	interfaceSlice := metricToLabels(m)
	expected := []interface{}{
		"leaving=jet_plane",
		"don't=know_when",
		"i'll=be_back_again",
	}
	assert.Equal(t, expected, interfaceSlice)
}
