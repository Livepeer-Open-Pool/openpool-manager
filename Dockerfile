# -------------------
# BUILD STAGE (Reduces final image size)
# -------------------
FROM golang:1.23.3 AS plugin_builder

ENV CGO_ENABLED=1
ENV GIN_MODE=release

# Set up working directories
WORKDIR /app

# Copy source code
COPY . .

# Build the plugins
RUN go mod download
RUN go mod tidy
RUN go build -buildmode=plugin -o /app/app_plugins/test-storage.so test-storage/plugin.go
RUN go build -buildmode=plugin -o /app/app_plugins/sqlite-storage.so sqlite-storage/plugin.go
RUN go build -buildmode=plugin -o /app/app_plugins/dataloader.so dataloader/plugin.go
RUN go build -buildmode=plugin -o /app/app_plugins/payoutloop.so payoutloop/plugin.go
RUN go build -buildmode=plugin -o /app/app_plugins/api.so api/plugin.go
RUN go build -o /app/dist/open-pool-manager main.go


# -------------------
# RUNTIME STAGE (Slim final image)
# -------------------
FROM ubuntu:24.04
WORKDIR /app
# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive

# Install required runtime dependencies
RUN apt-get update && apt-get install -y \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# Create necessary directories
RUN mkdir -p /etc/pool /var/lib/open-pool/

# Copy compiled application & plugin from builder stage
COPY --from=plugin_builder /app/dist/* /usr/local/bin/
COPY --from=plugin_builder /app/app_plugins /var/lib/open-pool/app_plugins

# Copy preloaded config.json into the container
COPY sample.config.json /etc/open-pool/config.json

EXPOSE 8080
ENV PATH=/usr/local/bin/:$PATH
CMD ["/usr/local/bin/openpool-manager","-f /etc/open-pool/config.json"]