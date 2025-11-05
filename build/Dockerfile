# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Copy go.mod and go.sum and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build statically linked Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./main.go

# --- Final minimal image ---
FROM gcr.io/distroless/static

# Copy the compiled binary and migrations
COPY --from=builder /app /app
COPY --from=builder /src/migrations ./migrations

# Copy TLS certificates
COPY certs /certs

# Expose HTTPS server port
EXPOSE 8080

# Set entrypoint to run the application
ENTRYPOINT ["/app"]
