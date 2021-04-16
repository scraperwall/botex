package config

import (
	"time"
)

// Config contains all configurable bits and pieces the botex application needs
// The configuration gets passed on to all parts of the application that need to access it
type Config struct {
	WindowSize       time.Duration
	NumWindows       int
	DNSServer        string
	BadgerPath       string
	NatsAddr         string
	NatsPort         int
	NatsHTTPPort     int
	NatsUser         string
	NatsPassword     string
	GeoIPDBFile      string
	ASNDBFile        string
	KeepRequests     int
	ResolverWorkers  int
	ResolverTries    int
	ResolverTTL      time.Duration
	LogLevel         string
	BlockTTL         time.Duration
	MaxRatio         float64
	MinAppRequests   int
	MaxAppRequests   int
	LogReplay        string
	LogFormat        string
	APIAddress       string
	LogMemoryStats   bool
	WhitelistTOML    string
	WithNetworks     bool
	ClearBlocked     bool
	SocketFile       string
	IgnorePrivateIPs bool
	CookieName       string
	CookieKey        string
	CookieSecret     string
}
