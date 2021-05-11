FROM golang:alpine

ARG VERSION
ARG BUILDDATE
ARG HOST

ADD . /src
WORKDIR /src

# RUN go get
RUN env CGO_ENABLED=0 GOOS=linux \
    go build \
    -mod vendor \
    -tags netgo \
    -ldflags "-s -X main.Version=$VERSION -X main.BuildDate=$BUILDDATE -X main.BuildHost=$HOST -extldflags 'static'" \
    ./cmd/botex/

FROM alpine:latest

COPY --from=0 /src/botex /

# USER nobody:nobody
WORKDIR /
CMD /botex

EXPOSE 4223
EXPOSE 4343
