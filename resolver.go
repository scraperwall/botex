package botex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	// "github.com/go-redis/redis/v7"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/miekg/dns"
	nats "github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// DNSLookupError denotes a dns lookup error
const DNSLookupError = "lookup error"
const reverseLookupQueue = "reverse"
const resolveTopic = "resolv"
const resolveNamespace = "rl"

// Resolver is a DNS resolver with Redis cache
type Resolver struct {
	config          *Config
	inChan          chan *nats.Msg
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
func NewResolver(ctx context.Context, config *Config) (*Resolver, error) {
	var err error

	r := &Resolver{
		config:          config,
		ctx:             ctx,
		outChanIsClosed: false,
		inChan:          make(chan *nats.Msg),
	}

	r.natsJSONC, err = nats.NewEncodedConn(config.NatsConn, nats.JSON_ENCODER)
	if err != nil {
		return nil, err
	}

	r.subscription, err = r.natsJSONC.Conn.ChanSubscribe(resolveTopic, r.inChan)
	if err != nil {
		return nil, err
	}

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
	r.subscription.Drain()
}

func (r *Resolver) worker(id int) {
	for {
		select {
		case <-r.ctx.Done():
			log.Tracef("[Resolver::worker] resolver worker #%d exiting", id)
			break
		case msg := <-r.inChan:
			var rip IPResolv
			err := json.Unmarshal(msg.Data, &rip)
			log.Tracef("[Resolver::worker] raw resolv in %s", rip.IP)
			if err != nil {
				log.Warnf("[Resolver::worker] failed to unmarshal IPresolv (%s): %s", string(msg.Data), err)
				continue
			}
			log.Tracef("[Resolver::worker] resolving %s\n", rip.IP)
			r.reverseLookup(&rip)
		}
	}
}

func (r *Resolver) reverseLookupKey(rip *IPResolv) string {
	return fmt.Sprintf("rl:%s", rip.IP)
}

func (r *Resolver) reverseLookup(rip *IPResolv) {
	if r.outChanIsClosed {
		return
	}

	defer func() {
		// recovering from panic caused by writing to a closed channel:
		// mark the updateChan as closed
		if recover() != nil {
			r.outChanIsClosed = true
		}
	}()

	// does the reverse hostname already exist in our cache?
	//
	var err error
	ipKey := []byte(rip.IP.String())
	resData, err := r.config.KVStore.Get([]byte(resolveNamespace), ipKey)
	if err == nil {
		if len(resData) > 0 {
			rip.Host = string(resData)
			// log.Infof("kvstore %s = %s", rip.IP, rip.Host)
			r.outChan <- rip
			return
		}

		return
	}

	if err == badger.ErrKeyNotFound { // the hostname doesn't exist in the cache: resolve it via DNS
		hostname, err2 := r.reverseDNSLookup(rip.IP)

		// a DNS lookup error occured: try again
		if err2 != nil && err2.Error() != "Key not found" {
			log.Warnf("[Resolver::reverseLookup] reverse lookup %s error: %s", rip.IP, err)
			rip.Err = err.Error()
			r.Enqueue(rip)
			return
		}

		// the DNS lookup was successful: set the hostname and cache it
		log.Tracef("[Resolver::reverseLookup] dns %s -> %s", rip.IP, hostname)
		rip.Host = hostname

		err2 = r.config.KVStore.SetEx([]byte(resolveNamespace), ipKey, []byte(hostname), r.config.ResolverTTL)
		if err2 != nil {
			log.Errorf("[Resolver::reverseLookup] failed to write %s (%d) to the cache: %s", rip.IP, rip.Host, err2)
		}
	} else {
		// an error occured while retrieving the hostname from the cache: try again
		log.Tracef("[Resolver::reverseLookup] badger lookup %s serious error: %s", rip.IP, err)
		rip.Err = err.Error()
		r.Enqueue(rip)
		return
	}

	r.outChan <- rip
}

func (r *Resolver) reverseDNSLookup(ip net.IP) (string, error) {
	var hostname string

	reverse, err := dns.ReverseAddr(ip.To16().String())
	if err != nil {
		return "", err
	}

	dnsClient := new(dns.Client)
	dnsClient.Timeout = 2500 * time.Millisecond

	p := new(dns.Msg)
	p.Id = dns.Id()
	p.RecursionDesired = false
	p.SetQuestion(reverse, dns.TypePTR)

	resp, _, err := dnsClient.Exchange(p, r.config.DNSServer)
	if err != nil {
		return "", err
	}

	hostname = ip.To16().String()
	if len(resp.Answer) > 0 {
		if t, ok := resp.Answer[0].(*dns.PTR); ok {
			hostname = t.Ptr[0 : len(t.Ptr)-1]
		}
	}

	return hostname, err
}

// Enqueue queues a ReverseResolvable to be resolved
func (r *Resolver) Enqueue(rip *IPResolv) {
	rip.Tries++
	rip.Err = ""

	// stop trying if too many errors have occured
	if rip.Tries > r.config.ResolverTries {
		return
	}

	log.Tracef("[Resolver::Enqueue] *Q%d* %s\n", rip.Tries, rip.IP)

	// try to resolve the IP again
	err := r.natsJSONC.Publish(resolveTopic, rip)
	if err != nil {
		log.Errorf("[Resolver::Enqueue] error publishing: %s", err)
	}
}
