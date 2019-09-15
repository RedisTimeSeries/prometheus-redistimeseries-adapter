package main

import (
	"flag"
	"github.com/go-redis/redis"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/RedisTimeSeries/prometheus-redistimeseries-adapter/internal/redis_ts"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/profile"
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
	Profile                 bool
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
	flag.BoolVar(&cfg.Profile, "profile", false, "Run with profile")

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
	Write(samples []prompb.TimeSeries) error
	Name() string
}

type reader interface {
	Read(req *prompb.ReadRequest) (*prompb.ReadResponse, error)
	Name() string
}

func buildClient(cfg *config) *redis_ts.Client {
	if cfg.redisSentinelAddress != "" {
		log.WithFields(log.Fields{"sentinel_address": cfg.redisSentinelAddress}).Info("Creating redis sentinel client")
		client := redis_ts.NewFailoverClient(&redis.FailoverOptions{
			MasterName:         cfg.redisSentinelMasterName,
			SentinelAddrs:      []string{cfg.redisSentinelAddress},
			PoolSize:           cfg.PoolSize,
			IdleTimeout:        cfg.IdleTimeout,
			IdleCheckFrequency: cfg.IdleCheckFrequency,
			WriteTimeout:       cfg.WriteTimeout,
			Password:           cfg.redisAuth,
		})
		return client
	}
	if cfg.redisAddress != "" {
		log.WithFields(log.Fields{"redis_ts_address": cfg.redisAddress}).Info("Creating redis TS client")
		client := redis_ts.NewClient(
			cfg.redisAddress,
			cfg.redisAuth)
		return client
	}
	// TODO: build redis reader here
	log.Info("Starting up...")
	return nil
}

func serve(addr string, writer writer, reader reader) error {
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
		if err := req.Unmarshal(reqBuf); err != nil {
			log.WithFields(log.Fields{"err": err.Error()}).Error("Unmarshal error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sendSamples(writer, req.Timeseries)
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

		if reader == nil {
			http.Error(w, "Cannot serve data to an invalid reader", http.StatusInternalServerError)
			return
		}

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
	if cfg.Profile {
		defer profile.Start().Stop()
	}

	client := buildClient(cfg)
	log.WithFields(log.Fields{"address": cfg.listenAddr}).Info("listening...")
	if err := serve(cfg.listenAddr, client, client); err != nil {
		log.WithFields(log.Fields{"address": cfg.listenAddr, "err": err}).Error("Failed to listen")
		os.Exit(1)
	}
}

func sendSamples(w writer, samples []prompb.TimeSeries) {
	err := w.Write(samples)
	if err != nil {
		log.WithFields(log.Fields{"storage": w.Name(), "err": err, "num_samples": len(samples)}).Warn("Could not send samples to remote storage")
	}
}
