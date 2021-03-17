package data

type Blocker interface {
	BlockASN(BlockMessage)
	BlockIP(IPBlockMessage)
	BlockNetwork(NetworkBlockMessage)
}
