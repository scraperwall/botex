#
#	botex - a bad bot mitigation tool by ScraperWall
#	Copyright (C) 2021 ScraperWall, Tobias von Dewitz <tobias@scraperwall.com>
#
#	This program is free software: you can redistribute it and/or modify it
#	under the terms of the GNU Affero General Public License as published by
#	the Free Software Foundation, either version 3 of the License, or (at your
#	option) any later version.
#
#	This program is distributed in the hope that it will be useful, but WITHOUT
#	ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
#	FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License
#	for more details.
#
#	You should have received a copy of the GNU Affero General Public License
#	along with this program. If not, see <https://www.gnu.org/licenses/>.
#

VERSION=$(shell git describe --tags | sed 's/^v//')
BUILDDATE=$(shell date +%FT%T%z)
HOST=$(shell hostname)
NAME=$(shell basename $(PWD))

all: image
.PHONY: bin

image:	
	docker build \
    --rm \
    --build-arg VERSION=$(VERSION) \
    --build-arg BUILDDATE="$(BUILDDATE)" \
    --build-arg HOST=$(HOST) \
		-t registry.scw.systems/$(NAME):latest \
		-t registry.scw.systems/$(NAME):$(VERSION) \
		.

release:
	docker push registry.scw.systems/$(NAME):latest
	docker push registry.scw.systems/$(NAME):$(VERSION)


compile:
	docker run --rm -v $(PWD):/src \
      -w /src \
      golang:alpine \
      env CGO_ENABLED=0 go build \
        -tags netgo \
        -a -v \
        -mod=vendor \
        -ldflags "-s -X main.Version=$(VERSION) -X main.BuildDate=$(BUILDDATE) -X main.BuildHost=$(HOST) -extldflags 'static'"

bin:
	env GOOS=linux GOARCH=amd64 go build -ldflags "-s -X main.Version=$(VERSION) -X main.BuildDate=$(BUILDDATE) -X main.BuildHost=$(HOST) -extldflags 'static'" -o ./bin/linux-amd64/botex ./cmd/botex
	cp LICENSE ./bin/linux-amd64/
	(cd bin/linux-amd64 && tar cvfz botex-linux-amd64-$(VERSION).tar.gz *)
	env GOOS=darwin GOARCH=amd64 go build -ldflags "-s -X main.Version=$(VERSION) -X main.BuildDate=$(BUILDDATE) -X main.BuildHost=$(HOST)" -o ./bin/darwin-amd64/botex ./cmd/botex
	cp LICENSE ./bin/darwin-amd64/
	(cd bin/darwin-amd64 && tar cvfz botex-darwin-amd64-$(VERSION).tar.gz *)
	env GOOS=darwin GOARCH=arm64 go build -ldflags "-s -X main.Version=$(VERSION) -X main.BuildDate=$(BUILDDATE) -X main.BuildHost=$(HOST)" -o ./bin/darwin-arm64/botex ./cmd/botex
	cp LICENSE ./bin/darwin-arm64/
	(cd bin/darwin-arm64 && tar cvfz botex-darwin-arm64-$(VERSION).tar.gz *)
	sha256sum bin/*/*.tar.gz | perl -pe 's:bin/.+?/::' > bin/checksums-$(VERSION).txt
