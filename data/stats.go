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

// Stats contains aggregated statistics about a single IP
type Stats struct {
	Total int        `json:"total"`
	App   int        `json:"app"`
	Other int        `json:"other"`
	Ratio float64    `json:"ratio"`
	IPs   int        `json:"ips"`
	ASN   *asndb.ASN `json:"asn"`
}

type IPStats struct {
	Stats
	IP           net.IP `json:"ip"`
	WithHostname int    `json:"with_hostname"`
}

type NetworkStats struct {
	Stats
	Network      net.IPNet `json:"network"`
	NetworkSize  uint64    `json:"network_size"`
	NetworkRatio float64   `json:"network_ratio"`
}
