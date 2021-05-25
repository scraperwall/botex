# Botex

Botex is an open source bad bot management tool from [ScraperWall](https://scraperwall.com/). Botex enables you to protect your website from automated attacks on your application logic such as content and price scraping, account takeovers, mysterious load spikes or skewed metrics.

It can be integrated into Apache, Nginx, Varnish or third party WAFs such as Cloudflare or simply be used to analyze your website traffic in real time as a first step to evaluate whether integrating it into the web stack completely makes sense.


## Installation

If you've got the Go toolchain installed, botex can easily be installed by go

    $ go get github.com/scraperwall/botex/cmd/botex

Binary releases are available for Linux and Mac OS X.


## Analyze a log file

If you only want to check if your website has a bad bot problem you can replay a single log file. For this to work, botex needs to know the format of the log file and its location. 

	$ botex \
	    -log-replay /var/log/nginx/access.log.0 \
	    -log-format '$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"'

botex uses [nginx variable names](http://nginx.org/en/docs/varindex.html). It needs *\$remote_addr*, *\$time_local* and *\$request* as a bare minimum to analyze a log file. Make sure you use these names even if you analyze a log file from a different server such as Apache.

botex loads the log file so that its entire content fits into the time frame you've set it to observe.

### Time Frame

botex looks at requests in a time window you can set. An hour is a good starting point.

	$ botex -window-size 1m -num-windows 60

makes botex use 60 slices, each one minute long. This means that incoming HTTP requests get grouped into one-minute blocks and expire after 60 minutes.

### Detection configuration

The open source version of botex performs a heuristic analysis of incoming HTTP requests. It calculates the ratio of HTML URLs to those of assets (images, CSS, Javascript) on an IP, network and autonomous system level.

If an IP exceeds the specified ratio towards HTML URLs and a configurable number of total requests it gets flagged as *bad*.

	$ botex -min-app-requests 10 -max-app-requests 150 -max-ratio 0.91

tells botex to not flag any IP as bad if has less than 11 requests. Once it has made more requests in the specified time frame it will get flagged as *bad* only if the ratio of *total requests* : *HTML requests* is bigger than 0.91, i.e. the client has downloaded much more HTML content than assets.

This approach works well even if you use a CDN for assets since not all images will be cached in the CDN all the time.

### Network and Autonomous System analysis

botex will perform the heuristic analysis on entire networks and autonomous systems. VPNs and proxy farms usually rent IPs from a limited number of providers in a limited number of networks. Suppose an attacker rents 200 IPs from a proxy farm. This allows him to perform a much larger number of requests before botex detects him but since his IPs come from a small number of networks and an even smaller number of autonomous systems (i.e. companies) looking at the *total* : *HTML* ratio based on networks and autonomous systems detects him easily and makes it much more cumbersome for him to bypass the detection.

	$ botex -networks

### DNS Lookups

botex doesn't flag any IP before it hasn't looked up its host name successfully. The DNS lookup process happens in real time and resolved IPs are being cached. Depending on how many requests per second your web server receives you can make botex use a corresponding number of resolver threads.

	$ botex -resolver-workers 25 -dns-server 192.168.1.1:53

makes botex use 25 parallel DNS resolver workers that all use 192.168.1.1 as name server. If the number of requests is too much for your own name server, you may use a free public name server, e.g. from Google or Cloudflare. 25 threads is a good starting point for small to medium websites.

### GeoIP and ASN databases

For its analysis botex requires [two free databases from Maxmind](https://dev.maxmind.com/geoip/geoip2/geolite2/): *"GeoLite2 ASN: CSV Format"* (ZIP) and *"GeoLite2 City"* (MMDB, gzip).

At the time of writing you had to sign up for an account to download the databases.

	$ botex \
	    -asndb-file ./GeoLite2-ASN-CSV_20210511.zip \
	    -geoipdb-file ./GeoLite2-City_20210511.tar.gz


### Request logging

If you would like to see which URLs an IP has requested, botex remembers the latest requests.

	$ botex -keep-requests 100

lets botex keep the 100 latest requests per IP address.

## Whitelisting

There are guaranteed to be IP addresses which you don't want blocked no matter what. Those IPs can be whitelisted by a TOML file using a wide range of filters. 

	[[ClientHost]]
	Pattern = ".+\\.googlebot\\.com"
	Description = "Googlebot"

	[[ClientHost]]
	Pattern = ".+\\.search\\.msn\\.com"
	Description = "Bing Bot"

	[[ASN]]
	Pattern = "714"
	Description = "Apple"

	[[IP]]
	Pattern = "165\\.73\\.12\\.251"
	Description = "Monitoring"

	[[CIDR]]
	Pattern = "23.15.155.0/24"
	Description = "Partner Network"

	[[ServerHost]]
	Pattern = "cdn.+\\.mydomain\\.com"
	Description = "CDN"

	[[ServerPath]]
	Pattern = "/checkout/.+"
	Description = "Checkout Process"

	[[Useragent]]
	Pattern = "mymonitor/1\\..+ (136228b8b06f9296)"
	Description = "Internal Monitor"

All Patterns except for ASN and CIDR are evaluated as regular expressions. Backslashes (\\) need to be escaped as demonstrated above.

**ClientHost** can be used to whitelist remote hosts such as those from which the Google Bot visits websites.

With **ASN** an entire autonomous system (i.e. an entire company) can be whitelisted. In the example above Apple is whitelisted through its autonomous system number (ASN) 714.

**IP** can whitelist IP addresses. Since the pattern is evaluated as a regular expression whitelisting more than one IP is possible: 

	Pattern = "112\\.47\\..+

whitelistes all IPs that start with 112.47.

**CIDR** can be used to whitelist entire networks. Use the CIDR notation as demonstrated above.

If there are hosts on your side that you don't want to be observed for some reason, use ***ServerHost*** to whitelist them. 

	Pattern = "cdn.+\\.mysite\\.com"

would whitelist all traffic to your CDN servers, e.g. *cdn1.mysite.com* or *cdn-34-euwest.mysite.com*.

***ServerPath*** can in analogy be used to ignore traffic for specific URLs, for example if you don't want botex to count requests once a customer has entered the checkout process.

With ***Useragent*** you can whitelist clients by their user agent. This should only be the last resort if all other whitelist options fail since faking a user agent is extremely easy.

### Complete Example

Suppose your log file contains log entries that are formatted as follows

	94.134.88.168 - - [31/Mar/2020:10:00:00 +0200] "GET / HTTP/1.1" 200 12573 "https://scraperwall.com/" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36"

the corresponding log file format specification for botex is

	'$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"'

and in order to analyze the log file you would start botex with the following parameters:

	$ botex \
	   -log-replay=/var/log/nginx/access.log.0 \
	   -log-format '$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent" "$none"' \
           -resolver-workers 50 \
           -dns-server 192.168.1.1:53 \
           -asndb-file ./GeoLite2-ASN-CSV_20210511.zip \
           -geoipdb-file ./GeoLite2-City_20210511.tar.gz \
           -window-size 1m \
           -keep-requests 500 \
           -num-windows 60 \
           -max-app-requests 150 \
           -max-ratio 0.91  \
           -networks \
		   -whitelist ./whitelist.toml \
           -clear-blocked


*-clear-blocked* makes botex clear all blocked IPs from the database when it starts.

## API

The list of blocked IPs can be retrieved via an API in real-time. The API listens on port 4343.

	$ curl -s http://127.0.0.1:4343/blocked/ips
	 
	[{
	    "total": 134,
	    "app": 134,
	    "other": 0,
	    "ratio": 1,
	    "ips": 0,
	    "asn": {
	      "network": {
	        "IP": "3.120.0.0",
	        "Mask": "//gAAA=="
	      },
	      "from": "3.120.0.0",
	      "to": "3.127.255.255",
	      "cidr": "3.120.0.0/13",
	      "asn": 16509,
	      "organization": "AMAZON-02"
	    },
	    "reason": "too many requests (13/10) and app/asset ratio too high (1.00/0.91)",
	    "city": {
	      "name": "Frankfurt am Main",
	      "continent": "Europe",
	      "continent_code": "EU",
	      "country": "Germany",
	      "country_code": "DE",
	      "accuracy_radius": 1000,
	      "latitude": 50.1188,
	      "longitude": 8.6843,
	      "metro_code": 0,
	      "timezone": "Europe/Berlin",
	      "postcode": "60313",
	      "registered_country": "United States",
	      "registered_country_code": "US",
	      "represented_country": "",
	      "represented_country_code": "",
	      "represented_country_type": "",
	      "subdivisions": [
	        "Hesse"
	      ],
	      "is_anonymous_proxy": false,
	      "is_satellite_provider": false
	    },
	    "blocked_at": "2021-05-12T15:51:29.288571+02:00",
	    "ip": "3.123.36.206",
	    "hostname": "ec2-3-123-36-206.eu-central-1.compute.amazonaws.com"
	}]


### Everything about a single IP address

All available information about a single IP address can be retrieved, too:

	$ curl -s | jq .
	{
	  "ip_details": {
	    "ip": "107.178.98.165",
	    "hostname": "we.love.servers.at.ioflood.net",
	    "asn": {
	      "network": {
	        "IP": "107.178.64.0",
	        "Mask": "///AAA=="
	      },
	      "from": "107.178.64.0",
	      "to": "107.178.127.255",
	      "cidr": "107.178.64.0/18",
	      "asn": 53755,
	      "organization": "IOFLOOD"
	    },
	    "geoip": {
	      "ip": "107.178.98.165",
	      "anonymous": {
	        "is_anonymous": false,
	        "is_anonymous_vpn": false,
	        "is_hosting_provider": false,
	        "is_public_proxy": false,
	        "is_tor_exit_node": false
	      },
	      "city": {
	        "name": "Phoenix",
	        "continent": "North America",
	        "continent_code": "NA",
	        "country": "United States",
	        "country_code": "US",
	        "accuracy_radius": 1000,
	        "latitude": 33.4413,
	        "longitude": -112.0421,
	        "metro_code": 753,
	        "timezone": "America/Phoenix",
	        "postcode": "85034",
	        "registered_country": "United States",
	        "registered_country_code": "US",
	        "represented_country": "",
	        "represented_country_code": "",
	        "represented_country_type": "",
	        "subdivisions": [
	          "Arizona"
	        ],
	        "is_anonymous_proxy": false,
	        "is_satellite_provider": false
	      },
	      "country": {
	        "continent_code": "NA",
	        "continent": "North America",
	        "country_code": "US",
	        "country": "United States",
	        "registered_country_code": "US",
	        "registered_country": "United States",
	        "represented_country_code": "",
	        "represented_country_type": "",
	        "represented_country": "",
	        "is_anonymous_proxy": false,
	        "is_satellite_provider": false
	      },
	      "domain": {
	        "domain": ""
	      },
	      "isp": {
	        "autonomous_system_number": 0,
	        "autonomous_system_organization": "",
	        "isp": "",
	        "organization": ""
	      }
	    },
	    "total": 1,
	    "app": 1,
	    "other": 0,
	    "ratio": 1,
	    "is_blocked": true,
	    "block_reason": "asn has too many requests (2797/150) and ratio is too high (1.00/0.91)",
	    "whitelisted": false,
	    "whitelist_reason": "",
	    "created_at": "2021-05-12T16:20:21.360999+02:00",
	    "updated_at": "2021-05-12T16:21:45.991959+02:00",
	    "lastblock_at": "2021-05-12T16:22:21.428609+02:00"
	  },
	  "requests": [
	    {
	      "url": "/axess/Axess-2020.html",
	      "host": "scw.test",
	      "useragent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.181 Safari/537.36",
	      "source": "107.178.98.165",
	      "method": "GET",
	      "seq": 0,
	      "timestamp": 1620829305991955000,
	      "time": "2021-05-12T16:21:45.991955+02:00",
	      "asn": {
	        "network": {
	          "IP": "107.178.64.0",
	          "Mask": "///AAA=="
	        },
	        "from": "107.178.64.0",
	        "to": "107.178.127.255",
	        "cidr": "107.178.64.0/18",
	        "asn": 53755,
	        "organization": "IOFLOOD"
	      },
	      "is_app": true
	    }
	  ],
	  "useragents": {
	    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/65.0.3325.181 Safari/537.36": 1
	  }
	}

## Web GUI

There is a web GUI available at [github.com/scraperwall/botex-admin](https://github.com/scraperwall/botex-admin)

## License

Unless otherwise noted, the botex source files are distributed under the GNU AGPLv3 license found in the LICENSE file.
