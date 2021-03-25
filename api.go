package botex

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/fvbock/endless"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

// API provides the HTTP REST API for botex
type API struct {
	botex     *Botex
	router    *gin.Engine
	config    *config.Config
	resources *Resources
	ctx       context.Context
}

// NewAPI creates a new REST-API for botex
func NewAPI(ctx context.Context, config *config.Config, botex *Botex) (api *API, err error) {
	api = &API{
		config: config,
		ctx:    ctx,
		botex:  botex,
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

	go endless.ListenAndServe(a.config.APIAddress, a.router)
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
