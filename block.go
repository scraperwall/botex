package botex

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

const blockNamespace = "bl"

// Block is used to add IPs to the blocklist and to check whether an IP is blocked
type Block struct {
	networkCache      map[string]bool
	networkCacheMutex sync.RWMutex
	asnCache          map[int]bool
	asnCacheMutex     sync.RWMutex
	resources         *Resources
	blockTTL          time.Duration
	recheckChan       chan bool
	ctx               context.Context
}

// NewBlock creates a new Blocklist.
// The parent context and application configuration are passed on to the new instance
func NewBlock(ctx context.Context, recheckChan chan bool, resources *Resources, blockTTL time.Duration) *Block {
	b := Block{
		networkCache:      make(map[string]bool),
		networkCacheMutex: sync.RWMutex{},
		asnCache:          make(map[int]bool),
		asnCacheMutex:     sync.RWMutex{},
		resources:         resources,
		blockTTL:          blockTTL,
		recheckChan:       recheckChan,
		ctx:               ctx,
	}

	b.buildCache()

	go b.cleanup()

	return &b
}

// CountIPs returns the number of currently blocked IPs
func (b *Block) CountIPs() int {
	c, _ := b.resources.Store.Count(b.ipNamespace(), []byte{})
	return c
}

// BlockIP writes an IPs details to the blocklist.
// It returns an error if writing the information failed
func (b *Block) BlockIP(msg data.IPBlockMessage) error {
	if msg.IP == nil {
		return errors.New("IP is nil")
	}

	log.Infof("blocking %s  - %s (%s)", msg.IP, msg.Hostname, msg.Reason)

	data, err := b.resources.Store.Get(b.ipNamespace(), msg.IP)
	if err != nil && err != b.resources.Store.ErrNotFound() {
		return err
	}

	// the IP is already blocked
	if len(data) > 0 {
		return nil
	}

	// write the IP to the kvstore
	data, err = json.Marshal(msg)
	if err != nil {
		return err
	}
	return b.resources.Store.SetEx(b.ipNamespace(), msg.IP, data, b.blockTTL)
}

// RemoveIP removes an IP from the blocklist
func (b *Block) RemoveIP(ip net.IP) error {
	log.Tracef("removing %s from the blocklist", ip)
	return b.resources.Store.Remove(b.ipNamespace(), ip)
}

// GetIP retrieves an IPDetails item about a blocked IP. If the IP isn't blocked an error is returned
func (b *Block) GetIP(ip net.IP) (*IPDetails, error) {
	data, err := b.resources.Store.Get(b.ipNamespace(), ip)
	if err != nil {
		return nil, err
	}

	var ipd IPDetails
	err = json.Unmarshal(data, &ipd)
	return &ipd, err
}

// AllIPs returns all currently blocked IPs
func (b *Block) AllIPs() ([]*IPDetails, error) {
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

// CountNetworks returns the number of currently blocked networks
func (b *Block) CountNetworks() int {
	c, _ := b.resources.Store.Count(b.cidrNamespace(), []byte{})
	return c
}

// BlockNetwork writes an entire network to the block list
// It returns an error if writing the information failed
func (b *Block) BlockNetwork(network net.IPNet) error {
	if network.IP == nil {
		return errors.New("network is nil")
	}

	if b.isNetworkBlocked(network) {
		return nil
	}

	log.Tracef("blocking network %s", network)
	b.cacheBlockedNetwork(network)

	data, err := b.resources.Store.Get(b.cidrNamespace(), []byte(network.String()))
	if err != nil && err != b.resources.Store.ErrNotFound() {
		return err
	}

	// the network is already blocked
	if len(data) > 0 {
		return nil
	}

	// write the network to the kvstore
	data, err = json.Marshal(network)
	if err != nil {
		return err
	}
	return b.resources.Store.SetEx(b.cidrNamespace(), []byte(network.String()), data, b.blockTTL)
}

// RemoveNetwork removes a network from the blocklist
func (b *Block) RemoveNetwork(network net.IPNet) error {
	log.Tracef("removing network %s from the blocklist", network)
	return b.resources.Store.Remove(b.cidrNamespace(), []byte(network.String()))
}

// GetNetworkretrieves an IPDetails item about a blocked IP. If the IP isn't blocked an error is returned
func (b *Block) GetNetwork(network net.IPNet) (*net.IPNet, error) {
	data, err := b.resources.Store.Get(b.cidrNamespace(), []byte(network.String()))
	if err != nil {
		return nil, err
	}

	var n net.IPNet
	err = json.Unmarshal(data, &n)
	return &n, err
}

// AllNetworks returns all currently blocked IPs
func (b *Block) AllNetworks() ([]net.IPNet, error) {
	data, err := b.resources.Store.All(b.cidrNamespace(), []byte{})
	if err != nil {
		return nil, err
	}

	res := make([]net.IPNet, len(data))

	for i, jsonBytes := range data {
		res[i] = net.IPNet{}
		err = json.Unmarshal(jsonBytes, &res[i])
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

// CountASNs returns the number of currently blocked networks
func (b *Block) CountASNs() int {
	c, _ := b.resources.Store.Count(b.asnNamespace(), []byte{})
	return c
}

// BlockaASN writes an entire autonomous system (AS) to the block list
// It returns an error if writing the information failed
func (b *Block) BlockASN(asn *asndb.ASN) error {
	if asn == nil {
		return errors.New("asn is nil")
	}

	if b.isASNBlocked(asn.ASN) {
		return nil
	}

	log.Infof("blocking asn %s (%s)", asn.ASN, asn.Organization)
	b.cacheBlockedASN(asn)

	data, err := b.resources.Store.Get(b.asnNamespace(), b.asnToBytes(asn.ASN))
	if err != nil && err != b.resources.Store.ErrNotFound() {
		return err
	}

	// the network is already blocked
	if len(data) > 0 {
		return nil
	}

	// write the network to the kvstore
	data, err = json.Marshal(asn)
	if err != nil {
		return err
	}
	return b.resources.Store.SetEx(b.cidrNamespace(), b.asnToBytes(asn.ASN), data, b.blockTTL)
}

// RemoveNetwork removes a network from the blocklist
func (b *Block) RemoveASN(asn *asndb.ASN) error {
	log.Infof("removing asn %d (%s) from the blocklist", asn.ASN, asn.Organization)
	return b.resources.Store.Remove(b.asnNamespace(), b.asnToBytes(asn.ASN))
}

// GetASN retrieves a blocked ASN. If the aSN isn't blocked an error is returned
func (b *Block) GetASN(asn *asndb.ASN) (*asndb.ASN, error) {
	data, err := b.resources.Store.Get(b.ipNamespace(), b.asnToBytes(asn.ASN))
	if err != nil {
		return nil, err
	}

	var a asndb.ASN
	err = json.Unmarshal(data, &a)
	return &a, err
}

// AllASNs returns all currently blocked IPs
func (b *Block) AllASNs() ([]asndb.ASN, error) {
	data, err := b.resources.Store.All(b.asnNamespace(), []byte{})
	if err != nil {
		return nil, err
	}

	res := make([]asndb.ASN, len(data))

	for i, jsonBytes := range data {
		res[i] = asndb.ASN{}
		err = json.Unmarshal(jsonBytes, &res[i])
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

// Clear removes all currently blocked items from the store
func (b *Block) Clear() {
	b.resources.Store.Remove([]byte(blockNamespace), []byte{})
}

// cleanup unblocks every IP that has been whitelisted since it was written to the blocklist
func (b *Block) cleanup() {
	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-b.ctx.Done():
			ticker.Stop()
			ticker = nil
			return
		case <-ticker.C:
			b.buildCache()
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

func (b *Block) ipNamespace() []byte {
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, "ip"))
}

func (b *Block) asnNamespace() []byte {
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, "asn"))
}

func (b *Block) cidrNamespace() []byte {
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, "cidr"))
}

func (b *Block) isNetworkBlocked(network net.IPNet) bool {
	b.networkCacheMutex.RLock()
	defer b.networkCacheMutex.RUnlock()

	_, blocked := b.networkCache[network.String()]
	return blocked
}

func (b *Block) cacheBlockedNetwork(network net.IPNet) {
	b.networkCacheMutex.Lock()
	defer b.networkCacheMutex.Unlock()

	b.networkCache[network.String()] = true
}

func (b *Block) isASNBlocked(asn int) bool {
	b.asnCacheMutex.RLock()
	defer b.asnCacheMutex.RUnlock()

	_, blocked := b.asnCache[asn]
	return blocked
}

func (b *Block) cacheBlockedASN(asn *asndb.ASN) {
	b.asnCacheMutex.Lock()
	defer b.asnCacheMutex.Unlock()

	b.asnCache[asn.ASN] = true
}

func (b *Block) asnToBytes(asn int) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, uint64(asn))
	return data
}

func (b *Block) buildCache() error {
	cidrCache := make(map[string]bool)
	asnCache := make(map[int]bool)

	allNetworks, err := b.AllNetworks()
	if err != nil {
		return err
	}

	for _, n := range allNetworks {
		cidrCache[n.String()] = true
	}

	allASNs, err := b.AllASNs()
	if err != nil {
		return err
	}

	for _, a := range allASNs {
		asnCache[a.ASN] = true
	}

	b.asnCacheMutex.Lock()
	defer b.asnCacheMutex.Unlock()
	b.networkCacheMutex.Lock()
	defer b.networkCacheMutex.Unlock()
	b.networkCache = cidrCache
	b.asnCache = asnCache

	return nil
}
