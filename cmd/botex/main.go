package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/namsral/flag"
	"github.com/scraperwall/botex"
	log "github.com/sirupsen/logrus"
)

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
	flag.StringVar(&config.StaticBaseURL, "static-base-url", "https://scw.im/files", "the base path for all static data files")
	flag.IntVar(&config.KeepRequests, "keep-requests", 50, "keep this many most recent requests")
	flag.IntVar(&config.ResolverWorkers, "resolver-workers", 30, "number of DNS resolver workers")
	flag.IntVar(&config.ResolverTries, "resolver-tries", 5, "try to resolve an IP this many times before giving up")
	flag.IntVar(&config.LogLevel, "loglevel", int(log.ErrorLevel), "the log level")
	flag.DurationVar(&config.ResolverTTL, "resolver-ttl", 3*30*24*time.Hour, "cache reverse hostnames this long")

	flag.Parse()

	log.SetLevel(log.Level(config.LogLevel))

	ctx, cancel := context.WithCancel(context.Background())

	b, err := botex.New(ctx, &config)
	if err != nil {
		log.Fatal(err)
	}

	b.Idle()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Println("exiting...")
	cancel()

	<-make(chan bool)
}
