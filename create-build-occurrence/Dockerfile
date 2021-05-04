# syntax = docker/dockerfile:experimental
FROM golang:1.16-alpine

WORKDIR /workspace

RUN apk add --no-cache git
COPY go.mod go.sum /workspace/
RUN go mod download

COPY main.go main.go
RUN go build -o action && mv action /usr/local/bin/action

ENTRYPOINT ["action"]
