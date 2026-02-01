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

# Install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    ffmpeg \
    fonts-noto-cjk \
    ca-certificates \
    dumb-init \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r appuser && useradd -r -g appuser -u 1000 -m -d /home/appuser appuser

# Create app directory and volumes with correct permissions
WORKDIR /app
RUN mkdir -p /app/data /app/recordings && \
    chown -R appuser:appuser /app

# Copy binary
COPY --from=builder --chown=appuser:appuser /app/dashboard-recorder /app/server
COPY --from=frontend-builder --chown=appuser:appuser /app/web/dist /app/web/dist
# Copy Playwright browsers (Chromium)
COPY --from=builder --chown=appuser:appuser /app/pw-browsers /home/appuser/pw-browsers
# Copy Playwright Driver (The nodejs scripts) - It was installed to /root/.cache in builder
COPY --from=builder --chown=appuser:appuser /root/.cache/ms-playwright-go /home/appuser/.cache/ms-playwright-go

ENV PLAYWRIGHT_BROWSERS_PATH=/home/appuser/pw-browsers
# Playwright-Go looks for the driver relative to the cache usually, or we might need to set a driver path.
# But copying to ~/.cache/ms-playwright-go usually works if the user matches.
# I will include the frontend copy if the previous context suggests it exists, 
# but strictly the user gave specific base image instructions.)

# Let's verify if I should include frontend. The user said "Files to Generate ... Dockerfile".
# I will stick to a standard multi-stage build similar to before but refined.
# Actually, I'll stick to the user's explicit request for the *Runtime* part details.
# But I need to include the frontend content to serve it. 
# I'll re-add the frontend builder stage from the previous successful state to be safe.

USER appuser
EXPOSE 8080
ENV HOME=/home/appuser
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/app/server"]
