# Dashboard Recorder

## Overview

**Dashboard Recorder** is a high-performance containerized browser recording service. Built with Go (Echo) and Playwright, it captures web pages as video files at specified frame rates and resolutions. It also features an interactive mode for real-time remote browser control.

## Features

- **Container Native**: Runs entirely within Docker, deployable anywhere.
- **High-Quality Recording**: Configurable frame rates (1-15 FPS) and resolution (1920x1080) per task.
- **Interactive Mode**: Remote control the browser similar to OBS browser sources.
- **Custom CSS**: Inject custom CSS to hide elements or adjust styles before recording.
- **Secure**: Non-root execution, security headers, JWT auth, and rate limiting.

## Requirements

- **Docker**
- **Docker Compose**
- Recommended: 4GB+ RAM (for browser execution)

## Installation

### 1. Docker Compose (Recommended)

Create a `compose.yml` file and start the service using the DockerHub image.

```yaml
services:
  app:
    image: nullpo7z/dashboard-recorder:latest
    container_name: dashboard_recorder
    restart: unless-stopped
    ports:
      - "80:8080"   # HTTP (Redirect to HTTPS)
      - "443:8443"  # HTTPS
    environment:
      - TZ=Asia/Tokyo
      - LOG_LEVEL=info
      - JWT_SECRET=change_me_in_production
      - DATABASE_PATH=/app/data/app.db
      # OIDC Configuration (Optional)
      - OIDC_PROVIDER=
      - OIDC_CLIENT_ID=
      - OIDC_CLIENT_SECRET=
      - OIDC_REDIRECT_URL=
      - OIDC_ALLOWED_EMAILS=
      # Auto-HTTPS (Let's Encrypt) Configuration
      - TLS_DOMAIN=yourdomain.com
      - TLS_EMAIL=youremail@example.com
    volumes:
      - ./backend_data:/app/data
      - ./backend_recordings:/app/recordings
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE
    security_opt:
      - no-new-privileges:true
    user: "1000:1000"
```

### 2. Directory Permissions (Important)

This application runs as a non-root user (UID 1000) for enhanced security.
You MUST set the correct write permissions for the host volume directories.

```bash
# Create and set permissions for data directories
mkdir -p backend_data backend_recordings
sudo chown -R 1000:1000 backend_data backend_recordings
```

```bash
# Start
docker compose up -d
```

### 2. Build from Source (For Development)

Clone the repository and build locally.

```bash
git clone https://github.com/nullpo7z/browser-recorder.git
cd browser-recorder
docker compose up -d --build
```

## Usage

### 1. Access Dashboard
Navigate to `http://localhost:8090` in your browser.

### 2. Login
Use the default credentials:
- **Username**: `admin`
- **Password**: `admin`

### 3. Create Task
Click "New Recording Task" on the Dashboard to create a task.
- **Task Name**: A name for your task.
- **Target URL**: The URL of the website to record.
- **Filename Prefix**: Template for the recording filename.
- **FPS**: Set between 1 and 15.
- **Custom CSS**: Enter CSS to style the page (optional).

### 4. Recording & Interaction
- **Start**: Begin recording.
- **Stop**: Stop recording and save the video file.
- **Interact**: Remote control the browser (supports clicks and keyboard input).
- **Setting**: Change task settings.

## Notes

- **Security**: **Change your password immediately** after the first login. For production, change the `JWT_SECRET` in `compose.yml` to a random string.
- **Performance**: FPS is limited to **15 FPS** to manage server load.
- **Data Persistence**: Recordings are saved in `./backend_recordings`, and the database in `./backend_data`.

## Web UI Features

### Dashboard
The central management hub.
- **Task Management**: Create, edit, and delete recording tasks.
- **Control**: Start and stop recordings.
- **Interact**: Remote control browsers via the "Interact" button.

### Live Monitor
Displays real-time previews of currently recording tasks in a grid layout. Useful for checking the status of active recordings at a glance.

### Archives
A library of recorded video files.
- **Download**: Save files locally.
- **Delete**: Remove unwanted recordings.

## License

[MIT License](LICENSE)
