# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build all binaries
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/migrate ./cmd/migrate/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/seed ./cmd/seed/

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 appuser

WORKDIR /app

COPY --from=builder /bin/server /bin/migrate /bin/seed /usr/local/bin/
COPY docker-entrypoint.sh /usr/local/bin/
COPY db/migrations ./db/migrations/

RUN chmod +x /usr/local/bin/docker-entrypoint.sh

USER appuser

STOPSIGNAL SIGTERM

EXPOSE 3000

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server"]
