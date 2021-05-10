VERSION=$(shell git describe --tags | sed 's/^v//')
BUILDDATE=$(shell date +%FT%T%z)
HOST=$(shell hostname)
NAME=$(shell basename $(PWD))

all: image


image:	
	docker build \
    --rm \
    --build-arg VERSION=$(VERSION) \
    --build-arg BUILD="$(BUILDDATE)" \
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
        -ldflags "-s -X main.Version="$(VERSION)" -X main.BuildDate="$(BUILDDATE)" -X main.BuildHost="$(HOST)" -extldflags 'static'"






