package botex

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml"
	log "github.com/sirupsen/logrus"
	fsnotify "gopkg.in/fsnotify.v1"
)

// Whitelist is the structure that contains all whitelist logic
type Whitelist struct {
	regexps              *WhitelistRegexps
	mutex                sync.RWMutex
	asns                 map[int]string
	cidrs                []CIDRWhitelistRule
	serverHostPattern    *regexp.Regexp
	serverPathPattern    *regexp.Regexp
	useragentPattern     *regexp.Regexp
	UpdatedAt            time.Time
	appConfig            *Config
	rules                WhitelistRules
	rulesWatcher         *fsnotify.Watcher
	blocklistRecheckChan chan bool
	ctx                  context.Context
}

// WhitelistRules contains the Whitelist configuration
type WhitelistRules struct {
	IP         []WhitelistRule
	CIDR       []WhitelistRule
	ClientHost []WhitelistRule
	Org        []WhitelistRule
	ASN        []WhitelistRule
	Useragent  []WhitelistRule
	ServerHost []WhitelistRule
	ServerPath []WhitelistRule
}

// WhitelistRule represents a single Whitelist rule
type WhitelistRule struct {
	Pattern     string
	Description string
	Regexp      *regexp.Regexp
}

// CIDRWhitelistRule is a CIDR whitelist rule
type CIDRWhitelistRule struct {
	Network     *net.IPNet
	Description string
}

// WhitelistRegexps contains whitelist regexps for various data fields
type WhitelistRegexps struct {
	IP        *regexp.Regexp
	Host      *regexp.Regexp
	Org       *regexp.Regexp
	ASN       *regexp.Regexp
	Target    *regexp.Regexp
	URL       *regexp.Regexp
	UserAgent *regexp.Regexp
}

// NewWhitelist creates a new whitelist data structure from MongoDB
func NewWhitelist(ctx context.Context, blocklistRecheckChan chan bool, config *Config) (*Whitelist, error) {
	wl := &Whitelist{
		ctx:                  ctx,
		appConfig:            config,
		mutex:                sync.RWMutex{},
		asns:                 make(map[int]string),
		blocklistRecheckChan: blocklistRecheckChan,
		cidrs:                make([]CIDRWhitelistRule, 0),
	}

	go wl.cleanup()

	wl.Load()
	wl.reloadOnConfigChanges()

	return wl, nil
}

// IsURLWhitelisted determines whether a requested URL is whitelisted and should not be processed
func (wl *Whitelist) IsURLWhitelisted(url string) bool {
	wl.mutex.Lock()
	whitelisted := wl.regexps.URL.MatchString(url)
	wl.mutex.Unlock()

	return whitelisted
}

// IsCIDRWhitelisted determines if the given IP is part of a network specified by the cidr
func (wl *Whitelist) IsCIDRWhitelisted(ip net.IP) (bool, string) {
	wl.mutex.RLock()
	defer wl.mutex.RUnlock()

	for _, cidrwl := range wl.cidrs {
		if cidrwl.Network.Contains(ip) {
			return true, cidrwl.Description
		}
	}

	return false, ""
}

// Load retrieves all whitelist rules from the configuration filee
func (wl *Whitelist) Load() error {
	var rules WhitelistRules

	fh, err := os.Open(wl.appConfig.WhitelistTOML)
	if err != nil {
		return err
	}
	defer fh.Close()

	configBytes, err := ioutil.ReadAll(fh)
	if err != nil {
		return err
	}

	err = toml.Unmarshal(configBytes, &rules)
	if err != nil {
		return err
	}

	// ASNs
	asns := make(map[int]string)
	for i, r := range rules.ASN {
		asn, err := strconv.ParseInt(r.Pattern, 10, 64)
		if err != nil {
			return fmt.Errorf("%s is not a valid ASN", r.Pattern)
		}
		asns[int(asn)] = r.Description

		rules.ASN[i].Regexp, err = regexp.Compile(r.Pattern)
		if err != nil {
			return fmt.Errorf("can't parse ASN whitelist rule %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// Client (remote) hostname
	for i, r := range rules.ClientHost {
		rules.ClientHost[i].Regexp, err = regexp.Compile(fmt.Sprintf("^%s$", r.Pattern))
		if err != nil {
			return fmt.Errorf("can't parse whitelist client host regexp %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// IPs
	for i, r := range rules.IP {
		rules.IP[i].Regexp, err = regexp.Compile(fmt.Sprintf("^%s$", r.Pattern))
		if err != nil {
			return fmt.Errorf("can't parse whitelist IP regexp %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// Oganizations
	for i, r := range rules.Org {
		rules.Org[i].Regexp, err = regexp.Compile(fmt.Sprintf("^%s$", r.Pattern))
		if err != nil {
			return fmt.Errorf("can't parse whitelist org regexp %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// Server Hostname
	shostPatterns := make([]string, len(rules.ServerHost))
	for i, r := range rules.ServerHost {
		shostPatterns[i] = r.Pattern
		rules.ServerHost[i].Regexp, err = regexp.Compile(fmt.Sprintf("^%s$", r.Pattern))
		if err != nil {
			return fmt.Errorf("can't parse whitelist server host regexp %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// Server Path
	spathPatterns := make([]string, len(rules.ServerPath))
	for i, r := range rules.ServerPath {
		spathPatterns[i] = r.Pattern
		rules.ServerPath[i].Regexp, err = regexp.Compile(fmt.Sprintf("^%s$", r.Pattern))
		if err != nil {
			return fmt.Errorf("can't parse whitelist server path regexp %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// Useragent
	uaPatterns := make([]string, len(rules.Useragent))
	for i, r := range rules.Useragent {
		uaPatterns[i] = r.Pattern
		rules.ServerPath[i].Regexp, err = regexp.Compile(fmt.Sprintf("^%s$", r.Pattern))
		if err != nil {
			return fmt.Errorf("can't parse whitelist useragent regexp %s (%s): %s", r.Pattern, r.Description, err)
		}
	}

	// CIDR
	cidrRules := make([]CIDRWhitelistRule, len(rules.CIDR))
	for i, r := range rules.CIDR {
		cidr := strings.TrimSpace(strings.Replace(r.Pattern, `\`, "", -1))
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("can't parse whitelist CIDR regexp %s (%s): %s", r.Pattern, r.Description, err)
		}

		cidrRules[i] = CIDRWhitelistRule{
			Network:     network,
			Description: r.Description,
		}
	}

	wl.mutex.Lock()
	wl.asns = asns
	wl.rules = rules
	wl.cidrs = cidrRules
	wl.serverHostPattern = regexp.MustCompile(fmt.Sprintf("^(%s)$", strings.Join(shostPatterns, "|")))
	wl.serverPathPattern = regexp.MustCompile(fmt.Sprintf("^(%s)$", strings.Join(spathPatterns, "|")))
	wl.useragentPattern = regexp.MustCompile(fmt.Sprintf("^(%s)$", strings.Join(uaPatterns, "|")))
	wl.UpdatedAt = time.Now()
	wl.mutex.Unlock()

	// remove any blocked IPs that might have been whitelisted by the recent changes
	wl.blocklistRecheckChan <- true
	log.Infof("whitelist rules loaded successfully")
	return nil
}

// IsWhitelistedByServerHost checks whether an incoming request is whitelisted by a server hostname rule
func (wl *Whitelist) IsWhitelistedByServerHost(r *Request) bool {
	wl.mutex.RLock()
	defer wl.mutex.RUnlock()

	return wl.serverHostPattern.MatchString(r.Host)
}

// IsWhitelistedByServerPath checks whether an incoming request is whitelisted by a server path (URL) rule
func (wl *Whitelist) IsWhitelistedByServerPath(r *Request) bool {
	wl.mutex.RLock()
	defer wl.mutex.RUnlock()

	return wl.serverPathPattern.MatchString(r.URL)
}

// IsWhitelistedByUseragent checks whether an incoming request is whitelisted by a useragent rule
func (wl *Whitelist) IsWhitelistedByUseragent(r *Request) bool {
	wl.mutex.RLock()
	defer wl.mutex.RUnlock()

	return wl.useragentPattern.MatchString(r.URL)
}

// IsWhitelisted determines whether the IP represented by ipd is whitelisted. The method returns whether
// the IP is whitelisted and the description of the rule that matched
func (wl *Whitelist) IsWhitelisted(ipd *IPDetails) (whitelisted bool, description string) {
	wl.mutex.RLock()
	defer wl.mutex.RUnlock()

	if descr, ok := wl.asns[ipd.ASN.ASN]; ok {
		return true, descr
	}

	for _, r := range wl.rules.ClientHost {
		if r.Regexp.MatchString(ipd.Hostname) {
			return true, r.Description
		}
	}

	for _, r := range wl.cidrs {
		if r.Network.Contains(ipd.IP) {
			return true, r.Description
		}
	}

	for _, r := range wl.rules.IP {
		if r.Regexp.MatchString(ipd.IP.String()) {
			return true, r.Description
		}
	}

	for _, r := range wl.rules.Org {
		if r.Regexp.MatchString(ipd.ASN.Organization) {
			return true, r.Description
		}
	}

	return false, ""
}

func (wl *Whitelist) reloadOnConfigChanges() {
	if wl.rulesWatcher != nil {
		log.Warn("whitelist rules file watcher already exists")
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Errorf("couldn't start whitelist config fsnotify watcher: %s", err)
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					wl.Load()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Warn("whitelist rule watcher error event: %s", err)
			}
		}
	}()

	err = watcher.Add(wl.appConfig.WhitelistTOML)
	if err != nil {
		log.Fatal(err)
	}
}

func (wl *Whitelist) cleanup() {
	<-wl.ctx.Done()
	log.Infof("whitelist config watcher exiting")
	//wl.rulesWatcher.Close()
}
