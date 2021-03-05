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
	config      *Config
	recheckChan chan bool
	ctx         context.Context
}

// NewBlocklist creates a new Blocklist.
// The parent context and application configuration are passed on to the new instance
func NewBlocklist(ctx context.Context, recheckChan chan bool, config *Config) *Blocklist {
	bl := &Blocklist{
		config:      config,
		recheckChan: recheckChan,
		ctx:         ctx,
	}

	go bl.recheck()

	return bl
}

// Count returns the number of currently blocked IPs
func (b *Blocklist) Count() int {
	c, _ := b.config.KVStore.Count([]byte(blockNamespace), []byte{})
	return c
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

// All returns all currently blocked IPs
func (b *Blocklist) All() ([]*IPDetails, error) {
	data, err := b.config.KVStore.All([]byte(blockNamespace), []byte{})
	if err != nil {
		return nil, err
	}

	res := make([]*IPDetails, len(data))

	for i, jsonBytes := range data {
		res[i] = new(IPDetails)
		err = json.Unmarshal(jsonBytes, &res[i])
		if err != nil {
			return nil, err
		}
	}

	return res, nil
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

// Remove removes an IP from the blocklist
func (b *Blocklist) Remove(ip net.IP) error {
	log.Infof("removing %s from the blocklist", ip)
	return b.config.KVStore.Remove([]byte(blockNamespace), []byte(ip.String()))
}

// echeck unblocks every IP that has been whitelisted since it was written to the blocklist
func (b *Blocklist) recheck() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.recheckChan:
			var ipd IPDetails
			b.config.KVStore.Each([]byte(blockNamespace), []byte{}, func(v []byte) {
				err := json.Unmarshal(v, &ipd)
				if err != nil {
					log.Warn("failed to unmarshal blocked IP for blocklist recheck")
				}

				if wl, _ := b.config.Whitelist.IsWhitelisted(&ipd); wl {
					b.Remove(ipd.IP)
				}
			})
		}
	}

}
