/*
	botex - a bad bot mitigation tool by ScraperWall
	Copyright (C) 2021 ScraperWall, Tobias von Dewitz <tobias@scraperwall.com>

	This program is free software: you can redistribute it and/or modify it
	under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or (at your
	option) any later version.

	This program is distributed in the hope that it will be useful, but WITHOUT
	ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
	FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License
	for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package botex

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
	nats "github.com/nats-io/nats.go"
	"github.com/scraperwall/botex/config"
	log "github.com/sirupsen/logrus"
)

// DNSLookupError denotes a dns lookup error
const DNSLookupError = "lookup error"
const reverseLookupQueue = "reverse"
const resolveTopic = "resolv"

// const resolveNamespace = "rl"

// Resolver is a DNS resolver with Redis cache
type Resolver struct {
	config          *config.Config
	resources       *Resources
	inChan          chan *IPResolv
	outChan         chan *IPResolv
	outChanIsClosed bool
	natsJSONC       *nats.EncodedConn
	subscription    *nats.Subscription
	ctx             context.Context
}

// IPResolv contains an IP address and its corresponding reverse hostname
type IPResolv struct {
	IP     net.IP    `json:"ip"`
	Host   string    `json:"host"`
	Err    string    `json:"err"`
	Tries  int       `json:"tries"`
	TEnd   time.Time `json:"tend"`
	TStart time.Time `json:"tstart"`
}

// NewIPResolv creates a new IP that needs to be resolved
func NewIPResolv(ip net.IP) *IPResolv {
	return &IPResolv{
		IP:     ip,
		TStart: time.Now(),
		Tries:  0,
	}
}

// TimeTaken returns the amount of time it has taken to resolve the IP
func (rip *IPResolv) TimeTaken() time.Duration {
	if rip.TEnd.After(rip.TStart) {
		return rip.TEnd.Sub(rip.TStart)
	}

	return time.Now().Sub(rip.TStart)
}

// NewResolver creates a new Resolver item
func NewResolver(ctx context.Context, resources *Resources, config *config.Config) (*Resolver, error) {
	var err error

	r := &Resolver{
		config:          config,
		resources:       resources,
		ctx:             ctx,
		outChanIsClosed: false,
		inChan:          make(chan *IPResolv),
	}

	r.natsJSONC, err = nats.NewEncodedConn(resources.NatsConn, nats.JSON_ENCODER)
	if err != nil {
		return nil, err
	}

	r.subscription, err = r.natsJSONC.BindRecvChan(resolveTopic, r.inChan)
	if err != nil {
		return nil, err
	}
	r.subscription.SetPendingLimits(100000, 1024*1024*1024) // 100.000 messages or 1GB

	go r.autoClose()

	return r, nil
}

// StartWorkers starts the resolver workers. It pulls IPs from the input queue and processes them internally.
// when an IP has been resolved the result is sent over the output channel
func (r *Resolver) StartWorkers(outChan chan *IPResolv) error {
	if r.outChan != nil {
		return errors.New("workers have already been started")
	}
	r.outChan = outChan

	for i := 0; i < r.config.ResolverWorkers; i++ {
		go r.worker(i)
	}

	return nil
}

// Close cleans up all open connections and file handles
func (r *Resolver) autoClose() {
	<-r.ctx.Done()
	log.Infof("resolver stopping nats subscription")
	r.subscription.Drain()
}

func (r *Resolver) worker(id int) {
	count := 0
	for {
		select {
		case <-r.ctx.Done():
			log.Tracef("resolver worker #%d exiting", id)
			return
		case rip := <-r.inChan:
			count++
			log.Tracef("worker %d #%d - resolving %s", id, count, rip.IP)
			r.reverseLookup(rip)
		}
	}
}

func (r *Resolver) reverseLookupKey(rip *IPResolv) string {
	return fmt.Sprintf("rl:%s", rip.IP)
}

func (r *Resolver) reverseLookup(rip *IPResolv) {
	if r.outChanIsClosed {
		log.Warn("outChan is closed")
		return
	}

	// does the reverse hostname already exist in our cache?
	//
	var err error
	ipKey := []byte(rip.IP.String())
	resData, err := r.resources.Store.Get(r.resolveNamespace(ipKey))
	if err == nil {
		if len(resData) > 0 {
			rip.Host = string(resData)
			log.Tracef("kvstore %s = %s", rip.IP, rip.Host)
			r.outChan <- rip
			return
		}
	}

	if err == r.resources.Store.ErrNotFound() { // the hostname doesn't exist in the cache: resolve it via DNS
		hostname, err2 := r.reverseDNSLookup(rip.IP)

		// a DNS lookup error occured: try again
		if err2 != nil {
			rip.Err = err.Error()
			r.Resolve(rip)
			return
		}

		// the DNS lookup was successful: set the hostname and cache it
		log.Tracef("dns %s -> %s", rip.IP, hostname)
		rip.Host = hostname
		r.outChan <- rip

		err2 = r.resources.Store.SetEx(r.resolveNamespace(ipKey), []byte(hostname), r.config.ResolverTTL)
		if err2 != nil {
			log.Errorf("failed to write %s (%s) to the cache: %s", rip.IP, rip.Host, err2)
		}
	} else {
		// an error occured while retrieving the hostname from the cache: try again
		log.Tracef("serious badger lookup error: %s", err)
		// rip.Err = err.Error()
		r.Resolve(rip)
		return
	}
}

type resolvResult struct {
	Hostname string
	Error    error
}

func (r *Resolver) reverseDNSLookup(ip net.IP) (string, error) {
	var hostname string

	if ip == nil {
		return "", fmt.Errorf("ip is nil")
	}

	reverse, err := dns.ReverseAddr(ip.To16().String())
	if err != nil {
		return "", err
	}

	dnsClient := new(dns.Client)
	// dnsClient.Timeout = 2500 * time.Millisecond

	p := new(dns.Msg)
	p.Id = dns.Id()
	p.RecursionDesired = false
	p.SetQuestion(reverse, dns.TypePTR)

	resp, _, err := dnsClient.Exchange(p, r.config.DNSServer)
	if err != nil {
		log.Warnf("dns exchange error for %s: %s", ip, err)
		return "", err
	}

	hostname = ip.To16().String()
	if len(resp.Answer) > 0 {
		if t, ok := resp.Answer[0].(*dns.PTR); ok {
			hostname = t.Ptr[0 : len(t.Ptr)-1]
		}
	} else {
		log.Tracef("dns answer for %s too small. Using %s", ip, ip)
	}

	return hostname, nil
}

// Resolve queues a ReverseResolvable to be resolved
func (r *Resolver) Resolve(rip *IPResolv) {
	rip.Tries++
	rip.Err = ""

	// stop trying if too many errors have occured
	if rip.Tries > r.config.ResolverTries {
		log.Tracef("max tries (%d) reached for %d", r.config.ResolverTries, rip.Tries)
		return
	}

	log.Tracef("try #%d %s", rip.Tries, rip.IP)

	// try to resolve the IP again
	err := r.natsJSONC.Publish(resolveTopic, rip)
	if err != nil {
		log.Errorf("error publishing: %s", err)
	}
}

func (r *Resolver) resolveNamespace(ip []byte) []byte {
	return []byte(fmt.Sprintf("%s:%s:%s", "rl", "ip", ip))
}
