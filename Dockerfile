# Stage 1: Frontend Builder
FROM node:22-bookworm-slim AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Backend Builder
FROM golang:1.24-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Enforce CGO for go-sqlite3
# Install Playwright driver keys to a fixed path
ENV PLAYWRIGHT_BROWSERS_PATH=/app/pw-browsers
RUN go run github.com/playwright-community/playwright-go/cmd/playwright@v0.4101.1 install --with-deps
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -trimpath -o dashboard-recorder ./cmd/server

# Stage 2: Runtime
FROM debian:bookworm-slim

# Install dependencies with security updates
# STRICTLY chain update and upgrade to ensure security patches
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    chromium \
    ffmpeg \
    fonts-noto-cjk \
    ca-certificates \
    dumb-init \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r appuser && useradd -r -g appuser -u 1000 -m -d /home/appuser appuser

# Create app directory and volumes with correct permissions
# Ensure /app/data and /app/recordings are writable by the user
WORKDIR /app
RUN mkdir -p /app/data/certs /app/recordings && \
    chown -R appuser:appuser /app

# Copy binary
COPY --from=builder --chown=appuser:appuser /app/dashboard-recorder /app/server
COPY --from=frontend-builder --chown=appuser:appuser /app/web/dist /app/web/dist
# Copy Playwright browsers (Chromium)
COPY --from=builder --chown=appuser:appuser /app/pw-browsers /home/appuser/pw-browsers
# Copy Playwright Driver
COPY --from=builder --chown=appuser:appuser /root/.cache/ms-playwright-go /home/appuser/.cache/ms-playwright-go

ENV PLAYWRIGHT_BROWSERS_PATH=/home/appuser/pw-browsers

USER appuser
# Expose unprivileged ports
EXPOSE 8080 8443
ENV HOME=/home/appuser
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/app/server"]
