package data

type Blocker interface {
	BlockASN(ASNBlockMessage)
	BlockIP(IPBlockMessage)
	BlockNetwork(NetworkBlockMessage)
}
