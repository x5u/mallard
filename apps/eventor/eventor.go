package main

import (
	"runtime"
	"time"

	"github.com/baishancloud/mallard/componentlib/eventor/eventdata"
	"github.com/baishancloud/mallard/componentlib/eventor/eventorhandler"
	"github.com/baishancloud/mallard/componentlib/eventor/redisdata"
	"github.com/baishancloud/mallard/corelib/expvar"
	"github.com/baishancloud/mallard/corelib/httputil"
	"github.com/baishancloud/mallard/corelib/models"
	"github.com/baishancloud/mallard/corelib/osutil"
	"github.com/baishancloud/mallard/corelib/utils"
	"github.com/baishancloud/mallard/corelib/zaplog"
	"github.com/baishancloud/mallard/extralib/configapi"
	"github.com/go-redis/redis"
)

var (
	version    = "2.5.2"
	configFile = "config.json"
	cfg        = defaultConfig()
	log        = zaplog.Zap("eventor")
)

func prepare() {
	osutil.Flags(version, BuildTime, cfg)
	log.SetDebug(cfg.Debug)
	log.Info("init", "core", runtime.GOMAXPROCS(0), "version", version)

	// utils.WriteConfigFile(configFile, cfg)
	if err := utils.ReadConfigFile(configFile, &cfg); err != nil {
		log.Fatal("config-error", "error", err)
	}
}

func main() {
	prepare()

	queueCli, err := initRedis(cfg.RedisAddr,
		cfg.RedisPassword,
		cfg.RedisQueueDb, time.Second*10, 40)
	if err != nil {
		log.Fatal("init-redis-fail", "error", err)
	}
	cacheCli, err := initRedis(cfg.RedisAddr,
		cfg.RedisPassword,
		cfg.RedisCacheDb, time.Second*10, 40)
	if err != nil {
		log.Fatal("init-redis-fail", "error", err)
	}

	configapi.SetAPI(cfg.CenterAddr)
	configapi.SetHostService(&models.HostService{
		Hostname:       utils.HostName(),
		IP:             utils.LocalIP(),
		ServiceName:    "mallard2-eventor",
		ServiceVersion: version,
		ServiceBuild:   BuildTime,
	})
	configapi.SetIntervals([]string{"strategies", "endpoints", "expressions", "sync-hostservice"})
	go configapi.Intervals(time.Minute)

	redisdata.SetClient(queueCli, cacheCli)
	redisdata.SetAlarmLayout(cfg.RedisQueueLayout)

	eventdata.InitMemory()
	go eventdata.ScanOutdated(time.Minute * 2)
	go eventdata.ScanNodata(time.Minute * 2)
	go eventdata.StartGC(time.Minute)

	go httputil.Listen(cfg.HTTPAddr, eventorhandler.Create())

	go expvar.PrintAlways("mallard2_eventor_perf", cfg.PerfFile, time.Minute)

	/*etcdapi.MustSetClient([]string{"http://127.0.0.1:2379"}, "", "", time.Second)
	etcdapi.Register(etcdapi.Service{
		Name:      "mallard2-eventor",
		Endpoint:  utils.HostName(),
		Version:   "2.5.0",
		BuildTime: "",
	}, nil, time.Second*10)*/

	osutil.Wait()

	httputil.Close()
	// etcdapi.Deregister()
	log.Sync()
}

func initRedis(address, pwd string, db int, timeout time.Duration, poolSize int) (*redis.Client, error) {
	cli := redis.NewClient(&redis.Options{
		Addr:         address,
		Password:     pwd,
		DB:           db,
		DialTimeout:  timeout,
		WriteTimeout: timeout,
		ReadTimeout:  timeout,
		PoolSize:     poolSize,
	})
	return cli, cli.Ping().Err()
}
