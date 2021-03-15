package botex

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/scraperwall/botex/config"
	log "github.com/sirupsen/logrus"
)

const whitelistTestTOML = `
[[IP]]
Pattern = "1\\.1\\..+"
Description = "Cloudflare"

[[IP]]
Pattern = "faa0:0000:0000:0000:0000:0000:0000:00001"
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
Pattern = "staging.scw.systems"
Description = "staging"

[[ServerPath]]
Pattern = "/api/.+"
Description = "API"
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

	wl, err := NewWhitelist(context.Background(), nil, &config)
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
}
