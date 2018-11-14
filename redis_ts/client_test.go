package redis_ts

import (
	"github.com/garyburd/redigo/redis"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const redisAddress = "127.0.0.1:6379"

var redisPool *redis.Pool = &redis.Pool{
	MaxIdle:     3,
	IdleTimeout: 240 * time.Second,
	Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", redisAddress) },
}
var remoteClient *Client = NewClient(nil, redisAddress, "")

func TestWriteSingleSample(t *testing.T) {
	samples := model.Samples{
		&model.Sample{
			Metric: model.Metric{
				model.LabelName("label_1"): model.LabelValue("value_1"),
				model.LabelName("label_2"): model.LabelValue("value_2"),
			},
			Value:     42.0,
			Timestamp: model.Now(),
		},
	}

	err := remoteClient.Write(samples)
	require.Nil(t, err, "Write of samples should succeed")

	conn := redisPool.Get()
	defer conn.Close()

	t.Log(conn.Do("KEYS", "*"))
}
