FROM golang:alpine AS build

ARG VERSION
ARG BUILD
ARG HOST

ADD . /src
WORKDIR /src

# RUN go get
RUN go build -mod vendor -tags netgo -ldflags "-s -X main.Version="$VERSION" -X main.BuildDate="$BUILD" -X main.BuildHost="$HOST" -extldflags 'static'"

FROM alpine:latest

# RUN apk --no-cache add ca-certificates

COPY --from=build /src/botex /

USER nobody:nobody
WORKDIR /
CMD /botex

EXPOSE 4223
EXPOSE 4343
