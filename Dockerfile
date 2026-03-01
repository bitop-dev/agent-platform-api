# Build stage
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o api ./cmd/api

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app

COPY --from=builder /app/api .

# Skills directory for WASM tool modules
RUN mkdir -p /app/data /home/app/.agent-core/skills && \
    chown -R app:app /app/data /home/app/.agent-core

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q --spider http://localhost:8080/health || exit 1

CMD ["./api"]
