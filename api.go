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

	a.router.GET("/blocked", a.getBlocked)
	a.router.GET("/ip/:ip", a.getIP)

	go endless.ListenAndServe(a.config.APIAddress, a.router)
}

func (a *API) getBlocked(c *gin.Context) {
	blocked := make([]IPDetails, 0)

	err := a.botex.resources.Store.Each([]byte(blockNamespace), func(v []byte) {
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
