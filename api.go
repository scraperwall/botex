package botex

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"

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
	engine    *gin.Engine
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

	go api.run()

	return api, nil
}

func (a *API) run() {
	r := gin.Default()
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	r.Use(cors.New(corsConfig))
	a.engine = r
	r.GET("/blocked", a.getBlocked)
	r.GET("/ip/:ip", a.getIP)
	/*
		if a.config.WithNetworks {
			r.GET("/networks", a.getNetworks)
			r.GET("/network/:ip/:bits", a.getNetwork)
		}
	*/

	endless.ListenAndServe(a.config.APIAddress, r)
}

func (a *API) getBlocked(c *gin.Context) {
	blocked := make([]IPDetails, 0)

	err := a.botex.resources.KVStore.Each([]byte(blockNamespace), []byte{}, func(v []byte) {
		var ipd IPDetails
		err := json.Unmarshal(v, &ipd)
		blocked = append(blocked, ipd)
		if err != nil {
			log.Warn(err)
		}
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to load all blocked IPs"})
		return
	}

	sort.Slice(blocked, func(a, b int) bool {
		return blocked[a].Total > blocked[b].Total
	})

	c.JSON(http.StatusOK, blocked)
}

func (a *API) getIP(c *gin.Context) {
	ipdata := a.botex.history.IPData(net.ParseIP(c.Param("ip")))

	if ipdata == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{})
		return
	}

	var data struct {
		IPDetails  *IPDetails
		Requests   []*data.Request
		Useragents map[string]int
	}

	data.IPDetails = &ipdata.IPDetails
	data.Requests = ipdata.Requests.Latest()
	data.Useragents = ipdata.Requests.Useragents()

	c.JSON(http.StatusOK, data)
}
