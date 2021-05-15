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

package data

import (
	"net"

	"github.com/scraperwall/asndb/v2"
)

type Blocker interface {
	BlockASN(msg BlockMessage) error
	BlockIP(msg IPBlockMessage) error
	BlockNetwork(msg NetworkBlockMessage) error
	BlockedNetworks() []NetworkBlockMessage
	BlockedASNs() []BlockMessage
	BlockedIPs() []IPBlockMessage
	IPNamespace(ip []byte) []byte
	ASNNamespace(asn int) []byte
	CIDRNamespace(cidr string) []byte
	IsBlocked(ip net.IP, asn *asndb.ASN) bool
	IsBlockedByASN(asn *asndb.ASN) (blocked bool, reason string)
	CheckBlocked()
}
