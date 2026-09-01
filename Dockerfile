# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Build static binary
ARG TARGETOS
ARG TARGETARCH
ARG GOAMD64=v1
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} GOAMD64=${GOAMD64} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /bin/nedis-server ./cmd/nedis-server

# Runtime Stage
FROM alpine:3.21

# Install ca-certificates and tzdata for TLS/timezones
RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S -g 10001 nedis \
    && adduser -S -u 10001 -G nedis -h /data -s /sbin/nologin nedis \
    && mkdir -p /data \
    && chown -R nedis:nedis /data

COPY --from=builder /bin/nedis-server /usr/local/bin/nedis-server

USER 10001:10001
WORKDIR /data
VOLUME ["/data"]

EXPOSE 6379

ENTRYPOINT ["/usr/local/bin/nedis-server"]
CMD ["-host", "0.0.0.0", "-port", "6379"]
