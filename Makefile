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






