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

	"github.com/gorilla/websocket"
	"github.com/nullpo7z/dashboard-recorder/internal/config"
	"github.com/nullpo7z/dashboard-recorder/internal/database"
	"github.com/playwright-community/playwright-go"
	"golang.org/x/exp/slog"
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
		return nil, fmt.Errorf("could not start playwright: %w", err)
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
		pw.Stop()
		return nil, fmt.Errorf("could not launch browser: %w", err)
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
func (w *Worker) StartRecording(ctx context.Context, taskID int64, url string, recordingID int64, outputPath string, customCSS string, fps int64) error {
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

		err := w.recordLoop(recCtx, taskID, url, outputPath, customCSS, fps)

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

func (w *Worker) recordLoop(ctx context.Context, taskID int64, url, outputPath, customCSS string, fps int64) error {
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

	// Inject Custom CSS if present
	if customCSS != "" {
		if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{
			Content: playwright.String(customCSS),
		}); err != nil {
			log.Printf("Failed to inject custom CSS for task %d: %v", taskID, err)
			// Continue recording even if CSS fails
		}
	}

	// Start FFmpeg
	// Using "ultrafast" and "crf 28" for low CPU usage during live capture
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
				Quality: playwright.Int(70),
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

// CapturePreview records a 5-second, 5fps video of the target URL with optional custom CSS.
func (w *Worker) CapturePreview(url, customCSS string) ([]byte, error) {
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

	// Navigate with timeout
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("nav failed: %w", err)
	}

	// Inject CSS
	if customCSS != "" {
		if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{
			Content: playwright.String(customCSS),
		}); err != nil {
			return nil, fmt.Errorf("css injection failed: %w", err)
		}
	}

	// Prepare temp file for output
	tmpFile := fmt.Sprintf("/tmp/preview_%d.mp4", time.Now().UnixNano())
	defer os.Remove(tmpFile)

	// Start FFmpeg
	// Input: mjpeg stream from stdin at 5 fps
	// Output: mp4 file
	ffmpegCmd := exec.Command("ffmpeg",
		"-y",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-r", "5", // Input FPS
		"-i", "-",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-preset", "ultrafast",
		"-r", "5", // Output FPS
		tmpFile,
	)

	stdin, err := ffmpegCmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdin failed: %w", err)
	}

	if err := ffmpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start failed: %w", err)
	}

	// Capture loop: 5 seconds @ 5 fps = 25 frames
	// Interval = 200ms
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	frameCount := 0
	maxFrames := 25 // 5 seconds * 5 fps

	for frameCount < maxFrames {
		<-ticker.C
		screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
			Type:    playwright.ScreenshotTypeJpeg,
			Quality: playwright.Int(70), // Lower quality for speed
		})
		if err != nil {
			log.Printf("preview screenshot failed: %v", err)
			continue
		}
		if _, err := stdin.Write(screenshot); err != nil {
			log.Printf("ffmpeg write failed: %v", err)
			break
		}
		frameCount++
	}

	// Close stdin to finish encoding
	stdin.Close()

	// Wait for FFmpeg
	if err := ffmpegCmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg wait failed: %w", err)
	}

	// Read result
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read preview file failed: %w", err)
	}

	return data, nil
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
