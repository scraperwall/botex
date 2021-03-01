package botex

import (
	"context"
	"encoding/json"
	"net"
)

const blockNamespace = "bl"

type Blocklist struct {
	config *Config
	ctx    context.Context
}

func NewBlocklist(ctx context.Context, config *Config) *Blocklist {
	return &Blocklist{
		config: config,
		ctx:    ctx,
	}
}

func (b *Blocklist) Get(ip net.IP) (*IPDetails, error) {
	data, err := b.config.KVStore.Get([]byte(blockNamespace), []byte(ip.String()))
	if err != nil {
		return nil, err
	}

	var ipd IPDetails
	err = json.Unmarshal(data, &ipd)
	return &ipd, err
}

func (b *Blocklist) Block(ipd *IPDetails) error {
	ipstr := ipd.IP.String()

	data, err := b.config.KVStore.Get([]byte(blockNamespace), []byte(ipstr))
	if err != nil {
		return err
	}

	// the IP is already blocked
	if len(data) > 0 {
		return nil
	}

	// write the IP to the kvstore
	data, err = json.Marshal(ipd)
	if err != nil {
		return err
	}
	return b.config.KVStore.Set([]byte(blockNamespace), []byte(ipstr), data)
}
