package redis_ts

import (
	"testing"

	"github.com/go-redis/redis"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

const redisAddress = "127.0.0.1:6379"
const redisAuth = ""

var redisClient = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: redisAuth, // no password set
	DB:       0,         // use default DB
})

func cleanup(client redis.Client) {
	_ = client.FlushAll()
}

func Test_MetricToKeyName(t *testing.T) {
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
	defer cleanup(*redisClient)
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
}
