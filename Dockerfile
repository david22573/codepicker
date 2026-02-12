# Build Stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

# Copy module files first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -o codepicker main.go

# Final Stage
FROM alpine:latest

WORKDIR /root/

# Install git and ca-certificates (needed for agent git ops and https calls)
RUN apk add --no-cache git ca-certificates

COPY --from=builder /app/codepicker .

# Expose server port (API)
EXPOSE 22573
# Expose metrics/health port (Prometheus/K8s)
EXPOSE 9090

# Set default env
ENV OPENROUTER_API_KEY=""

# Entrypoint to server mode by default, but allow overriding
ENTRYPOINT ["./codepicker"]
CMD ["serve"]
