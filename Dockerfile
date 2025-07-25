FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o cf-updater -ldflags="-s -w" .

FROM alpine:3.22 AS release
RUN apk add --no-cache ca-certificates
USER nobody
COPY --from=builder /app/cf-updater /usr/local/bin/cf-updater
ENTRYPOINT ["cf-updater"]
