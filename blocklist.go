package botex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

const blockNamespace = "bl"
const blockByIPNamespace = "ip"
const blockByASNNamespace = "asn"
const blockByCIDRNamespace = "cidr"

// Blocklist is used to add IPs to the blocklist and to check whether an IP is blocked
type Blocklist struct {
	resources   *Resources
	blockTTL    time.Duration
	recheckChan chan bool
	ctx         context.Context
}

// NewBlocklist creates a new Blocklist.
// The parent context and application configuration are passed on to the new instance
func NewBlocklist(ctx context.Context, recheckChan chan bool, resources *Resources, blockTTL time.Duration) *Blocklist {
	bl := &Blocklist{
		resources:   resources,
		blockTTL:    blockTTL,
		recheckChan: recheckChan,
		ctx:         ctx,
	}

	go bl.recheck()

	return bl
}

// Count returns the number of currently blocked IPs
func (b *Blocklist) Count() int {
	c, _ := b.resources.Store.Count([]byte(blockNamespace), []byte{})
	return c
}

// GetIP retrieves an IPDetails item about a blocked IP. If the IP isn't blocked an error is returned
func (b *Blocklist) GetIP(ip net.IP) (*IPDetails, error) {
	data, err := b.resources.Store.Get(b.ipNamespace(), ip)
	if err != nil {
		return nil, err
	}

	var ipd IPDetails
	err = json.Unmarshal(data, &ipd)
	return &ipd, err
}

// All returns all currently blocked IPs
func (b *Blocklist) All() ([]*IPDetails, error) {
	data, err := b.resources.Store.All(b.ipNamespace(), []byte{})
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

// BlockIP writes an IPs details to the blocklist.
// It returns an error if writing the information failed
func (b *Blocklist) BlockIP(ipd *IPDetails) error {
	if ipd == nil {
		return errors.New("IPDetails are nil")
	}

	if ipd.IsBlocked {
		return nil
	}

	log.Infof("blocking %s  - %s (%s)", ipd.IP, ipd.Hostname, ipd.BlockReason)

	ipd.IsBlocked = true

	data, err := b.resources.Store.Get(b.ipNamespace(), ipd.IP)
	if err != nil && err != b.resources.Store.ErrNotFound() {
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
	return b.resources.Store.SetEx([]byte(blockNamespace), ipd.IP, data, b.blockTTL)
}

// RemoveIP removes an IP from the blocklist
func (b *Blocklist) RemoveIP(ip net.IP) error {
	log.Infof("removing %s from the blocklist", ip)
	return b.resources.Store.Remove([]byte(blockNamespace), ip)
}

// recheck unblocks every IP that has been whitelisted since it was written to the blocklist
func (b *Blocklist) recheck() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.recheckChan:
			var ipd IPDetails
			b.resources.Store.Each(b.ipNamespace(), []byte{}, func(v []byte) {
				err := json.Unmarshal(v, &ipd)
				if err != nil {
					log.Warn("failed to unmarshal blocked IP for blocklist recheck")
				}

				if wl, _ := b.resources.Whitelist.IsWhitelisted(&ipd); wl {
					b.RemoveIP(ipd.IP)
				}
			})
		}
	}
}

func (b *Blocklist) ipNamespace() []byte {
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, blockByIPNamespace))
}

func (b *Blocklist) asbNamespace() []byte {
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, blockByASNNamespace))
}

func (b *Blocklist) cidrNamespace() []byte {
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, blockByCIDRNamespace))
}
