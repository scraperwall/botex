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
	networkCache           map[string]bool
	networkCacheMutex      sync.RWMutex
	asnCache               map[int]bool
	asnCacheMutex          sync.RWMutex
	resources              *Resources
	blockTTL               time.Duration
	unblockWhitelistedChan chan bool
	blockASNNetworkChan    chan bool
	ctx                    context.Context
}

// NewBlock creates a new Blocklist.
// The parent context and application configuration are passed on to the new instance
func NewBlock(ctx context.Context, wlChan, blChan chan bool, resources *Resources, blockTTL time.Duration) *Block {
	b := Block{
		networkCache:           make(map[string]bool),
		networkCacheMutex:      sync.RWMutex{},
		asnCache:               make(map[int]bool),
		asnCacheMutex:          sync.RWMutex{},
		resources:              resources,
		blockTTL:               blockTTL,
		unblockWhitelistedChan: wlChan,
		blockASNNetworkChan:    blChan,
		ctx:                    ctx,
	}

	b.buildCache()

	go b.cleanup()

	return &b
}

// CountIPs returns the number of currently blocked IPs
func (b *Block) CountIPs() int {
	c, _ := b.resources.Store.Count(b.IPNamespace([]byte{}))
	return c
}

// BlockIP writes an IPs details to the blocklist.
// It returns an error if writing the information failed
func (b *Block) BlockIP(msg data.IPBlockMessage) error {
	if msg.IP == nil {
		return errors.New("IP is nil")
	}

	data, err := b.resources.Store.Get(b.IPNamespace(msg.IP))
	if err != nil && err != b.resources.Store.ErrNotFound() {
		return err
	}

	// the IP is already blocked
	if len(data) > 0 {
		return nil
	}

	log.Tracef("blocking %s  - %s (%s)", msg.IP, msg.Hostname, msg.Reason)

	// write the IP to the kvstore
	data, err = json.Marshal(msg)
	if err != nil {
		return err
	}
	return b.resources.Store.SetEx(b.IPNamespace(msg.IP), data, b.blockTTL)
}

// RemoveIP removes an IP from the blocklist
func (b *Block) RemoveIP(ip net.IP) error {
	log.Tracef("removing %s from the blocklist", ip)
	return b.resources.Store.Remove(b.IPNamespace(ip))
}

// GetIP retrieves an IPDetails item about a blocked IP. If the IP isn't blocked an error is returned
func (b *Block) GetIP(ip net.IP) (*IPDetails, error) {
	data, err := b.resources.Store.Get(b.IPNamespace(ip))
	if err != nil {
		return nil, err
	}

	var ipd IPDetails
	err = json.Unmarshal(data, &ipd)
	return &ipd, err
}

func (b *Block) BlockedIPs() []data.IPBlockMessage {
	res := make([]data.IPBlockMessage, 0)

	b.resources.Store.Each(b.IPNamespace([]byte{}), func(jsonData []byte) {
		var msg data.IPBlockMessage
		err := json.Unmarshal(jsonData, &msg)
		if err != nil {
			log.Warn(err)
			return
		}
		res = append(res, msg)
	})

	return res
}

// AllIPs returns all currently blocked IPs
func (b *Block) AllIPs() ([]*IPDetails, error) {
	data, err := b.resources.Store.All(b.IPNamespace([]byte{}))
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
	c, _ := b.resources.Store.Count(b.CIDRNamespace(""))
	return c
}

// BlockNetwork writes an entire network to the block list
// It returns an error if writing the information failed
func (b *Block) BlockNetwork(msg data.NetworkBlockMessage) error {
	if msg.Network == nil {
		return errors.New("network is nil")
	}

	if b.IsBlockedByASN(msg.ASN) {
		return nil
	}

	log.Tracef("blocking network %s (%s): total: %d, app: %d, ratio: %.2f", msg.Network, msg.ASN.Organization, msg.Total, msg.App, msg.Ratio)
	b.cacheBlockedNetwork(msg.Network)

	data, err := b.resources.Store.Get(b.CIDRNamespace(msg.Network.String()))
	if err != nil && err != b.resources.Store.ErrNotFound() {
		return err
	}

	// the network is already blocked
	if len(data) > 0 {
		return nil
	}

	// write the network to the kvstore
	data, err = json.Marshal(msg)
	if err != nil {
		return err
	}

	err = b.resources.Store.SetEx(b.CIDRNamespace(msg.Network.String()), data, b.blockTTL)
	return err
}

// RemoveNetwork removes a network from the blocklist
func (b *Block) RemoveNetwork(network net.IPNet) error {
	log.Tracef("removing network %s from the blocklist", network)
	return b.resources.Store.Remove(b.CIDRNamespace(network.String()))
}

// GetNetworkretrieves an IPDetails item about a blocked IP. If the IP isn't blocked an error is returned
func (b *Block) GetNetwork(network net.IPNet) (*net.IPNet, error) {
	data, err := b.resources.Store.Get(b.CIDRNamespace(network.String()))
	if err != nil {
		return nil, err
	}

	var n net.IPNet
	err = json.Unmarshal(data, &n)
	return &n, err
}

// AllNetworks returns all currently blocked IPs
func (b *Block) AllNetworks() ([]net.IPNet, error) {
	data, err := b.resources.Store.All(b.CIDRNamespace(""))
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

func (b *Block) BlockedNetworks() []data.NetworkBlockMessage {
	res := make([]data.NetworkBlockMessage, 0)

	b.resources.Store.Each(b.CIDRNamespace(""), func(jsonData []byte) {
		var msg data.NetworkBlockMessage
		err := json.Unmarshal(jsonData, &msg)
		if err != nil {
			log.Warn(err)
			return
		}
		res = append(res, msg)
	})

	return res
}

// CountASNs returns the number of currently blocked networks
func (b *Block) CountASNs() int {
	c, _ := b.resources.Store.Count(b.ASNNamespace(-1))
	return c
}

// BlockaASN writes an entire autonomous system (AS) to the block list
// It returns an error if writing the information failed
func (b *Block) BlockASN(msg data.BlockMessage) error {
	if msg.ASN == nil {
		return errors.New("asn is nil")
	}

	if b.IsBlockedByASN(msg.ASN) {
		return nil
	}

	log.Infof("blocking asn %d (%s): total: %d, app: %d, ratio: %.2f", msg.ASN.ASN, msg.ASN.Organization, msg.Total, msg.App, msg.Ratio)
	b.cacheBlockedASN(msg.ASN)

	data, err := b.resources.Store.Get(b.ASNNamespace(msg.ASN.ASN))
	if err != nil && err != b.resources.Store.ErrNotFound() {
		return err
	}

	// the network is already blocked
	if len(data) > 0 {
		return nil
	}

	// write the network to the kvstore
	data, err = json.Marshal(msg)
	if err != nil {
		return err
	}

	err = b.resources.Store.SetEx(b.ASNNamespace(msg.ASN.ASN), data, b.blockTTL)
	if err != nil {
		return err
	}
	// b.blockASNNetworkChan <- true

	return nil
}

// RemoveNetwork removes a network from the blocklist
func (b *Block) RemoveASN(asn *asndb.ASN) error {
	log.Infof("removing asn %d (%s) from the blocklist", asn.ASN, asn.Organization)
	return b.resources.Store.Remove(b.ASNNamespace(asn.ASN))
}

// GetASN retrieves a blocked ASN. If the aSN isn't blocked an error is returned
func (b *Block) GetASN(asn *asndb.ASN) (*asndb.ASN, error) {
	data, err := b.resources.Store.Get(b.ASNNamespace(asn.ASN))
	if err != nil {
		return nil, err
	}

	var a asndb.ASN
	err = json.Unmarshal(data, &a)
	return &a, err
}

// AllASNs returns all currently blocked IPs
func (b *Block) AllASNs() ([]asndb.ASN, error) {
	data, err := b.resources.Store.All(b.ASNNamespace(-1))
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

func (b *Block) BlockedASNs() []data.BlockMessage {
	res := make([]data.BlockMessage, 0)

	b.resources.Store.Each(b.ASNNamespace(-1), func(jsonData []byte) {
		var msg data.BlockMessage
		err := json.Unmarshal(jsonData, &msg)
		if err != nil {
			log.Warn(err)
			return
		}
		res = append(res, msg)
	})

	return res
}

func (b *Block) IsBlockedByASN(asn *asndb.ASN) bool {
	if _, err := b.GetASN(asn); err == nil {
		return true
	}

	if _, err := b.GetNetwork(*asn.Network); err == nil {
		return true
	}

	return false
}

func (b *Block) IsBlocked(ip net.IP, asn *asndb.ASN) bool {
	if _, err := b.GetIP(ip); err == nil {
		return true
	}

	if b.IsBlockedByASN(asn) {
		return true
	}

	return false
}

// Clear removes all currently blocked items from the store
func (b *Block) Clear() {
	b.resources.Store.Remove([]byte(blockNamespace))
	c, err := b.resources.Store.Count([]byte(blockNamespace))
	if err != nil {
		log.Warn(err)
		return
	}
	log.Infof("blocked items cleared: %d items in the database", c)
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
		case <-b.unblockWhitelistedChan:
			var ipd IPDetails
			b.resources.Store.Each(b.IPNamespace([]byte{}), func(v []byte) {
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

func (b *Block) CheckBlocked() {
	b.blockASNNetworkChan <- true
}

func (b *Block) IPNamespace(ip []byte) []byte {
	return []byte(fmt.Sprintf("%s:%s:%s", blockNamespace, "ip", ip))
}

func (b *Block) ASNNamespace(asn int) []byte {
	if asn > 0 {
		return []byte(fmt.Sprintf("%s:%s:%d", blockNamespace, "asn", asn))
	}
	return []byte(fmt.Sprintf("%s:%s", blockNamespace, "asn"))
}

func (b *Block) CIDRNamespace(cidr string) []byte {
	return []byte(fmt.Sprintf("%s:%s:%s", blockNamespace, "cidr", cidr))
}

func (b *Block) cacheBlockedNetwork(network *net.IPNet) {
	b.networkCacheMutex.Lock()
	defer b.networkCacheMutex.Unlock()

	b.networkCache[network.String()] = true
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
