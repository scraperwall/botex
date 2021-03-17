package data

import (
	"time"

	"github.com/scraperwall/asndb/v2"
)

// Request represents an HTTP request
type Request struct {
	URL       string     `json:"url"`
	Host      string     `json:"host"`
	UserAgent string     `json:"useragent"`
	Source    string     `json:"source"`
	Method    string     `json:"method"`
	Seq       int        `json:"seq"`
	Timestamp int64      `json:"timestamp"`
	Time      time.Time  `json:"-"`
	ASN       *asndb.ASN `json:"asn"`
}
