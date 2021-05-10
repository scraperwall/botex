package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"runtime"
	"syscall"
	"time"

	"github.com/namsral/flag"
	"github.com/scraperwall/botex"
	"github.com/scraperwall/botex/config"
	log "github.com/sirupsen/logrus"
)

type lineNumberHook struct{}

func (hook lineNumberHook) Levels() []log.Level {
	return log.AllLevels
}

func (hook lineNumberHook) Fire(entry *log.Entry) error {
	if pc, file, line, ok := runtime.Caller(9); ok {
		funcName := runtime.FuncForPC(pc).Name()

		entry.Data["source"] = fmt.Sprintf("%s:%v:%s", path.Base(file), line, path.Base(funcName))
	}

	return nil
}

func main() {
	go http.ListenAndServe(":8080", http.DefaultServeMux)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	config := config.Config{}

	flag.IntVar(&config.NumWindows, "num-windows", 60, "number of time windows")
	flag.DurationVar(&config.WindowSize, "window-size", time.Minute, "size of one window")
	flag.StringVar(&config.DNSServer, "dns-server", "8.8.8.8:53", "the DNS server to use")
	flag.StringVar(&config.BadgerPath, "badger-path", "./badger", "the directory where the badger database resides")
	flag.StringVar(&config.NatsAddr, "nats-addr", "0.0.0.0", "bind NATS to this IP")
	flag.IntVar(&config.NatsPort, "nats-port", 4223, "the port on which NATS listens")
	flag.IntVar(&config.NatsHTTPPort, "nats-http-port", 8889, "the HTTP port on which NATS listens")
	flag.StringVar(&config.NatsUser, "nats-user", "scw", "the NATS user")
	flag.StringVar(&config.NatsPassword, "nats-password", "scw", "the NATS password")
	flag.StringVar(&config.NatsCA, "nats-ca", "", "the NATS TLS CA certificate. leave empty if not using TLS")
	flag.StringVar(&config.NatsCert, "nats-cert", "", "the NATS TLS certificate. leave empty if not using TLS")
	flag.StringVar(&config.NatsKey, "nats-key", "", "the NATS TLS key. leave empty if not using TLS")
	flag.StringVar(&config.GeoIPDBFile, "geoipdb-file", "./maxmind/geoip-2021-03-02.tar.gz", "the path to the Maxmind GeoLite2 City file")
	flag.StringVar(&config.ASNDBFile, "asndb-file", "./maxmind/asndb-2021-03-02.zip", "the path to the Maxmind GeoLite2 ASN file")
	flag.IntVar(&config.KeepRequests, "keep-requests", 50, "keep this many most recent requests")
	flag.IntVar(&config.ResolverWorkers, "resolver-workers", 30, "number of DNS resolver workers")
	flag.IntVar(&config.ResolverTries, "resolver-tries", 5, "try to resolve an IP this many times before giving up")
	flag.StringVar(&config.LogLevel, "loglevel", "error", "the logrus loglevel (panic, fatal, error, warn, info, debug, trace)")
	flag.DurationVar(&config.ResolverTTL, "resolver-ttl", 3*30*24*time.Hour, "cache reverse hostnames this long")
	flag.DurationVar(&config.BlockTTL, "block-ttl", 3*time.Hour, "block bot IPs this long")
	flag.IntVar(&config.MinAppRequests, "min-app-requests", 10, "don't block an IP if it makes less than this many app requests")
	flag.IntVar(&config.MaxAppRequests, "max-app-requests", 50, "block an IP if it makes more than this many app requests")
	flag.Float64Var(&config.MaxRatio, "max-ratio", 0.8, "block IPs if the app/total ratio is above this value and it has more than -min-app-requests app requests")
	flag.StringVar(&config.LogFormat, "log-format", `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`, "the log format to parse")
	flag.StringVar(&config.LogReplay, "log-replay", "", "replay this log file")
	flag.StringVar(&config.APIAddress, "api-address", "127.0.0.1:4343", "the address and port at which the API listens")
	flag.BoolVar(&config.LogMemoryStats, "log-memory-stats", false, "regularly log the memory consumption of the program")
	flag.StringVar(&config.WhitelistTOML, "whitelist", "./etc/whitelist.toml", "the whitelist configuration file")
	flag.BoolVar(&config.WithNetworks, "networks", false, "analyze requests on a per-network basis")
	flag.BoolVar(&config.ClearBlocked, "clear-blocked", false, "removes all blocked items from the database")
	flag.StringVar(&config.SocketFile, "socket-file", "", "the name of the Unix domain socket file to use. Serves as interface block requests in webservers")
	flag.BoolVar(&config.IgnorePrivateIPs, "ignore-private-ips", true, "ignore private non-routable IPs")
	flag.StringVar(&config.CookieName, "cookie-name", "scw", "the name of the human verification cookie set by the external captcha service")
	flag.StringVar(&config.CookieKey, "cookie-key", "", "the human cookie AES-256 key (base64-encoded)")
	flag.StringVar(&config.CookieSecret, "cookie-secret", "", "the secret to use inside the encrypted human cookie")

	flag.Parse()

	ll, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(ll)
	log.AddHook(lineNumberHook{})

	ctx, cancel := context.WithCancel(context.Background())

	b, err := botex.New(ctx, &config)
	if err != nil {
		log.Fatal(err)
	}

	// b.Idle()

	if config.LogReplay != "" {
		go func() {
			b.LogReplay(config.LogReplay, config.LogFormat)
		}()
	}

	<-quit
	cancel()
	log.Println("exiting...")

	<-make(chan bool)
}
