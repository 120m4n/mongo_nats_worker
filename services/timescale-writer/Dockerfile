# syntax=docker/dockerfile:1
FROM golang:1.21-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o timescale-writer .

# Runtime stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates wget

COPY --from=builder /src/timescale-writer /usr/local/bin/

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD wget --spider -q http://localhost:3010/health || exit 1

ENTRYPOINT ["timescale-writer"]
