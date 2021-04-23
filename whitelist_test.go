package botex

import (
	"context"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/scraperwall/asndb/v2"
	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

const whitelistTestTOML = `
[[IP]]
Pattern = "1\\.1\\..+"
Description = "Cloudflare"

[[IP]]
Pattern = "faa0::1"
Description = "Test IPv6"

[[CIDR]]
Pattern = "45.153.12.0/24"
Description = "Test CIDR"

[[CIDR]]
Pattern = "2663:c931:2e77:edc2::/64"
Description = "Test IPv6 CIDR"

[[ClientHost]]
Pattern = ".+\\.googlebot\\.com"
Description = "Googlebot"

[[ClientHost]]
Pattern = ".+\\.search\\.msn\\.com"
Description = "Bing Bot"

[[Org]]
Pattern = "APPLE-ENGINEERING"
Description = "Apple Bot"

[[ASN]]
Pattern = "714"
Description = "Apple Bot by ASN"

[[ServerHost]]
Pattern = "test.scw.systems"
Description = "test"

[[ServerPath]]
Pattern = "/api/.+"
Description = "API"

[[Useragent]]
Pattern = "testUA/1\\.0"
Description = "Test Useragent"
`

func TestWhitelistLoad(t *testing.T) {
	fh, err := ioutil.TempFile("", "botex-whitelist")
	if err != nil {
		t.Error(err)
	}
	_, err = fh.WriteString(whitelistTestTOML)
	if err != nil {
		t.Error(err)
	}

	config := config.Config{
		WhitelistTOML: fh.Name(),
	}

	fh.Close()
	defer os.Remove(fh.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recheckChan := make(chan bool, 100)
	wl, err := NewWhitelist(ctx, recheckChan, &config)
	if err != nil {
		t.Errorf("failed to create Whitelist: %s", err)
	}

	if len(wl.rules.IP) != 2 {
		log.Errorf("there are %d IP entries but 2 are expected", len(wl.rules.IP))
	}

	if len(wl.rules.CIDR) != 2 {
		log.Errorf("there are %d CIDR entries but 2 are expected", len(wl.rules.CIDR))
	}

	if len(wl.rules.ClientHost) != 2 {
		log.Errorf("there are %d ClientHost entries but 2 are expected", len(wl.rules.ClientHost))
	}

	if len(wl.rules.Org) != 1 {
		log.Errorf("there are %d Org entries but 1 is expected", len(wl.rules.Org))
	}

	if len(wl.rules.ASN) != 1 {
		log.Errorf("there are %d ASN entries but 1 is expected", len(wl.rules.ASN))
	}

	if len(wl.rules.ServerHost) != 1 {
		log.Errorf("there are %d ServerHost entries but 1 is expected", len(wl.rules.ServerHost))
	}

	if len(wl.rules.ServerPath) != 1 {
		log.Errorf("there are %d ServerPath entries but 1 is expected", len(wl.rules.ServerPath))
	}

	if len(wl.rules.Useragent) != 1 {
		log.Errorf("there are %d Useragent entries but 1 is expected", len(wl.rules.Useragent))
	}

	// ASN
	//
	ipFrom := net.ParseIP("17.64.0.0")
	ipTo := net.ParseIP("17.95.255.255")
	isWL, msg := wl.IsWhitelisted(&IPDetails{
		ASN: &asndb.ASN{
			From:         &ipFrom,
			To:           &ipTo,
			Cidr:         "17.64.0.0/11",
			ASN:          714,
			Organization: "None",
		},
	})
	if !isWL {
		log.Error("ASN 714 should be whitelisted")
	}

	if msg != "Apple Bot by ASN" {
		log.Errorf("whitelist message for ASN 714 should be 'Apple Bot by ASN' but is '%s'", msg)
	}

	// ASN negative
	//
	isWL, _ = wl.IsWhitelisted(&IPDetails{
		ASN: &asndb.ASN{
			From:         &ipFrom,
			To:           &ipTo,
			Cidr:         "17.64.0.0/11",
			ASN:          715,
			Organization: "None",
		},
	})
	if isWL {
		log.Error("ASN 715 should not be whitelisted")
	}

	// IPv6
	//
	ip := net.ParseIP("faa0::1")
	isWL, msg = wl.IsWhitelisted(&IPDetails{
		IP: ip,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "faa0::1/64",
			ASN:          1,
			Organization: "None",
		},
	})

	if !isWL {
		t.Errorf("IP %s should be whitelisted", ip)
	}

	if msg != "Test IPv6" {
		t.Errorf("whitelist message for IP %s should be 'Test IPv6' but is '%s'", ip, msg)
	}

	// IPv6 negative
	//
	ip = net.ParseIP("faab::3")
	isWL, _ = wl.IsWhitelisted(&IPDetails{
		IP: ip,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "faab::1/64",
			ASN:          1,
			Organization: "None",
		},
	})

	if isWL {
		t.Errorf("IP %s should not be whitelisted", ip)
	}

	// IPv4
	//
	ip = net.ParseIP("1.1.1.1")
	isWL, msg = wl.IsWhitelisted(&IPDetails{
		IP: ip,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "1.1.0.0/16",
			ASN:          1,
			Organization: "Cloudflare",
		},
	})

	if !isWL {
		t.Errorf("IP %s should be whitelisted", ip)
	}

	if msg != "Cloudflare" {
		t.Errorf("whitelist message for IP %s should be 'Cloudflare' but is '%s'", ip, msg)
	}

	// IPv4 negative
	//
	ip = net.ParseIP("1.2.1.1")
	isWL, _ = wl.IsWhitelisted(&IPDetails{
		IP: ip,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "1.2.0.0/16",
			ASN:          1,
			Organization: "None",
		},
	})

	if isWL {
		t.Errorf("IP %s should not be whitelisted", ip)
	}

	// CIDR
	//
	ip = net.ParseIP("45.153.12.41")
	isWL, msg = wl.IsWhitelisted(&IPDetails{
		IP: ip,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "45.153.12.0/24",
			ASN:          1,
			Organization: "None",
		},
	})

	if !isWL {
		t.Errorf("IP %s should be whitelisted", ip)
	}

	if msg != "Test CIDR" {
		t.Errorf("whitelist message for IP %s should be 'Test CIDR' but is '%s'", ip, msg)
	}

	// CIDR negative
	//
	ip = net.ParseIP("45.153.13.41")
	isWL, _ = wl.IsWhitelisted(&IPDetails{
		IP: ip,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "45.153.13.0/24",
			ASN:          1,
			Organization: "None",
		},
	})

	if isWL {
		t.Errorf("IP %s should not be whitelisted", ip)
	}

	// Client Hostname
	//
	ip = net.ParseIP("5.5.5.5")
	host := "test.googlebot.com"
	isWL, msg = wl.IsWhitelisted(&IPDetails{
		IP:       ip,
		Hostname: host,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "5.5.5.5/24",
			ASN:          1,
			Organization: "None",
		},
	})

	if !isWL {
		t.Errorf("Hostname %s should be whitelisted", host)
	}

	if msg != "Googlebot" {
		t.Errorf("whitelist message for Host %s should be 'Googlebot' but is '%s'", host, msg)
	}

	// Client Hostname negative
	//
	ip = net.ParseIP("5.5.5.5")
	host = "test.nogooglebot.com"
	isWL, _ = wl.IsWhitelisted(&IPDetails{
		IP:       ip,
		Hostname: host,
		ASN: &asndb.ASN{
			From:         &ip,
			To:           &ip,
			Cidr:         "5.5.5.5/24",
			ASN:          1,
			Organization: "None",
		},
	})

	if isWL {
		t.Errorf("Hostname %s should not be whitelisted", host)
	}

	// Organization
	//
	ipFrom = net.ParseIP("17.64.0.0")
	ipTo = net.ParseIP("17.95.255.255")
	isWL, msg = wl.IsWhitelisted(&IPDetails{
		ASN: &asndb.ASN{
			From:         &ipFrom,
			To:           &ipTo,
			Cidr:         "17.64.0.0/11",
			ASN:          715,
			Organization: "APPLE-ENGINEERING",
		},
	})
	if !isWL {
		log.Error("Organization APPLE-ENGINEERING should be whitelisted")
	}

	if msg != "Apple Bot" {
		log.Errorf("whitelist message for Organization 'APPLE-ENGINEERING' should be 'Apple Bot' but is '%s'", msg)
	}

	isWL, _ = wl.IsWhitelisted(&IPDetails{
		ASN: &asndb.ASN{
			From:         &ipFrom,
			To:           &ipTo,
			Cidr:         "17.64.0.0/11",
			ASN:          715,
			Organization: "PEAR-ENGINEERING",
		},
	})
	if isWL {
		log.Error("Organization PEAR-ENGINEERING should not be whitelisted")
	}

	// Server Hostname
	//
	host = "test.scw.systems"
	isWL = wl.IsWhitelistedByServerHost(&data.Request{
		Host: host,
	})

	if !isWL {
		t.Errorf("Server Hostname %s should be whitelisted", host)
	}

	// Server Hostname negative
	//
	host = "notest.scw.systems"
	isWL = wl.IsWhitelistedByServerHost(&data.Request{
		Host: host,
	})

	if isWL {
		t.Errorf("Server Hostname %s should not be whitelisted", host)
	}

	// Server Path
	//
	path := "/api/test"
	isWL = wl.IsWhitelistedByServerPath(&data.Request{
		URL: path,
	})

	if !isWL {
		t.Errorf("Server Path %s should be whitelisted", path)
	}

	// Server Path negative
	//
	path = "/index/index.html"
	isWL = wl.IsWhitelistedByServerPath(&data.Request{
		URL: path,
	})

	if isWL {
		t.Errorf("Server Path %s should not be whitelisted", path)
	}

	// Useragent
	//
	ua := "testUA/1.0"
	isWL = wl.IsWhitelistedByUseragent(&data.Request{
		UserAgent: ua,
	})

	if !isWL {
		t.Errorf("Useragent %s should be whitelisted", ua)
	}

	// Useragent negative
	//
	ua = "notestUA/1.0"
	isWL = wl.IsWhitelistedByUseragent(&data.Request{
		UserAgent: ua,
	})

	if isWL {
		t.Errorf("Useragent %s should not be whitelisted", ua)
	}

	<-recheckChan
}
