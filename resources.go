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

package botex

import (
	natsd "github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/store"
	"github.com/scraperwall/geoip/v2"
)

type Resources struct {
	ASNDB         *asndb.DB
	GEOIPDB       *geoip.DB
	NatsServer    *natsd.Server
	NatsConn      *nats.Conn
	Store         store.KVStore
	Whitelist     *Whitelist
	Resolver      *Resolver
	WebsocketChan chan interface{}

	BlockChan chan *IPDetails
}

func NewResources() *Resources {
	return &Resources{
		WebsocketChan: make(chan interface{}, 100),
	}
}
