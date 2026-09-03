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
    -o /bin/gopherdis-server ./cmd/gopherdis-server

# Runtime Stage
FROM alpine:3.21

# Install ca-certificates and tzdata for TLS/timezones
RUN apk --no-cache add ca-certificates tzdata \
    && addgroup -S -g 10001 gopherdis \
    && adduser -S -u 10001 -G gopherdis -h /data -s /sbin/nologin gopherdis \
    && mkdir -p /data \
    && chown -R gopherdis:gopherdis /data

COPY --from=builder /bin/gopherdis-server /usr/local/bin/gopherdis-server

USER 10001:10001
WORKDIR /data
VOLUME ["/data"]

EXPOSE 6379

ENTRYPOINT ["/usr/local/bin/gopherdis-server"]
CMD ["-host", "0.0.0.0", "-port", "6379"]
