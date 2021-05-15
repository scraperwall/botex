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
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scraperwall/botex/config"
	"github.com/scraperwall/botex/data"
	log "github.com/sirupsen/logrus"
)

const wssCallsign = "[scw-wss]"
const wssOK = "OK"
const wssBlock = "BLOCK"

type WebserverSocket struct {
	listener        net.Listener
	block           *Block
	cookieKeyBin    []byte
	privateIPBlocks []*net.IPNet
	requestHandler  func(*data.Request)
	config          *config.Config
	mutex           sync.RWMutex
	ctx             context.Context
}

func NewWebserverSocket(ctx context.Context, config *config.Config, block *Block, requestHandler func(*data.Request)) (*WebserverSocket, error) {
	var err error

	wss := WebserverSocket{
		privateIPBlocks: make([]*net.IPNet, 0),
		block:           block,
		requestHandler:  requestHandler,
		config:          config,
		mutex:           sync.RWMutex{},
		ctx:             ctx,
	}

	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, netBlock, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Errorf("parse error on %q: %v", cidr, err)
		}
		wss.privateIPBlocks = append(wss.privateIPBlocks, netBlock)
	}

	wss.cookieKeyBin, err = base64.StdEncoding.DecodeString(config.CookieKey)
	if err != nil {
		return nil, err
	}

	wss.listener, err = net.Listen("unix", config.SocketFile)
	if err != nil {
		return nil, err
	}

	go wss.run()

	return &wss, nil
}

func (wss *WebserverSocket) run() {
	for {
		select {
		case <-wss.ctx.Done():
			log.Infof("closing web server socket %s", wss.listener.Addr())
			wss.listener.Close()
			err := os.Remove(wss.listener.Addr().String())
			if err != nil {
				log.Errorf("%s: %s", wss.listener.Addr(), err)
			}
			return
		default:
			conn, err := wss.listener.Accept()
			if err != nil {
				log.Fatal("accept error:", err)
			}

			go wss.serve(conn)
		}
	}
}

func (wss *WebserverSocket) serve(conn net.Conn) {
	var serverData struct {
		URL       string            `json:"url"`
		IP        string            `json:"ip"`
		Xff       string            `json:"xff"`
		Cookies   string            `json:"cookies"`
		Useragent string            `json:"useragent"`
		Headers   map[string]string `json:"headers"`
		Method    string            `json:"method"`
	}
	var err error

	log.Infof("Client connected [%s]", conn.RemoteAddr().Network())

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := scanner.Text()
		log.Infof("processing: %s", line)

		err = json.Unmarshal(scanner.Bytes(), &serverData)
		if err != nil {
			conn.Write([]byte(wssOK + "\n"))
			log.Warnf("%s isn't valid: %s!", line, err)
			continue
		}

		if wss.requestHandler != nil {
			ip := serverData.IP
			if serverData.Xff != "" {
				xips := strings.Split(serverData.Xff, ",")
				if len(xips) > 0 {
					ip = strings.TrimSpace(xips[0])
				}
			}
			ts := time.Now()

			wss.requestHandler(&data.Request{
				URL:       serverData.URL,
				Host:      serverData.Headers["host"],
				UserAgent: serverData.Useragent,
				Source:    ip,
				Method:    serverData.Method,
				Seq:       0,
				Timestamp: ts.UnixNano(),
				Time:      ts,
			})
		}
		log.Printf("ip: %s, xff: %s, cookies: %s\n", serverData.IP, serverData.Xff, serverData.Cookies)
		conn.Write([]byte(wss.decide(serverData.IP, serverData.Xff, serverData.Cookies) + "\n"))
	}

	conn.Close()
	log.Println("connection closed")
}

func (wss *WebserverSocket) decide(ip, xforwardedfor, cookies string) string {
	ips := []net.IP{}
	log.Infof("remote ip: %s", ip)

	if remote := wss.parseIP(ip); remote != nil { //&& !isPrivateIP(remote) {
		log.Infof("adding remote IP: %s", remote)
		ips = append(ips, remote)
	}

	for _, xff := range strings.Split(xforwardedfor, ",") {
		if parsedIP := wss.parseIP(strings.TrimSpace(xff)); parsedIP != nil && !wss.isPrivateIP(parsedIP) {
			ips = append(ips, parsedIP)
			log.Infof("adding X-Forwarded-For IP: %s", parsedIP.String())
		}
	}

	decision := wssOK
	decChan := make(chan string)

	for i, ip := range ips {
		log.Infof("[%d] ip: %s", i, ip)

		go wss.lookup(ip, decChan)
	}

	for i := 0; i < len(ips); i++ {
		dec := <-decChan
		if dec == wssBlock {
			decision = wssBlock
		}
	}
	close(decChan)

	signature := fmt.Sprintf("%s - %s - %s", ip, xforwardedfor, cookies)
	log.Infof("decision for %s: %s", signature, decision)

	if decision != wssOK {
		if wss.hasValidCaptchaCookie(cookies, ips) {
			log.Infof("%s has valid cookie!", signature)
			decision = wssOK
		} else {
			log.Infof("%s has no valid cookie!", signature)
		}
	}

	return decision
}

func (wss *WebserverSocket) lookup(ip net.IP, decChan chan string) {
	_, err := wss.block.GetIP(ip)

	// the IP isn't blocked
	if err != nil {
		decChan <- wssOK
		return
	}

	decChan <- wssBlock
}

func (wss *WebserverSocket) hasValidCaptchaCookie(cookies string, ips []net.IP) bool {
	// log.Printf("cookies: %s\n", cookies)
	for _, cookie := range strings.Split(cookies, ";") {
		cookie = strings.TrimSpace(cookie)

		// log.Printf("cookie: %s\n", cookie)
		if strings.HasPrefix(cookie, fmt.Sprintf("%s=", wss.config.CookieName)) {
			idx := strings.Index(cookie, "=")
			name := cookie[0:idx]
			if name != wss.config.CookieName {
				continue
			}

			value := cookie[idx+1:]

			if l := len(wss.cookieKeyBin); l != 32 {
				log.Errorf("cookie key has wrong length: %d instead of 32. Letting IP pass", l)
				return true
			}

			valueUnescaped, err := url.QueryUnescape(value)
			if err != nil {
				log.Infof("invalid URI encoding of cookie data: %s (%s)", err, value)
			}
			valueBin, err := base64.StdEncoding.DecodeString(valueUnescaped)
			if err != nil {
				log.Infof("invalid cookie encoding for %s: %s", value, err)
				return false
			}
			dec, err := wss.decrypt(valueBin, wss.cookieKeyBin)
			if err != nil {
				log.Infof("invalid cookie value for %s: %s", value, err)
				return false
			}

			cookieParts := strings.Split(string(dec), "|")

			if len(cookieParts) != 3 {
				log.Infof("invalid cookie content: %s", string(dec))
				return false
			}

			if cookieParts[0] != wss.config.CookieSecret {
				log.Infof("invalid secret: %s", cookieParts[0])
				return false
			}

			ts, err := strconv.ParseInt(cookieParts[2], 10, 64)
			if err != nil {
				log.Infof("invalid timestamp: %s", cookieParts[2])
				return false
			}

			timeStamp := time.Unix(ts, 0)
			if timeStamp.Before(time.Now()) {
				log.Infof("timestamp is in the past: %s", timeStamp.String())
				return false
			}

			ipMap := make(map[string]net.IP)
			for _, ip := range ips {
				ipMap[ip.String()] = ip
				log.Infof(fmt.Sprintf("setting IP %s to %s", ip, ipMap[ip.String()]))
			}

			for _, ip := range strings.Split(cookieParts[1], ",") {
				ipClean := strings.TrimSpace(ip)
				log.Infof(fmt.Sprintf("checking for IP %s", ipClean))
				if v, ok := ipMap[ipClean]; ok && v != nil {
					log.Infof("cookie contains valid IP: %s", v)
					return true
				}
			}
		}
	}

	return false
}

func (wss *WebserverSocket) encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte{}, err
	}

	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)

	plaintextPadded := wss.pad(plaintext)
	encrypted := make([]byte, len(plaintextPadded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encrypted, plaintextPadded)

	return append(iv, encrypted...), nil
}

func (wss *WebserverSocket) decrypt(encryptedIVandData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte{}, err
	}

	iv := encryptedIVandData[0:aes.BlockSize]
	encrypted := encryptedIVandData[aes.BlockSize:]

	mode := cipher.NewCBCDecrypter(block, iv)

	plaintextPadded := make([]byte, len(encrypted))
	mode.CryptBlocks(plaintextPadded, encrypted)
	plaintext := wss.unpad(plaintextPadded)

	return plaintext, nil
}

func (wss *WebserverSocket) unpad(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}

	padding := in[len(in)-1]
	if int(padding) > len(in) || padding > aes.BlockSize {
		return nil
	} else if padding == 0 {
		return nil
	}

	for i := len(in) - 1; i > len(in)-int(padding)-1; i-- {
		if in[i] != padding {
			return nil
		}
	}
	return in[:len(in)-int(padding)]
}

func (wss *WebserverSocket) pad(in []byte) []byte {
	padding := aes.BlockSize - (len(in) % aes.BlockSize)
	for i := 0; i < padding; i++ {
		in = append(in, byte(padding))
	}
	return in
}

func (wss *WebserverSocket) parseIP(ip string) net.IP {
	return net.ParseIP(ip).To16()
}

func (wss *WebserverSocket) isPrivateIP(ip net.IP) bool {
	if wss.config.IgnorePrivateIPs == false {
		return false
	}

	for _, block := range wss.privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
