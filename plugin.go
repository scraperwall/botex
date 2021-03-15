package botex

import (
	"github.com/gin-gonic/gin"
	"github.com/scraperwall/botex/data"
)

type Plugin interface {
	HandleRequest(r *data.Request) (cont bool)
	APIHooks(r *gin.Engine)
	ShouldBeBlocked(ipd data.IPStats) (block, next bool)
}
