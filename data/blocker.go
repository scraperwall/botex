package data

type Blocker interface {
	BlockASN(BlockMessage) error
	BlockIP(IPBlockMessage) error
	BlockNetwork(NetworkBlockMessage) error
}
