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

	keys := redisClient.Keys("__name__=test_series,label_1=value_1,label_2=value_2").Val()
	assert.Len(t, keys, 1)
	labelsMatchers := []interface{}{"label_1=value_1"}
	cmd := redisTsClient.rangeByLabels(labelsMatchers, 0, now.Unix()+5)
	err = redisTsClient.Process(cmd)
	assert.Nil(t, err, "rangeByLabels failed to process")
	ranges, err := cmd.Result()
	assert.Nil(t, err, "rangeByLabels failed")
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
		"__name__": "wow",
		"leaving":  "jet_plane",
		"don't":    "know_when",
		"i'll":     "be_back_again",
	}
	labels, keyName := metricToLabels(m)
	expectedLabels := []string{
		"__name__=wow",
		"leaving=jet_plane",
		"don't=know_when",
		"i'll=be_back_again",
	}
	assert.ElementsMatch(t, expectedLabels, labels)
	expectedName := "__name__=wow,don't=know_when,i'll=be_back_again,leaving=jet_plane"
	assert.Equal(t, expectedName, keyName)

}
