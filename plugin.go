package botex

import (
	"net"

	"github.com/gin-gonic/gin"
	"github.com/scraperwall/botex/data"
)

type Plugin interface {
	HandleRequest(r *data.Request)
	APIHooks(r *gin.Engine)
	SetBlocker(b data.Blocker)
	ShouldBeBlocked(stats data.IPStats) (block bool)
	IsWhitelisted(ip net.IP) bool
}
