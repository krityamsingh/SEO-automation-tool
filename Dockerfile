# Build stage
FROM golang:1.23-alpine AS builder

# sqlite3 requires CGO for the mattn/go-sqlite3 driver
RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o agent cmd/agent/main.go

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/agent .
COPY --from=builder /app/.env.example .

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

CMD ["./agent"]
