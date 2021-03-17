package data

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
}
