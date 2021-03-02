package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"syscall"
	"time"

	"github.com/namsral/flag"
	"github.com/scraperwall/botex"
	log "github.com/sirupsen/logrus"
)

type lineNumberHook struct{}

func (hook lineNumberHook) Levels() []log.Level {
	return log.AllLevels
}

func (hook lineNumberHook) Fire(entry *log.Entry) error {
	if pc, file, line, ok := runtime.Caller(10); ok {
		funcName := runtime.FuncForPC(pc).Name()

		entry.Data["source"] = fmt.Sprintf("%s:%v:%s", path.Base(file), line, path.Base(funcName))
	}

	return nil
}

func main() {

	config := botex.Config{}

	flag.IntVar(&config.NumWindows, "num-windows", 60, "number of time windows")
	flag.DurationVar(&config.WindowSize, "window-size", time.Minute, "size of one window")
	flag.StringVar(&config.DNSServer, "dns-server", "8.8.8.8:53", "the DNS server to use")
	flag.StringVar(&config.BadgerPath, "badger-path", "./badger", "the directory where the badger database resides")
	flag.StringVar(&config.NatsAddr, "nats-addr", "0.0.0.0", "bind NATS to this IP")
	flag.IntVar(&config.NatsPort, "nats-port", 4223, "the port on which NATS listens")
	flag.IntVar(&config.NatsHTTPPort, "nats-http-port", 8889, "the HTTP port on which NATS listens")
	flag.StringVar(&config.NatsUser, "nats-user", "scw", "the NATS user")
	flag.StringVar(&config.NatsPassword, "nats-password", "scw", "the NATS password")
	flag.StringVar(&config.GeoIPDBFile, "geoipdb-file", "./maxmind/geoip-2021-03-02.tar.gz", "the path to the Maxmind GeoLite2 City file")
	flag.StringVar(&config.ASNDBFile, "asndb-file", "./maxmind/asndb-2021-03-02.zip", "the path to the Maxmind GeoLite2 ASN file")
	flag.IntVar(&config.KeepRequests, "keep-requests", 50, "keep this many most recent requests")
	flag.IntVar(&config.ResolverWorkers, "resolver-workers", 30, "number of DNS resolver workers")
	flag.IntVar(&config.ResolverTries, "resolver-tries", 5, "try to resolve an IP this many times before giving up")
	flag.IntVar(&config.LogLevel, "loglevel", int(log.ErrorLevel), "the log level")
	flag.DurationVar(&config.ResolverTTL, "resolver-ttl", 3*30*24*time.Hour, "cache reverse hostnames this long")
	flag.DurationVar(&config.BlockTTL, "block-ttl", 3*time.Hour, "block bot IPs this long")
	flag.IntVar(&config.MinAppRequests, "min-app-requests", 10, "don't block an IP if it makes less than this many app requests")
	flag.IntVar(&config.MaxAppRequests, "max-app-requests", 50, "block an IP if it makes more than this many app requests")
	flag.Float64Var(&config.MaxRatio, "max-ratio", 0.8, "block IPs if the app/total ratio is above this value and it has more than -min-app-requests app requests")

	flag.Parse()

	log.SetLevel(log.Level(config.LogLevel))
	log.AddHook(lineNumberHook{})

	ctx, cancel := context.WithCancel(context.Background())

	b, err := botex.New(ctx, &config)
	if err != nil {
		log.Fatal(err)
	}

	b.Idle()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	cancel()
	log.Println("exiting...")

	<-make(chan bool)
}
