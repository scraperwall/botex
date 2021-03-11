package botex

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/satyrius/gonx"
	log "github.com/sirupsen/logrus"
)

// LogReplay replays a log file into the application
func (b *Botex) LogReplay(logfile, format string) {

	fh, err := os.Open(logfile)
	if err != nil {
		log.Fatalf("%s: %s", logfile, err)
	}
	defer fh.Close()

	lines := 0
	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		lines++
	}

	fh.Seek(0, io.SeekStart)
	scanner = bufio.NewScanner(fh)

	/*
		jsonc, err := nats.NewEncodedConn(b.config.NatsConn, nats.JSON_ENCODER)
		if err != nil {
			log.Fatal(err)
		}
	*/

	tOffset := b.config.WindowSize * time.Duration(b.config.NumWindows)
	tStart := time.Now().Add(-1 * tOffset)
	timePerLine := time.Duration(int(tOffset) / lines)
	log.Infof("time per line: %v, lines: %d", timePerLine, lines)

	if format == "" {
		format = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`
	}
	p := gonx.NewParser(format)
	reqRegexp := regexp.MustCompile(`^([A-Z]+)\s+(.+?)\s+(HTTP/\d+\.\d+)$`)

	for scanner.Scan() {
		l := scanner.Text()

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}

		var remote string
		var err error

		logEntry, err := p.ParseString(l)
		if err != nil {
			continue
		}

		remote, err = logEntry.Field("remote_addr")
		if err != nil {
			continue
		}

		xff, err := logEntry.Field("http_x_forwarded_for")
		if err != nil && xff != "" {
			remote = xff
		}

		if remote == "" {
			continue
		}

		// only use the first host in case there are multiple hosts in the log
		if cidx := strings.Index(remote, ","); cidx >= 0 {
			remote = remote[0:cidx]
		}

		httpRequest, err := logEntry.Field("request")
		if err != nil {
			continue
		}

		reqData := reqRegexp.FindStringSubmatch(httpRequest)
		if len(reqData) < 4 {
			log.Infof("reqData is too short: %d instead of 4\n", len(reqData))
			continue
		}

		request := Request{
			Source:    remote,
			Timestamp: tStart.UnixNano(),
			URL:       reqData[2],
			Host:      "scw.test",
			Method:    reqData[1],
		}

		tStart = tStart.Add(time.Duration(timePerLine))

		request.UserAgent, _ = logEntry.Field("http_user_agent")

		b.HandleRequest(&request)
		// jsonc.Publish(natsRequestsSubject, request)
	}

	time.Sleep(b.config.WindowSize)

	for {
		fh.Seek(0, io.SeekStart)
		scanner = bufio.NewScanner(fh)

		for scanner.Scan() {
			l := scanner.Text()

			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}

			var remote string
			var err error

			logEntry, err := p.ParseString(l)
			if err != nil {
				log.Println(err)
				continue
			}

			remote, err = logEntry.Field("remote_addr")
			if err != nil {
				log.Println(err)
				continue
			}

			xff, err := logEntry.Field("http_x_forwarded_for")
			if err != nil && xff != "" {
				remote = xff
			}

			if remote == "" {
				log.Println("remote is empty: ignoring request.")
				continue
			}

			// only use the first host in case there are multiple hosts in the log
			if cidx := strings.Index(remote, ","); cidx >= 0 {
				remote = remote[0:cidx]
			}

			httpRequest, err := logEntry.Field("request")
			if err != nil {
				log.Println(err)
				continue
			}

			reqData := reqRegexp.FindStringSubmatch(httpRequest)
			if len(reqData) < 4 {
				log.Printf("reqData is too short: %d instead of 4\n", len(reqData))
				continue
			}

			request := Request{
				Source:    remote,
				Timestamp: time.Now().UnixNano(),
				URL:       reqData[2],
				Host:      "scw.test",
				Method:    reqData[1],
			}

			request.UserAgent, _ = logEntry.Field("http_user_agent")

			b.HandleRequest(&request)
			//jsonc.Publish(natsRequestsSubject, request)
			time.Sleep(timePerLine)
		}
	}
}
