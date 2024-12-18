# -------------------
# BUILD STAGE (Reduces final image size)
# -------------------
FROM golang:1.23.3 AS builder

#ENV CGO_ENABLED=1

# Set up working directories
WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the plugin
RUN go build -buildmode=plugin -o ./app_plugins/sqlite_loader_plugin.so sqlite-plugin/sqlite_loader_plugin.go

# Build the main application
RUN go build -o /app/dist/openpool-manager ./cmd/manager/main.go &&\
    go build -o /app/dist/openpool-data-loader ./cmd/data_loader/main.go &&\
    go build -o /app/dist/openpool-payout-loop ./cmd/payouts/main.go &&\
    go build -o /app/dist/openpool-api ./cmd/server/main.go

# -------------------
# RUNTIME STAGE (Slim final image)
# -------------------
FROM ubuntu:22.04
WORKDIR /app
# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive

# Install required runtime dependencies
RUN apt-get update && apt-get install -y \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# Create necessary directories
RUN mkdir -p /etc/pool /app /app/app_plugins

# Copy compiled application & plugin from builder stage
COPY --from=builder /app/dist/* /usr/local/bin/
COPY --from=builder /app/app_plugins/sqlite_loader_plugin.so /app/app_plugins/sqlite_loader_plugin.so

# Copy preloaded config.json into the container
COPY config.json /etc/pool/config.json

EXPOSE 8080
ENV PATH=/usr/local/bin/:$PATH
CMD ["openpool-api","-f /etc/pool/config.json"]