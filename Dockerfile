FROM golang:1.18
# https://hub.docker.com/_/caddy?tab=description has docker compose example

RUN apt-get update && apt-get install git

COPY . /go/src/app
WORKDIR /go/src/app

RUN go get ./...
RUN go build .

ENTRYPOINT  ["./MensaQueueBot"]

