package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"net"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/nullpo7z/dashboard-recorder/internal/config"
	"github.com/nullpo7z/dashboard-recorder/internal/database"
	"github.com/playwright-community/playwright-go"
	"golang.org/x/exp/slog"
)

const (
	// DefaultJpegQuality is the fallback quality if calculation fails
	DefaultJpegQuality = 70
	// MaxJpegQuality Caps the maximum JPEG quality to prevent resource exhaustion (DoS)
	// Pure lossless (100) can cause massive file sizes.
	MaxJpegQuality = 95
	// MinJpegQuality ensures the image is still somewhat recognizable
	MinJpegQuality = 30
)

type Worker struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	config  *config.Config
	queries *database.Queries

	// Active sessions
	mu       sync.Mutex
	sessions map[int64]context.CancelFunc

	// Live preview frame cache (zero-overhead: reuse recording frames)
	framesMu     sync.RWMutex
	latestFrames map[int64][]byte // taskID -> latest JPEG bytes
}

func New(cfg *config.Config, q *database.Queries) (*Worker, error) {
	// Initialize Playwright
	// Use RunWithOptions to preventing it from trying to download browsers or install drivers if they are missing
	// since we manually installed them or are using system ones.
	pw, err := playwright.Run(&playwright.RunOptions{
		SkipInstallBrowsers: true,
	})
	if err != nil {
		// SOFT FAIL for OIDC Testing: Log warning and continue without recorder
		log.Printf("WARNING: Could not start Playwright: %v. Recorder features will be disabled.", err)
		return &Worker{
			config:       cfg,
			queries:      q,
			sessions:     make(map[int64]context.CancelFunc),
			latestFrames: make(map[int64][]byte),
		}, nil
	}

	opts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args: []string{
			"--no-sandbox",
			"--disable-setuid-sandbox",
			"--disable-dev-shm-usage",
		},
	}

	if cfg.PlaywrightPath != "" {
		opts.ExecutablePath = playwright.String(cfg.PlaywrightPath)
	} else if _, err := os.Stat("/usr/bin/chromium"); err == nil {
		opts.ExecutablePath = playwright.String("/usr/bin/chromium")
	}

	browser, err := pw.Chromium.Launch(opts)
	if err != nil {
		log.Printf("WARNING: Could not launch browser: %v. Recorder features will be disabled.", err)
		pw.Stop()
		return &Worker{
			pw:           pw,
			config:       cfg,
			queries:      q,
			sessions:     make(map[int64]context.CancelFunc),
			latestFrames: make(map[int64][]byte),
		}, nil
	}

	return &Worker{
		pw:           pw,
		browser:      browser,
		config:       cfg,
		queries:      q,
		sessions:     make(map[int64]context.CancelFunc),
		latestFrames: make(map[int64][]byte),
	}, nil
}

func (w *Worker) Stop() {
	w.mu.Lock()
	for id, cancel := range w.sessions {
		cancel()
		delete(w.sessions, id)
	}
	w.mu.Unlock()

	if w.browser != nil {
		w.browser.Close()
	}
	if w.pw != nil {
		w.pw.Stop()
	}
}

// StartRecording initiates a recording session.
func (w *Worker) StartRecording(ctx context.Context, taskID int64, url string, recordingID int64, outputPath string, customCSS string, fps int64, crf int64, timeOverlay bool, timeOverlayConfig string) error {
	w.mu.Lock()
	if _, exists := w.sessions[taskID]; exists {
		w.mu.Unlock()
		return fmt.Errorf("recording already in progress for task %d", taskID)
	}
	w.mu.Unlock()

	// Pre-flight Check: Write Permissions
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	f, err := os.CreateTemp(dir, "perm_check")
	if err != nil {
		return fmt.Errorf("write permission denied for %s: %w", dir, err)
	}
	f.Close()
	os.Remove(f.Name())

	// Use WithCancel for the recording lifecycle (controlled by StopRecording or internal error)
	// We detach from the caller's request context because recording runs in background.
	recCtx, cancel := context.WithCancel(context.Background())

	w.mu.Lock()
	w.sessions[taskID] = cancel
	w.mu.Unlock()

	// Launch storage path (provided by caller now)

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.sessions, taskID)
			w.mu.Unlock()

			// Clean up frame cache to prevent memory leaks
			w.framesMu.Lock()
			delete(w.latestFrames, taskID)
			w.framesMu.Unlock()
		}()

		if fps > 30 {
			slog.Info("High FPS recording started", "task_id", taskID, "fps", fps, "warning", "Significant disk usage expected")
		}

		err := w.recordLoop(recCtx, taskID, url, outputPath, customCSS, fps, crf, timeOverlay, timeOverlayConfig)

		status := "COMPLETED"
		if err != nil {
			log.Printf("Recording %d failed: %v", recordingID, err)
			status = "FAILED"
			// In a real app we'd save error message too
		}

		// Update DB
		// Note: We need a background context here as the session ctx is cancelled
		_ = w.queries.UpdateRecordingStatus(context.Background(), database.UpdateRecordingStatusParams{
			Status: status,
			ID:     recordingID,
		})
	}()

	return nil
}

func (w *Worker) StopRecording(taskID int64) error {
	w.mu.Lock()
	cancel, exists := w.sessions[taskID]
	w.mu.Unlock()

	if !exists {
		return fmt.Errorf("no active recording for task %d", taskID)
	}

	cancel() // Signal loop to stop
	return nil
}

func (w *Worker) recordLoop(ctx context.Context, taskID int64, url, outputPath, customCSS string, fps int64, crf int64, timeOverlay bool, timeOverlayConfig string) error {
	opts := playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: 1920, Height: 1080},
		BypassCSP:         playwright.Bool(true),
		IgnoreHttpsErrors: playwright.Bool(true),
	}

	// Load session if exists
	sessionFile := fmt.Sprintf("/app/data/sessions/task_%d.json", taskID)
	if _, err := os.Stat(sessionFile); err == nil {
		opts.StorageStatePath = playwright.String(sessionFile)
		log.Printf("Loaded session from %s", sessionFile)
	}

	bCtx, err := w.browser.NewContext(opts)
	if err != nil {
		return err
	}
	defer bCtx.Close()

	page, err := bCtx.NewPage()
	if err != nil {
		return err
	}

	// Navigate
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(60000),
	}); err != nil {
		return fmt.Errorf("nav failed: %w", err)
	}

	// Inject Time Overlay if enabled
	if timeOverlay {
		if err := w.InjectTimeOverlay(page, timeOverlayConfig, w.config.NtpServer); err != nil {
			log.Printf("Failed to inject time overlay for task %d: %v", taskID, err)
			// Continue recording even if overlay fails
		}
	}

	// Inject Custom CSS if present
	if customCSS != "" {
		if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{
			Content: playwright.String(customCSS),
		}); err != nil {
			log.Printf("Failed to inject custom CSS for task %d: %v", taskID, err)
			// Continue recording even if CSS fails
		}
	}

	// Calculate JPEG quality based on CRF
	jpegQuality := calculateJpegQuality(crf)
	slog.Info("Starting recording loop",
		"task_id", taskID,
		"crf", crf,
		"jpeg_quality", jpegQuality,
		"time_overlay", timeOverlay,
	)

	// Start FFmpeg
	// Using "ultrafast" and configurable CRF for cpu/quality balance
	// Use exec.Command instead of CommandContext so context cancellation doesn't kill it immediately
	// Start FFmpeg
	// Use exec.Command instead of CommandContext so we can manage graceful shutdown manually
	// FPS is configurable.
	ffmpegCmd := exec.Command("ffmpeg",
		"-y",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-r", fmt.Sprintf("%d", fps),
		"-i", "-",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-crf", fmt.Sprintf("%d", crf),
		"-r", fmt.Sprintf("%d", fps),
		outputPath,
	)

	stdin, err := ffmpegCmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := ffmpegCmd.Start(); err != nil {
		return err
	}

	// Wait for FFmpeg in a separate goroutine to avoid blocking close
	ffmpegDone := make(chan error)
	go func() {
		ffmpegDone <- ffmpegCmd.Wait()
	}()

	// Ticker for screenshots
	// We aim for the target FPS, but if capture is slow, we calculate how many frames
	// "should" have passed and duplicate the screenshot to maintain A/V sync (wall clock time).
	frameIntervalMs := 1000.0 / float64(fps)
	ticker := time.NewTicker(time.Duration(frameIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	var framesSent int64 = 0

	for {
		select {
		case <-ctx.Done():
			// Stop signal received. Close stdin to flush FFmpeg.
			stdin.Close()

			// Wait for FFmpeg to finish gracefully, with a timeout
			select {
			case err := <-ffmpegDone:
				return err
			case <-time.After(5 * time.Second):
				// Force kill if it doesn't shut down
				if ffmpegCmd.Process != nil {
					ffmpegCmd.Process.Kill()
				}
				return fmt.Errorf("ffmpeg shutdown timed out")
			}
		case <-ticker.C:
			// Capture
			buf, err := page.Screenshot(playwright.PageScreenshotOptions{
				Type:    playwright.ScreenshotTypeJpeg,
				Quality: playwright.Int(jpegQuality),
			})
			if err != nil {
				log.Printf("screenshot error: %v", err)
				continue
			}

			// Cache frame for live preview (zero-overhead: reuse same bytes)
			w.framesMu.Lock()
			w.latestFrames[taskID] = buf
			w.framesMu.Unlock()

			// Calculate how many frames we need to send to match wall clock time
			elapsed := time.Since(startTime).Seconds()
			expectedFrames := int64(elapsed * float64(fps))

			// Always send at least one frame if we captured one, to ensure progress,
			// but theoretically if we are super fast we might skip?
			// In practice, capture is slow, so we usually need >= 1.
			duplicates := expectedFrames - framesSent
			if duplicates < 1 {
				duplicates = 1
			}

			// Write to FFmpeg stdin (duplicated as needed)
			for i := int64(0); i < duplicates; i++ {
				if _, err := stdin.Write(buf); err != nil {
					return err
				}
			}
			framesSent += duplicates
		}
	}
}

// GetLatestFrame returns the latest cached frame for a task (thread-safe)
// Returns nil if no frame is available
func (w *Worker) GetLatestFrame(taskID int64) []byte {
	w.framesMu.RLock()
	defer w.framesMu.RUnlock()

	frame, exists := w.latestFrames[taskID]
	if !exists {
		return nil
	}

	// Return a copy to prevent concurrent modification issues
	frameCopy := make([]byte, len(frame))
	copy(frameCopy, frame)
	return frameCopy
}

// validateURL performs strict validation to prevent SSRF
func validateURL(targetURL string) error {
	u, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid url format")
	}

	// 1. Check Protocol
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid protocol: %s", u.Scheme)
	}

	// 2. Resolve Hostname
	hostname := u.Hostname()
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}

	// 3. Check IP Addresses
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
			return fmt.Errorf("access to private IP %s is denied", ip.String())
		}
	}

	return nil
}

// CapturePreview captures a single JPEG screenshot of the target URL with optional custom CSS.
// It includes strict URL validation and timeouts.
func (w *Worker) CapturePreview(targetURL, customCSS string) ([]byte, error) {
	// 1. SSRF Protect
	if err := validateURL(targetURL); err != nil {
		return nil, fmt.Errorf("security check failed: %w", err)
	}

	// 2. Setup Context with Timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 3. Launch Browser Context (Incognito)
	// We use the context for the browser context creation if the library supported it,
	// but playwright-go's NewContext doesn't take a context.Context.
	// However, we should respect the timeout for the overall operation.
	// Since NewContext is fast, we just proceed.
	// We will use the context for the page navigation if possible, or just rely on the defer cancel.

	bCtx, err := w.browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: 1280, Height: 720},
		BypassCSP:         playwright.Bool(true),
		IgnoreHttpsErrors: playwright.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	defer bCtx.Close()

	page, err := bCtx.NewPage()
	if err != nil {
		return nil, err
	}

	// 4. Navigate
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if _, err := page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(20000), // 20s navigation timeout
	}); err != nil {
		return nil, fmt.Errorf("nav failed: %w", err)
	}

	// 5. Inject CSS
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if customCSS != "" {
		if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{
			Content: playwright.String(customCSS),
		}); err != nil {
			return nil, fmt.Errorf("css injection failed: %w", err)
		}
	}

	// 6. Capture Screenshot
	// We use a high quality JPEG for preview
	screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
		Type:    playwright.ScreenshotTypeJpeg,
		Quality: playwright.Int(80),
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	if len(screenshot) == 0 {
		return nil, fmt.Errorf("captured empty screenshot")
	}

	return screenshot, nil
}

// Interactive Event Types
type InteractionEvent struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
	Key  string  `json:"key"`
}

// HandleInteractive manages a remote control session via WebSocket.
func (w *Worker) HandleInteractive(ctx context.Context, taskID int64, url string, conn *websocket.Conn) error {
	defer conn.Close()

	// 1. Setup Browser Context with Persistent Storage
	storageDir := "/app/data/sessions"
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage dir: %w", err)
	}
	stateFile := fmt.Sprintf("%s/task_%d.json", storageDir, taskID)

	opts := playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: 1920, Height: 1080},
		BypassCSP:         playwright.Bool(true),
		IgnoreHttpsErrors: playwright.Bool(true),
	}
	// Load storage state if exists
	if _, err := os.Stat(stateFile); err == nil {
		opts.StorageStatePath = playwright.String(stateFile)
	}

	bCtx, err := w.browser.NewContext(opts)
	if err != nil {
		return fmt.Errorf("context creation failed: %w", err)
	}
	defer bCtx.Close()

	page, err := bCtx.NewPage()
	if err != nil {
		return fmt.Errorf("page creation failed: %w", err)
	}

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return fmt.Errorf("nav failed: %w", err)
	}

	// 2. Stream Loop (Send Screenshots)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // 10 FPS
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
					Type:    playwright.ScreenshotTypeJpeg,
					Quality: playwright.Int(60), // Low quality for speed
				})
				if err != nil {
					continue
				}
				// Send as Binary Message
				if err := conn.WriteMessage(websocket.BinaryMessage, screenshot); err != nil {
					return
				}
			}
		}
	}()

	// 3. Command Loop (Receive Inputs)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var event InteractionEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			log.Printf("Invalid event: %v", err)
			continue
		}

		switch event.Type {
		case "click":
			if err := page.Mouse().Click(event.X, event.Y); err != nil {
				log.Printf("Click failed: %v", err)
			}
		case "type":
			if err := page.Keyboard().Type(event.Text); err != nil {
				log.Printf("Type failed: %v", err)
			}
		case "key":
			if err := page.Keyboard().Press(event.Key); err != nil {
				log.Printf("Key press failed: %v", err)
			}
		case "save":
			// Save Storage State
			if _, err := bCtx.StorageState(stateFile); err != nil {
				log.Printf("Failed to save state: %v", err)
			} else {
				log.Printf("Session state saved to %s", stateFile)
			}
			return nil // Exit session loop, triggering defer conn.Close()
		}
	}
}

// calculateJpegQuality determines the JPEG quality (0-100) based on the CRF value (0-51).
// Lower CRF means higher quality.
// The formula is roughly: Quality = 100 - (CRF / 2).
// We cap the quality at MaxJpegQuality (95) to prevent DoS via massive file sizes.
func calculateJpegQuality(crf int64) int {
	// 1. Sanitize Input
	if crf < 0 {
		crf = 0 // Treat negative as 0 (highest quality)
	}
	if crf > 51 {
		crf = 51 // Clamp to max valid CRF
	}

	// 2. Calculate Quality
	// CRF 0 -> 100
	// CRF 23 -> ~88
	// CRF 51 -> ~74
	// usage of float to ensure better precision before casting back
	quality := 100 - (float64(crf) / 2.0)

	// 3. Apply Constraints
	qInt := int(quality)

	if qInt > MaxJpegQuality {
		qInt = MaxJpegQuality
	}
	if qInt < MinJpegQuality {
		qInt = MinJpegQuality
	}

	return qInt
}

// InjectTimeOverlay injects a time overlay into the page, synchronized with NTP.
func (w *Worker) InjectTimeOverlay(page playwright.Page, config string, ntpServer string) error {
	// 1. Get NTP Offset
	offset, err := GetNTPTime(ntpServer)
	if err != nil {
		slog.Error("NTP query failed, falling back to system time", "error", err)
		offset = 0
	}

	// 2. Validate Config
	validConfigs := map[string]bool{
		"top-left":     true,
		"top-right":    true,
		"bottom-left":  true,
		"bottom-right": true,
	}
	if !validConfigs[config] {
		config = "bottom-right" // Default
	}

	// 3. Prepare Injection Script
	// We use JSON.stringify to safely inject values into the script
	offsetMs := offset.Milliseconds()
	scriptTemplate := `
		(function() {
			const offsetMs = %d;
			const position = "%s";

			const div = document.createElement('div');
			div.id = 'uniquetimeoverlay';
			div.style.position = 'fixed';
			div.style.padding = '4px 8px';
			div.style.backgroundColor = 'rgba(0, 0, 0, 0.5)';
			div.style.color = 'white';
			div.style.fontSize = '14px';
			div.style.fontFamily = 'monospace';
			div.style.zIndex = '9999';
			div.style.pointerEvents = 'none';

			// Tailwind-like positioning
			if (position === 'top-left') {
				div.style.top = '10px';
				div.style.left = '10px';
			} else if (position === 'top-right') {
				div.style.top = '10px';
				div.style.right = '10px';
			} else if (position === 'bottom-left') {
				div.style.bottom = '10px';
				div.style.left = '10px';
			} else { // bottom-right
				div.style.bottom = '10px';
				div.style.right = '10px';
			}

			document.body.appendChild(div);

			function updateTime() {
				const now = new Date(Date.now() + offsetMs);
				// Format: YYYY-MM-DD HH:mm:ss.SSS
				const iso = now.toISOString(); // 2023-10-05T14:48:00.000Z
				// Convert to local time string or keep UTC? User requirement implies "current time".
				// Usually local time is preferred for display.
				// However, NTP gives UTC time generally.
				// We'll stick to local time of the browser (container) + offset.
				// Actually, offset applies to the timestamp.
				
				// Let's use a custom formatter for stability
				const pad = (n) => n.toString().padStart(2, '0');
				const pad3 = (n) => n.toString().padStart(3, '0');
				
				const YYYY = now.getFullYear();
				const MM = pad(now.getMonth() + 1);
				const DD = pad(now.getDate());
				const HH = pad(now.getHours());
				const mm = pad(now.getMinutes());
				const ss = pad(now.getSeconds());
				const SSS = pad3(now.getMilliseconds());

				div.textContent = ` + "`" + `${YYYY}-${MM}-${DD} ${HH}:${mm}:${ss}.${SSS}` + "`" + `;
			}

			setInterval(updateTime, 16); // ~60fps update
			updateTime();
		})();
	`

	script := fmt.Sprintf(scriptTemplate, offsetMs, config)

	// 4. Inject
	if err := page.AddInitScript(playwright.Script{
		Content: playwright.String(script),
	}); err != nil {
		return fmt.Errorf("failed to inject time overlay script: %w", err)
	}

	return nil
}
