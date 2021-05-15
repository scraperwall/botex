/*
	botex - a bad bot mitigation tool by ScraperWall
	Copyright (C) 2021 ScraperWall, Tobias von Dewitz <tobias@scraperwall.com>

	This program is free software: you can redistribute it and/or modify it
	under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or (at your
	option) any later version.

	This program is distributed in the hope that it will be useful, but WITHOUT
	ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
	FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License
	for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

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
	NatsCA           string
	NatsCert         string
	NatsKey          string
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
	LogAnonymize     bool
	APIAddress       string
	LogMemoryStats   bool
	WhitelistTOML    string
	WithNetworks     bool
	ClearBlocked     bool
	SocketFile       string
	WebsocketIngest  bool
	IgnorePrivateIPs bool
	CookieName       string
	CookieKey        string
	CookieSecret     string
}
