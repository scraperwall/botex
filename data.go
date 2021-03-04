package botex

import "time"

// Request represents an HTTP request
type Request struct {
	URL       string    `json:"url"`
	Host      string    `json:"host"`
	UserAgent string    `json:"useragent"`
	Source    string    `json:"source"`
	Method    string    `json:"method"`
	Timestamp int64     `json:"timestamp"`
	Time      time.Time `json:"-"`
}
