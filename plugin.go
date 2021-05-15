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
	"net"

	"github.com/gin-gonic/gin"
	"github.com/scraperwall/botex/data"
)

type Plugin interface {
	HandleRequest(r *data.Request)
	APIHooks(r *gin.Engine)
	SetBlocker(b data.Blocker)
	ShouldBeBlocked(stats data.IPStats) (blocked bool, reason string)
	IsWhitelisted(ip net.IP) bool
}
