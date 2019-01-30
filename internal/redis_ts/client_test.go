package redis_ts

import (
	"github.com/prometheus/prometheus/prompb"
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

	samples := []*prompb.TimeSeries{
		{
			Labels: []*prompb.Label{
				{
					Name:  "label_1",
					Value: "value_1",
				},
				{
					Name:  "label_2",
					Value: "value_2",
				},
				{
					Name:  "__name__",
					Value: "test_series",
				},
			},
			Samples: []prompb.Sample{
				{
					Timestamp: now.Unix(),
					Value:     answerToLifeTheUniverse,
				},
			},
		},
	}
	var redisTsClient = NewClient(redisAddress, redisAuth)

	err := redisTsClient.Write(samples)
	assert.Nil(t, err, "Write of samples failed")

	keys := redisClient.Keys("test_series{label_1=value_1,label_2=value_2}").Val()
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
	m1 := []*prompb.Label{
		{
			Name:  "leaving",
			Value: "jet_plane",
		},
		{
			Name:  "don't",
			Value: "know_when",
		},
		{
			Name:  "i'll",
			Value: "be_back_again",
		},
		{
			Name:  "__name__",
			Value: "wow",
		},
	}
	m2 := []*prompb.Label{
		{
			Name:  "leaving",
			Value: "jet_plane",
		},
		{
			Name:  "i'll",
			Value: "be_back_again",
		},
		{
			Name:  "don't",
			Value: "know_when",
		},
		{
			Name:  "__name__",
			Value: "wow",
		},
	}
	m3 := []*prompb.Label{

		{
			Name:  "i'll",
			Value: "be_back_again",
		},
		{
			Name:  "__name__",
			Value: "wow",
		},
		{
			Name:  "don't",
			Value: "know_when",
		},
		{
			Name:  "leaving",
			Value: "jet_plane",
		},
	}

	testMetricToLabels(t, m1)
	testMetricToLabels(t, m2)
	testMetricToLabels(t, m3)
}

func testMetricToLabels(t *testing.T, l []*prompb.Label) {
	labels, metricName := metricToLabels(l)
	expected := []interface{}{
		"leaving=jet_plane",
		"i'll=be_back_again",
		"don't=know_when",
	}
	assert.ElementsMatch(t, expected, *labels)
	assert.Equal(t, "wow", *metricName)

	keyName := metricToKeyName(metricName, labels)
	expected_key := "wow{don't=know_when,i'll=be_back_again,leaving=jet_plane}"
	assert.Equal(t, expected_key, keyName)
}
