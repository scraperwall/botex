package botex

import (
	"context"
	"encoding/json"
	"errors"
	"net"

	log "github.com/sirupsen/logrus"
)

const blockNamespace = "bl"

// Blocklist is used to add IPs to the blocklist and to check whether an IP is blocked
type Blocklist struct {
	config *Config
	ctx    context.Context
}

// NewBlocklist creates a new Blocklist.
// The parent context and application configuration are passed on to the new instance
func NewBlocklist(ctx context.Context, config *Config) *Blocklist {
	return &Blocklist{
		config: config,
		ctx:    ctx,
	}
}

// Get retrieves an IPDetails item about a blocked IP. If the IP isn't blocked an error is returned
func (b *Blocklist) Get(ip net.IP) (*IPDetails, error) {
	data, err := b.config.KVStore.Get([]byte(blockNamespace), []byte(ip.String()))
	if err != nil {
		return nil, err
	}

	var ipd IPDetails
	err = json.Unmarshal(data, &ipd)
	return &ipd, err
}

// Block writes an IPs details to the blocklist.
// It returns an error if writing the information failed
func (b *Blocklist) Block(ipd *IPDetails) error {
	ipstr := ipd.IP.String()

	if ipd == nil {
		return errors.New("IPDetails are nil")
	}

	if ipd.IsBlocked {
		return nil
	}

	log.Infof("blocking %s  - %s (%s)", ipd.IP, ipd.Hostname, ipd.BlockReason)

	ipd.IsBlocked = true

	data, err := b.config.KVStore.Get([]byte(blockNamespace), []byte(ipstr))
	if err != nil && err != b.config.KVStore.ErrNotFound() {
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
	return b.config.KVStore.SetEx([]byte(blockNamespace), []byte(ipstr), data, b.config.BlockTTL)
}
