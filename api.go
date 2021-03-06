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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/fvbock/endless"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

// API provides the HTTP REST API for botex
type API struct {
	botex                 *Botex
	router                *gin.Engine
	config                *config.Config
	websocketClients      map[*websocket.Conn]chan bool
	websocketClientsMutex sync.RWMutex
	ctx                   context.Context
}

// NewAPI creates a new REST-API for botex
func NewAPI(ctx context.Context, config *config.Config, botex *Botex) (api *API, err error) {
	api = &API{
		config:                config,
		websocketClients:      make(map[*websocket.Conn]chan bool),
		websocketClientsMutex: sync.RWMutex{},
		ctx:                   ctx,
		botex:                 botex,
	}

	api.run()

	return api, nil
}

func (a *API) run() {
	a.router = gin.Default()

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	a.router.Use(cors.New(corsConfig))

	a.router.GET("/blocked/ips", a.getBlockedIPs)
	a.router.GET("/ip/:ip", a.getIP)
	a.router.GET("/request-stats/:ip/:windows", a.getRequestStats)
	a.router.GET("/ips", a.getIPs)
	a.router.GET("/stats", a.getStats)
	a.router.GET("/ws", func(c *gin.Context) {
		a.websocketHandler(c.Writer, c.Request)
	})

	go a.processWebsocketData()
	go endless.ListenAndServe(a.config.APIAddress, a.router)
}

func (a *API) processWebsocketData() {
	for {
		select {
		case <-a.ctx.Done():
			a.websocketClientsMutex.Lock()
			for conn, ch := range a.websocketClients {
				conn.Close()
				ch <- true
				delete(a.websocketClients, conn)
			}
			a.websocketClientsMutex.Unlock()
			return
		case item := <-a.botex.resources.WebsocketChan:
			deadline := time.Now().Add(3 * time.Second)
			a.websocketClientsMutex.Lock()
			for conn, ch := range a.websocketClients {
				conn.SetWriteDeadline(deadline)
				err := conn.WriteJSON(item)
				if err != nil {
					log.Error(err)
					conn.Close()
					delete(a.websocketClients, conn) // delete the connection from our pool
					ch <- true                       // notify the handler function to terminate
				}
			}
			a.websocketClientsMutex.Unlock()
		}
	}
}

func (a *API) websocketHandler(w http.ResponseWriter, r *http.Request) {
	wsupgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to set websocket upgrade: %s", err)
		return
	}
	defer conn.Close()
	doneChan := make(chan bool)
	defer close(doneChan)
	a.websocketClientsMutex.Lock()
	a.websocketClients[conn] = doneChan
	a.websocketClientsMutex.Unlock()

	// send initial data to the client
	//
	a.botex.sendBlockedIPsToWebsocket()

	<-doneChan
	log.Tracef("leaving websocket handler")
}

func (a *API) getBlockedIPs(c *gin.Context) {
	sortBy := c.Query("sort")

	blocked := make([]data.IPBlockMessage, 0)

	err := a.botex.resources.Store.Each(a.botex.blocked.IPNamespace([]byte{}), func(v []byte) {
		var msg data.IPBlockMessage
		err := json.Unmarshal(v, &msg)
		if err != nil {
			log.Warn(err)
		}
		if ipd := a.botex.history.IPData(msg.IP); ipd != nil {
			msg.Total = ipd.Total
			msg.App = ipd.App
			msg.Other = ipd.Other
			msg.Ratio = ipd.Ratio
		}
		blocked = append(blocked, msg)
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to load all blocked IPs"})
		return
	}

	if sortBy == "total" {
		sort.Slice(blocked, func(a, b int) bool {
			return blocked[a].Total > blocked[b].Total
		})
	} else {
		sort.Slice(blocked, func(a, b int) bool {
			return int(blocked[a].BlockedAt.UnixNano()) > int(blocked[b].BlockedAt.UnixNano())
		})
	}

	c.JSON(http.StatusOK, blocked)
}

func (a *API) getIPs(c *gin.Context) {
	sortBy := c.Query("sort")
	byASN := c.Query("asn")
	byNetwork := c.Query("network")

	asnFilter := 0
	if byASN != "" {
		asn, err := strconv.Atoi(byASN)
		if err == nil {
			asnFilter = asn
		}
	}

	var networkFilter *net.IPNet
	if byNetwork != "" {
		_, network, err := net.ParseCIDR(byNetwork)
		if err == nil {
			networkFilter = network
		}
	}

	ips := make([]IPDetails, 0)

	a.botex.history.Each(func(ipstr string, ipd *IPData) {
		if asnFilter > 0 && ipd.ASN.ASN != asnFilter {
			return
		}

		if networkFilter != nil && !networkFilter.Contains(ipd.IP) {
			return
		}

		ips = append(ips, ipd.IPDetails)
	})

	if sortBy == "total" {
		sort.Slice(ips, func(a, b int) bool {
			return ips[a].Total > ips[b].Total
		})
	} else {
		sort.Slice(ips, func(a, b int) bool {
			return int(ips[a].CreatedAt.UnixNano()) > int(ips[b].CreatedAt.UnixNano())
		})
	}

	c.JSON(http.StatusOK, ips)
}

func (a *API) getIP(c *gin.Context) {
	ipdata := a.botex.history.IPData(net.ParseIP(c.Param("ip")))

	if ipdata == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{})
		return
	}

	var data struct {
		IPDetails  *IPDetails      `json:"ip_details"`
		Requests   []*data.Request `json:"requests"`
		Useragents map[string]int  `json:"useragents"`
	}

	data.IPDetails = &ipdata.IPDetails
	data.Requests = ipdata.Requests.Latest()
	data.Useragents = ipdata.Requests.Useragents()

	c.JSON(http.StatusOK, data)
}

func (a *API) getStats(c *gin.Context) {
	c.JSON(http.StatusOK, a.botex.history.stats.All())
}

func (a *API) getRequestStats(c *gin.Context) {
	numWindows, err := strconv.Atoi(c.Param("windows"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotAcceptable, gin.H{"error": "windows must be a number"})
		return
	}

	ip := net.ParseIP(c.Param("ip"))
	ipdata := a.botex.history.IPData(ip)
	if ipdata == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("IP %s not found", ip)})
		return
	}

	statsType := "all"
	if q := c.Query("type"); q != "" {
		statsType = q
	}

	var reqStats map[int]int64
	if statsType == "app" {
		reqStats = ipdata.Requests.AppStats()
	} else {
		reqStats = ipdata.Requests.TotalStats()
	}

	var tsMin, tsMax int
	for ts := range reqStats {
		if tsMin == 0 || tsMin > ts {
			tsMin = ts
		}

		if tsMax == 0 || tsMax < ts {
			tsMax = ts
		}
	}

	now := time.Now()
	windowSize := time.Duration(tsMax-tsMin) / time.Duration(numWindows)

	type RequestWindow struct {
		Time  time.Time `json:"time"`
		Count int64     `json:"count"`
	}

	stats := make([]RequestWindow, numWindows)

	if windowSize <= 0 {
		windowSize = time.Second
	}
	for i := 0; i < numWindows; i++ {
		stats[numWindows-1-i] = RequestWindow{
			Time:  now.Add(-1 * time.Duration(i) * windowSize).Truncate(windowSize),
			Count: 0,
		}
	}

	for ts, count := range reqStats {
		reqTime := time.Unix(0, int64(ts))

		idx := int(now.Sub(reqTime) / windowSize)
		if idx >= numWindows {
			idx = numWindows - 1
		}

		stats[idx].Count += count
	}

	c.JSON(http.StatusOK, stats)
}
