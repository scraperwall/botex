package plugins

import (
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/data"
	"github.com/scraperwall/geoip/v2"
	log "github.com/sirupsen/logrus"
)

type IPMeta struct {
	asndb *asndb.DB
	geodb *geoip.DB
}

func NewIPMeta(asndb *asndb.DB, geodb *geoip.DB) *IPMeta {
	return &IPMeta{
		asndb: asndb,
		geodb: geodb,
	}
}

func (i *IPMeta) IsWhitelisted(ip net.IP) bool {
	return false
}

func (i *IPMeta) SetBlocker(b data.Blocker) {}

func (i *IPMeta) HandleRequest(r *data.Request) {}

func (i *IPMeta) ShouldBeBlocked(stats data.IPStats) (blocked bool, reason string) {
	return false, ""
}

func (i *IPMeta) getASN(c *gin.Context) {
	ip := net.ParseIP(c.Param("ip"))
	if ip == nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Sprintf("%s is not a valid IP address", c.Param("ip"))})
		return
	}

	asn := i.asndb.Lookup(ip)
	c.JSON(http.StatusOK, asn)
}

func (i *IPMeta) getGeoIP(c *gin.Context) {
	ip := net.ParseIP(c.Param("ip"))
	if ip == nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Sprintf("%s is not a valid IP address", c.Param("ip"))})
		return
	}

	geo, err := i.geodb.Lookup(ip)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": err})
		return
	}

	c.JSON(http.StatusOK, geo)
}

func (i *IPMeta) APIHooks(r *gin.Engine) {
	if r == nil {
		log.Fatal("gin router is nil")
	}
	r.GET("/ipmeta/asn/:ip", i.getASN)
	r.GET("/ipmeta/geoip/:ip", i.getGeoIP)
}
