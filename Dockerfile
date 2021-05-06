FROM golang:1.16-alpine as builder

WORKDIR /workspace

RUN apk add --no-cache git
COPY go.mod go.sum /workspace/
RUN go mod download

COPY main.go main.go
COPY action.go action.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o action

# ---------------
FROM gcr.io/distroless/static:latest

LABEL org.opencontainers.image.source=https://github.com/rode/github-actions/create-build-occurrence

COPY --from=builder /workspace/action /usr/local/bin/action

ENTRYPOINT ["action"]
