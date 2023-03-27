FROM golang:1.18
# https://hub.docker.com/_/caddy?tab=description has docker compose example

RUN apt-get update && apt-get install -y git wget
RUN wget -q https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
RUN apt-get install -y ./google-chrome-stable_current_amd64.deb

COPY . /go/src/app
COPY ./static /static 
WORKDIR /go/src/app

RUN go get ./...
RUN go build .

ENTRYPOINT  ["./MensaQueueBot"]

