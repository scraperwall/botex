package botex

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// API provides the HTTP REST API for botex
type API struct {
	botex  *Botex
	engine *gin.Engine
	config *Config
	ctx    context.Context
}

// NewAPI creates a new REST-API for botex
func NewAPI(ctx context.Context, config *Config, botex *Botex) (api *API, err error) {
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
	a.engine = r
	r.GET("/blocked", a.getBlocked)

	endless.ListenAndServe(a.config.APIAddress, r)
}

func (a *API) getBlocked(c *gin.Context) {

	blocked := make([]IPDetails, 0)

	err := a.botex.blocklist.config.KVStore.Each([]byte(blockNamespace), []byte{}, func(v []byte) {
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
