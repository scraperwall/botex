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
	IsBlockedByASN(asn *asndb.ASN) bool
	CheckBlocked()
}
