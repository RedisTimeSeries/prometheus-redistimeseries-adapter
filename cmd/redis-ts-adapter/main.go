package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/RedisLabs/redis-ts-adapter/internal/redis_ts"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	log "github.com/sirupsen/logrus"
)

type config struct {
	redisAddress            string
	redisSentinelAddress    string
	redisSentinelMasterName string
	redisAuth               string
	remoteTimeout           time.Duration
	listenAddr              string
	logLevel                string
	PoolSize                int
	IdleTimeout             time.Duration
	IdleCheckFrequency      time.Duration
	WriteTimeout            time.Duration
}

var cfg = &config{}

func init() {
	parseFlags()
	setupLogger()
}

func parseFlags() {
	cfg.redisAuth = os.Getenv("REDIS_AUTH")

	flag.StringVar(&cfg.redisAddress, "redis-address", "",
		"The host:port of the Redis server to send samples to. empty, if empty.",
	)
	flag.StringVar(&cfg.redisSentinelAddress, "redis-sentinel-address", "",
		"The host:port of the Redis Sentinel server to query. empty, if empty.",
	)
	flag.StringVar(&cfg.redisSentinelMasterName, "redis-sentinel-master", "",
		"The name of the master to find in Redis Sentinel. empty, if empty.",
	)
	flag.DurationVar(&cfg.remoteTimeout, "send-timeout", 30*time.Second,
		"The timeout to use when sending samples to the remote storage.",
	)
	flag.StringVar(&cfg.listenAddr, "web.listen-address", "127.0.0.1:9201",
		"Address to listen on for web endpoints.",
	)
	flag.StringVar(&cfg.logLevel, "log.level", "info",
		"Only log messages with the given severity or above. One of: [debug, info, warn, error]",
	)
	flag.IntVar(&cfg.PoolSize, "redis-pool-size", 500,
		"Maximum number of socket connections.")
	flag.DurationVar(&cfg.IdleTimeout, "redis-idle-timeout", 10*time.Minute,
		"Amount of time after which client closes idle connections.")
	flag.DurationVar(&cfg.IdleCheckFrequency, "redis-idle-check-frequency", 30*time.Second,
		"Frequency of idle checks made by client.")
	flag.DurationVar(&cfg.WriteTimeout, "redis-write-timeout", 1*time.Minute,
		"Redis write timeout.")

	flag.Parse()
	validateConfiguration()
}

func validateConfiguration() {
	if cfg.redisAddress != "" && (cfg.redisSentinelAddress != "" || cfg.redisSentinelMasterName != "") {
		log.Error("Invalid configuration: Cannot have both redis-address and redis-sentinel-address")
		os.Exit(1)
	}

	if (cfg.redisSentinelAddress != "") != (cfg.redisSentinelMasterName != "") {
		log.Error("Invalid configuration: Sentinel configuration requires both sentinel address and master name")
		os.Exit(1)
	}
}

func setupLogger() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetOutput(os.Stdout)

	level, err := log.ParseLevel(cfg.logLevel)
	if err != nil {
		log.WithFields(log.Fields{"wantedLogLevel": cfg.logLevel}).Warn("Could not set log level. Reverting to info log level.")
		level = log.InfoLevel
	}
	log.SetLevel(level)
}

type writer interface {
	Write(samples model.Samples) error
	Name() string
}

type reader interface {
	Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error)
	Name() string
}

func buildClients(cfg *config) ([]writer, []reader) {
	var writers []writer
	var readers []reader
	if cfg.redisSentinelAddress != "" {
		log.WithFields(log.Fields{"sentinel_address": cfg.redisSentinelAddress}).Info("Creating redis sentinel client")
		client := redis_ts.NewFailoverClient(&redis.FailoverOptions{
			MasterName:         cfg.redisSentinelMasterName,
			SentinelAddrs:      []string{cfg.redisSentinelAddress},
			PoolSize:           cfg.PoolSize,
			IdleTimeout:        cfg.IdleTimeout,
			IdleCheckFrequency: cfg.IdleCheckFrequency,
			WriteTimeout:       cfg.WriteTimeout,
		})
		readers = append(readers, client)
		writers = append(writers, client)
	}
	if cfg.redisAddress != "" {
		log.WithFields(log.Fields{"redis_ts_address": cfg.redisAddress}).Info("Creating redis TS client")
		client := redis_ts.NewClient(
			cfg.redisAddress,
			cfg.redisAuth)
		readers = append(readers, client)
		writers = append(writers, client)
	}
	// TODO: build redis reader here
	log.Info("Starting up...")
	return writers, readers
}

func protoToSamples(req *prompb.WriteRequest) model.Samples {
	var samples model.Samples
	for _, ts := range req.Timeseries {
		metric := make(model.Metric, len(ts.Labels))
		for _, l := range ts.Labels {
			metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
		}

		for _, s := range ts.Samples {
			samples = append(samples, &model.Sample{
				Metric:    metric,
				Value:     model.SampleValue(s.Value),
				Timestamp: model.Time(s.Timestamp),
			})
		}
	}
	return samples
}

func serve(addr string, writers []writer, readers []reader) error {
	http.HandleFunc("/write", func(w http.ResponseWriter, r *http.Request) {
		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Read error")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Decode error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Unmarshal error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		samples := protoToSamples(&req)

		var wg sync.WaitGroup
		for _, w := range writers {
			wg.Add(1)
			go func(rw writer) {
				sendSamples(rw, samples)
				wg.Done()
			}(w)
		}
		wg.Wait()
	})

	http.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) {
		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Read error")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Decode error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.ReadRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Unmarshal error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// TODO: Support reading from more than one reader and merging the results.
		if len(readers) != 1 {
			http.Error(w, fmt.Sprintf("expected exactly one reader, found %d readers", len(readers)), http.StatusInternalServerError)
			return
		}
		reader := readers[0]

		var resp *prompb.ReadResponse
		resp, err = reader.Read(&req)
		if err != nil {
			log.WithFields(log.Fields{"query": req, "storage": reader.Name(), "err": err}).Error("Error executing query")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err := proto.Marshal(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Set("Content-Encoding", "snappy")

		compressed = snappy.Encode(nil, data)
		if _, err := w.Write(compressed); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	return http.ListenAndServe(addr, nil)
}

func main() {
	writers, readers := buildClients(cfg)
	log.WithFields(log.Fields{"address": cfg.listenAddr}).Info("listening...")
	if err := serve(cfg.listenAddr, writers, readers); err != nil {
		log.WithFields(log.Fields{"address": cfg.listenAddr, "err": err}).Error("Failed to listen")
		os.Exit(1)
	}
}

func sendSamples(w writer, samples model.Samples) {
	err := w.Write(samples)
	if err != nil {
		log.WithFields(log.Fields{"storage": w.Name(), "err": err, "num_samples": len(samples)}).Warn("Could not send samples to remote storage")
	}
}
