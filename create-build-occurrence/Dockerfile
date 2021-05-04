# syntax = docker/dockerfile:experimental
FROM golang:1.16-alpine

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY main.go main.go

ENTRYPOINT ["go run main.go"]
