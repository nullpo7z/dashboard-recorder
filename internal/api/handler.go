package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nullpo7z/dashboard-recorder/internal/auth"
	"github.com/nullpo7z/dashboard-recorder/internal/config"
	"github.com/nullpo7z/dashboard-recorder/internal/database"
	"github.com/nullpo7z/dashboard-recorder/internal/recorder"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

type Handler struct {
	Queries  *database.Queries
	Config   *config.Config
	Recorder *recorder.Worker
	DB       *sql.DB

	// Rate Limiter
	limiter     *rate.Limiter
	limiterMu   sync.Mutex
	lastCleanup time.Time
	clients     map[string]*rate.Limiter

	// Ticket Store
	TicketStore auth.TicketStore
}

func New(q *database.Queries, cfg *config.Config, rec *recorder.Worker, db *sql.DB) *Handler {
	h := &Handler{
		Queries:     q,
		Config:      cfg,
		Recorder:    rec,
		DB:          db,
		clients:     make(map[string]*rate.Limiter),
		TicketStore: auth.NewInMemoryTicketStore(),
	}

	// Initialize admin user if needed
	go h.initAdminUser()

	// Start ticket cleanup routine
	h.TicketStore.StartCleanupLoop(context.Background(), 1*time.Minute)

	return h
}

// initAdminUser creates a default admin user if none exists
func (h *Handler) initAdminUser() {
	ctx := context.Background()
	count, err := h.Queries.CountUsers(ctx)
	if err != nil {
		fmt.Printf("CRITICAL: Failed to count users: %v\n", err)
		return
	}

	if count == 0 {
		// Create default admin
		password := "admin" // Default password
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			fmt.Printf("CRITICAL: Failed to hash default password: %v\n", err)
			return
		}

		_, err = h.Queries.CreateUser(ctx, database.CreateUserParams{
			Username:     "admin",
			PasswordHash: string(hashed),
		})
		if err != nil {
			fmt.Printf("CRITICAL: Failed to create default admin: %v\n", err)
			return
		}
		fmt.Println("WARNING: Created default 'admin' user with password 'admin'. Please change this immediately.")
	} else {
		// Ensure admin exists, but DO NOT overwrite password
		_, err := h.Queries.GetUserByUsername(ctx, "admin")
		if err == sql.ErrNoRows {
			// If other users exist but not admin, maybe create it?
			// Logic says check count. If > 0 and no admin, strange but okay.
			// We stick to simple Count check as requested.
		}
	}
}

// RateLimitMiddleware enforces simple IP-based rate limiting
func (h *Handler) RateLimitMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// fmt.Println("DEBUG: Entering RateLimitMiddleware") // Reduced log spam
		ip := c.RealIP()

		h.limiterMu.Lock()
		defer h.limiterMu.Unlock()

		// Cleanup old clients every minute
		if time.Since(h.lastCleanup) > time.Minute {
			// Simplified cleanup for demo
			h.lastCleanup = time.Now()
		}

		limiter, exists := h.clients[ip]
		if !exists {
			// 5 requests per minute (refill 1 every 12s), burst 5
			limiter = rate.NewLimiter(rate.Every(time.Minute/5), 5)
			h.clients[ip] = limiter
		}

		if !limiter.Allow() {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Too many requests"})
		}

		// fmt.Println("DEBUG: Exiting RateLimitMiddleware")
		return next(c)
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	// Fetch user from DB
	user, err := h.Queries.GetUserByUsername(c.Request().Context(), req.Username)
	if err != nil {
		if err == sql.ErrNoRows {
			// Timing mitigation: fake hash comparison
			bcrypt.CompareHashAndPassword([]byte("$2a$10$abcdefghijklmnopqrstuv"), []byte(req.Password))
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}

	// Create JWT
	now := time.Now()
	// Reduced usage for security
	exp := now.Add(time.Hour * 24)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user": req.Username,
		"exp":  jwt.NewNumericDate(exp),
	})

	t, err := token.SignedString([]byte(h.Config.JWTSecret))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"token": t})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) ChangePassword(c echo.Context) error {
	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	// Get user from JWT
	userToken, ok := c.Get("user").(*jwt.Token)
	if !ok || userToken == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	}

	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
	}

	username, ok := claims["user"].(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid user claim"})
	}

	req.OldPassword = strings.TrimSpace(req.OldPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

	// 1. Password Policy Enforcment
	if len(req.NewPassword) < 12 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "New password must be at least 12 characters long"})
	}
	if req.NewPassword == req.OldPassword {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "New password must be different from the old password"})
	}

	// 2. Verify Old Password
	user, err := h.Queries.GetUserByUsername(c.Request().Context(), username)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "user not found"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Incorrect old password"})
	}

	// 3. Hash New Password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
	}

	// 4. Update DB
	if err := h.Queries.UpdateUserPassword(c.Request().Context(), database.UpdateUserPasswordParams{
		PasswordHash: string(hashed),
		Username:     username,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "password updated"})
}

// Authenticated route to generate a one-time ticket
// Authenticated route to generate a one-time ticket
func (h *Handler) GenerateTicket(c echo.Context) error {
	// Rate Limiting (Defense in Depth) is handled by middleware if applied,
	// but we can also check global limiter here if needed.

	// Get user from JWT
	userToken, ok := c.Get("user").(*jwt.Token)
	if !ok || userToken == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	}
	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
	}
	username, ok := claims["user"].(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid user claim"})
	}

	// Parse Request
	type TicketRequest struct {
		TaskID int64 `json:"task_id"`
	}
	var req TicketRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate Task Exists (RBAC check could be extended here)
	_, err := h.Queries.GetTask(c.Request().Context(), req.TaskID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
	}

	// Generate Ticket via Store (Atomic, Secure)
	ticket, err := h.TicketStore.Generate(username, req.TaskID, 30*time.Second)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate ticket"})
	}

	return c.JSON(http.StatusOK, map[string]string{"ticket": ticket.TicketID})
}

type TaskDTO struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	TargetURL        string    `json:"target_url"`
	IsEnabled        bool      `json:"is_enabled"`
	CreatedAt        time.Time `json:"created_at"`
	CustomCSS        string    `json:"custom_css"`
	Fps              int64     `json:"fps"`
	Crf              int64     `json:"crf"`
	FilenameTemplate string    `json:"filename_template"`
}

func (h *Handler) CreateTask(c echo.Context) error {
	type CreateTaskRequest struct {
		Name             string `json:"name"`
		TargetURL        string `json:"target_url"`
		FilenameTemplate string `json:"filename_template"`
		CustomCSS        string `json:"custom_css"`
		Fps              *int64 `json:"fps"`
		Crf              *int64 `json:"crf"`
	}

	var req CreateTaskRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Input Validation
	// 1. Target URL
	if _, err := url.ParseRequestURI(req.TargetURL); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid target_url"})
	}

	// 2. Filename Template (Path Traversal Prevention)
	if req.FilenameTemplate != "" {
		// Allow alphanumeric, underscore, dot, dash.
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9_.-]+$`, req.FilenameTemplate)
		if !matched {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename_template contains invalid characters. Allowed: a-z, A-Z, 0-9, _, ., -"})
		}
		// Explicitly reject traversal and separators
		if strings.Contains(req.FilenameTemplate, "..") || strings.Contains(req.FilenameTemplate, "/") || strings.Contains(req.FilenameTemplate, "\\") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename_template cannot contain path traversal or separators"})
		}
	}

	// 3. FPS Validation
	var fps int64 = 5 // Default
	if req.Fps != nil {
		fps = *req.Fps
		if fps < 1 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "fps must be >= 1"})
		}
		if fps > 15 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "fps cannot exceed 15"})
		}
		if int(fps) > h.Config.MaxFpsLimit {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("fps cannot exceed server limit of %d", h.Config.MaxFpsLimit)})
		}
	}

	// 4. CRF Validation
	var crf int64 = 23 // Default
	if req.Crf != nil {
		crf = *req.Crf
		if crf < 0 || crf > 51 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "crf must be between 0 and 51"})
		}
		if crf < 15 {
			fmt.Printf("Warning: Task '%s' created with very high quality (CRF %d). Large file sizes expected.\n", req.Name, crf)
		}
	}

	params := database.CreateTaskParams{
		Name:             req.Name,
		TargetUrl:        req.TargetURL,
		FilenameTemplate: req.FilenameTemplate,
		CustomCss:        req.CustomCSS,
		Fps:              fps,
		Crf:              crf,
	}

	task, err := h.Queries.CreateTask(c.Request().Context(), params)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, TaskDTO{
		ID:               task.ID,
		Name:             task.Name,
		TargetURL:        task.TargetUrl,
		IsEnabled:        task.IsEnabled,
		CreatedAt:        task.CreatedAt,
		Fps:              task.Fps,
		Crf:              task.Crf,
		CustomCSS:        task.CustomCss,
		FilenameTemplate: task.FilenameTemplate,
	})
}

func (h *Handler) ListTasks(c echo.Context) error {
	tasks, err := h.Queries.ListTasks(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	dtos := make([]TaskDTO, len(tasks))
	for i, t := range tasks {
		dtos[i] = TaskDTO{
			ID:               t.ID,
			Name:             t.Name,
			TargetURL:        t.TargetUrl,
			IsEnabled:        t.IsEnabled,
			CreatedAt:        t.CreatedAt,
			Fps:              t.Fps,
			Crf:              t.Crf,
			CustomCSS:        t.CustomCss,
			FilenameTemplate: t.FilenameTemplate,
		}
	}
	return c.JSON(http.StatusOK, dtos)
}

// StartTask enables the task and starts the worker
func (h *Handler) StartTask(c echo.Context) error {
	idParam := c.Param("id")
	var taskID int64
	if _, err := fmt.Sscanf(idParam, "%d", &taskID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	// 1. Enable Task in DB
	if err := h.Queries.EnableTask(c.Request().Context(), taskID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to enable task: %v", err)})
	}

	// 2. Fetch task details
	task, err := h.Queries.GetTask(c.Request().Context(), taskID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
	}

	// 3. Generate Filename
	timestamp := time.Now().Format("20060102150405")
	var filename string
	if task.FilenameTemplate != "" {
		// Defense-in-depth: Fallback sanitization
		safeTemplate := filepath.Base(task.FilenameTemplate)
		filename = fmt.Sprintf("%s_%s.mkv", safeTemplate, timestamp)
	} else {
		// Fallback to legacy ID_TIMESTAMP format if no template
		filename = fmt.Sprintf("%d_%d.mkv", taskID, time.Now().Unix())
	}
	fullPath := fmt.Sprintf("/app/recordings/%s", filename)

	// 4. Create Recording Entry
	rec, err := h.Queries.CreateRecording(c.Request().Context(), database.CreateRecordingParams{
		TaskID:   taskID,
		Status:   "RECORDING",
		FilePath: fullPath,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create recording log: %v", err)})
	}

	// 5. Start Worker
	if err := h.Recorder.StartRecording(c.Request().Context(), taskID, task.TargetUrl, rec.ID, fullPath, task.CustomCss, task.Fps, task.Crf); err != nil {
		// Update status to failed
		_ = h.Queries.UpdateRecordingStatus(c.Request().Context(), database.UpdateRecordingStatusParams{
			Status: "FAILED",
			ID:     rec.ID,
		})
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to start worker: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "started", "recording_id": fmt.Sprintf("%d", rec.ID)})
}

// StopTask disables the task and stops the worker
func (h *Handler) StopTask(c echo.Context) error {
	idParam := c.Param("id")
	var taskID int64
	if _, err := fmt.Sscanf(idParam, "%d", &taskID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	// 1. Disable Task in DB
	if err := h.Queries.DisableTask(c.Request().Context(), taskID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to disable task: %v", err)})
	}

	// 2. Stop Worker
	// We ignore error if "no active recording" because we just want to ensure it's stopped.
	if err := h.Recorder.StopRecording(taskID); err != nil {
		// Log but don't fail the request if it was already stopped
		fmt.Printf("StopTask: worker stop warning: %v\n", err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handler) UpdateTask(c echo.Context) error {
	idParam := c.Param("id")
	var taskID int64
	if _, err := fmt.Sscanf(idParam, "%d", &taskID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	type UpdateTaskRequest struct {
		Name             string `json:"name"`
		TargetURL        string `json:"target_url"`
		FilenameTemplate string `json:"filename_template"`
		CustomCSS        string `json:"custom_css"`
		Fps              *int64 `json:"fps"`
		Crf              *int64 `json:"crf"`
	}

	var req UpdateTaskRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Input Validation (Reuse logic from CreateTask, should ideally be shared)
	if _, err := url.ParseRequestURI(req.TargetURL); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid target_url"})
	}

	// 2. Filename Template (Path Traversal Prevention)
	if req.FilenameTemplate != "" {
		// Allow alphanumeric, underscore, dot, dash.
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9_.-]+$`, req.FilenameTemplate)
		if !matched {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename_template contains invalid characters. Allowed: a-z, A-Z, 0-9, _, ., -"})
		}
		// Explicitly reject traversal and separators
		if strings.Contains(req.FilenameTemplate, "..") || strings.Contains(req.FilenameTemplate, "/") || strings.Contains(req.FilenameTemplate, "\\") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename_template cannot contain path traversal or separators"})
		}
	}

	// 3. FPS Validation
	var fps int64 = 5
	if req.Fps != nil {
		fps = *req.Fps
		if fps < 1 || fps > 15 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "fps must be between 1 and 15"})
		}
		if int(fps) > h.Config.MaxFpsLimit {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("fps cannot exceed server limit of %d", h.Config.MaxFpsLimit)})
		}
	}

	// 4. CRF Validation
	var crf int64 = 23
	if req.Crf != nil {
		crf = *req.Crf
		if crf < 0 || crf > 51 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "crf must be between 0 and 51"})
		}
		if crf < 15 {
			fmt.Printf("Warning: Task '%s' updated with very high quality (CRF %d). Large file sizes expected.\n", req.Name, crf)
		}
	}

	err := h.Queries.UpdateTask(c.Request().Context(), database.UpdateTaskParams{
		Name:             req.Name,
		TargetUrl:        req.TargetURL,
		FilenameTemplate: req.FilenameTemplate,
		CustomCss:        req.CustomCSS,
		Fps:              fps,
		Crf:              crf,
		ID:               taskID,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) DeleteTask(c echo.Context) error {
	idParam := c.Param("id")
	var taskID int64
	if _, err := fmt.Sscanf(idParam, "%d", &taskID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	// Stop any active recording first
	_ = h.Recorder.StopRecording(taskID)

	if err := h.Queries.DeleteTask(c.Request().Context(), taskID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// Public routes with Rate Limiting
	e.POST("/api/login", h.Login, h.RateLimitMiddleware)

	g := e.Group("/api")
	g.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// CSP: Strict security policy
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' ws: wss:;")
			// Security Headers
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			return next(c)
		}
	})

	config := echojwt.Config{
		TokenLookup: "header:Authorization",
		ParseTokenFunc: func(c echo.Context, auth string) (interface{}, error) {
			// Support "Bearer " prefix
			if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
				auth = auth[7:]
			}
			return jwt.Parse(auth, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(h.Config.JWTSecret), nil
			})
		},
		Skipper: func(c echo.Context) bool {
			// Skip for OPTIONS (CORS preflight) and WebSocket Ticket auth
			if c.Request().Method == "OPTIONS" {
				return true
			}
			if strings.HasSuffix(c.Path(), "/interact") {
				return true
			}
			return false
		},
	}

	g.Use(echojwt.WithConfig(config))

	g.POST("/tasks", h.CreateTask)
	g.GET("/tasks", h.ListTasks)
	g.POST("/tasks/:id/start", h.StartTask)
	g.POST("/tasks/:id/stop", h.StopTask)
	g.PUT("/tasks/:id", h.UpdateTask)
	g.DELETE("/tasks/:id", h.DeleteTask)
	g.GET("/archives", h.ListArchives)
	g.GET("/stats", h.GetStats)

	// Tickets
	// Tickets
	g.POST("/tickets", h.GenerateTicket, h.RateLimitMiddleware)

	// Password Change with Rate Limiting
	g.POST("/password", h.ChangePassword, h.RateLimitMiddleware)

	// Live monitor endpoints
	g.GET("/recordings/live", h.GetLiveRecordings)
	g.GET("/recordings/:id/preview.jpg", h.GetRecordingPreview)
	g.DELETE("/recordings/:id", h.DeleteRecording)
	g.POST("/tasks/preview", h.PreviewTask)
	g.GET("/tasks/:id/interact", h.WsInteractive)
}

func (h *Handler) PreviewTask(c echo.Context) error {
	type PreviewRequest struct {
		TargetURL string `json:"target_url"`
		CustomCSS string `json:"custom_css"`
	}
	var req PreviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if req.TargetURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "target_url is required"})
	}

	img, err := h.Recorder.CapturePreview(req.TargetURL, req.CustomCSS)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.Blob(http.StatusOK, "video/mp4", img)
}

// WebSocket handler for interactive session
func (h *Handler) WsInteractive(c echo.Context) error {
	// 1. Get Ticket from Query
	ticketID := c.QueryParam("ticket")
	if ticketID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing ticket"})
	}

	// 2. Exchange Ticket (Atomic Check-and-Burn)
	ticket, err := h.TicketStore.Exchange(ticketID)
	if err != nil {
		// Return 401 for invalid/expired tickets
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired ticket"})
	}

	// 3. Get TaskID
	idParam := c.Param("id")
	var taskID int64
	if _, err := fmt.Sscanf(idParam, "%d", &taskID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid task id"})
	}

	// 4. Validate Authorization (RBAC: Ticket must match requested TaskID)
	if ticket.TaskID != taskID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "ticket mismatch"})
	}

	// 5. Check Task Exists and Get Details
	task, err := h.Queries.GetTask(c.Request().Context(), taskID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "task not found"})
	}

	// 6. Strict Upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Strict same-origin policy to prevent CSWSH
			origin := r.Header.Get("Origin")
			if origin == "" {
				// No Origin header (non-browser client) -> verification relies on Ticket
				return true
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			// Compare Host (including port if present)
			return strings.EqualFold(u.Host, r.Host)
		},
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	// 7. Handle Interactive Session
	return h.Recorder.HandleInteractive(c.Request().Context(), taskID, task.TargetUrl, ws)
}

func (h *Handler) DeleteRecording(c echo.Context) error {
	idParam := c.Param("id")
	var recID int64
	if _, err := fmt.Sscanf(idParam, "%d", &recID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid recording id"})
	}

	// 1. Get Recording to find file path
	rec, err := h.Queries.GetRecording(c.Request().Context(), recID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "recording not found"})
	}

	// 2. Delete file from disk
	if rec.FilePath != "" {
		if err := os.Remove(rec.FilePath); err != nil {
			fmt.Printf("Warning: failed to delete file %s: %v\n", rec.FilePath, err)
			// Continue to delete DB record even if file delete fails (maybe already gone)
		}
	}

	// 3. Delete from DB
	if err := h.Queries.DeleteRecording(c.Request().Context(), recID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

type RecordingDTO struct {
	ID        int64      `json:"id"`
	TaskID    int64      `json:"task_id"`
	Status    string     `json:"status"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	FilePath  string     `json:"file_path"`
	TaskName  string     `json:"task_name,omitempty"`
	Size      string     `json:"size"`
}

func (h *Handler) ListArchives(c echo.Context) error {
	recs, err := h.Queries.ListRecordings(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	dtos := make([]RecordingDTO, len(recs))
	for i, r := range recs {
		var endTime *time.Time
		if r.EndTime.Valid {
			endTime = &r.EndTime.Time
		}

		// Calculate file size
		sizeStr := "0 B"
		if info, err := os.Stat(r.FilePath); err == nil {
			size := info.Size()
			const unit = 1024
			if size < unit {
				sizeStr = fmt.Sprintf("%d B", size)
			} else {
				div, exp := int64(unit), 0
				for n := size / unit; n >= unit; n /= unit {
					div *= unit
					exp++
				}
				sizeStr = fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
			}
		}

		dtos[i] = RecordingDTO{
			ID:        r.ID,
			TaskID:    r.TaskID,
			Status:    r.Status,
			StartTime: r.StartTime,
			EndTime:   endTime,
			FilePath:  r.FilePath,
			TaskName:  r.TaskName,
			Size:      sizeStr,
		}
	}

	return c.JSON(http.StatusOK, dtos)
}

func (h *Handler) GetStats(c echo.Context) error {
	stats := make(map[string]interface{})

	// Get CPU usage (average over 100ms)
	cpuPercents, err := cpu.Percent(100*time.Millisecond, false)
	if err == nil && len(cpuPercents) > 0 {
		stats["cpu_percent"] = cpuPercents[0]
	} else {
		stats["cpu_percent"] = 0.0
	}

	// Get Memory stats
	memStats, err := mem.VirtualMemory()
	if err == nil {
		stats["memory_percent"] = memStats.UsedPercent
	} else {
		stats["memory_percent"] = 0.0
	}

	// Get Disk stats for /app/recordings
	diskStats, err := disk.Usage("/app/recordings")
	if err == nil {
		stats["disk_percent"] = diskStats.UsedPercent
	} else {
		stats["disk_percent"] = 0.0
	}

	// Additional metadata
	stats["timestamp"] = time.Now().Unix()

	return c.JSON(http.StatusOK, stats)
}

// LiveRecordingDTO represents active recording with real-time stats
type LiveRecordingDTO struct {
	ID             int64  `json:"id"`
	TaskID         int64  `json:"task_id"`
	TaskName       string `json:"task_name"`
	Status         string `json:"status"`
	ElapsedSeconds int64  `json:"elapsed_seconds"`
	FileSizeBytes  int64  `json:"file_size_bytes"`
	HasPreview     bool   `json:"has_preview"`
}

// GetLiveRecordings returns all active recordings with real-time stats
func (h *Handler) GetLiveRecordings(c echo.Context) error {
	// For now, query all recordings and filter by status
	// When sqlc regenerates, we'll have ListActiveRecordings
	recs, err := h.Queries.ListRecordings(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var result []LiveRecordingDTO
	for _, rec := range recs {
		// Only include RECORDING status
		if rec.Status != "RECORDING" {
			continue
		}

		// Calculate elapsed seconds
		elapsed := int64(time.Since(rec.StartTime).Seconds())

		// Get file size
		var fileSize int64
		if info, err := os.Stat(rec.FilePath); err == nil {
			fileSize = info.Size()
		}

		// Check if preview is available
		hasPreview := h.Recorder.GetLatestFrame(rec.TaskID) != nil

		result = append(result, LiveRecordingDTO{
			ID:             rec.ID,
			TaskID:         rec.TaskID,
			TaskName:       rec.TaskName,
			Status:         rec.Status,
			ElapsedSeconds: elapsed,
			FileSizeBytes:  fileSize,
			HasPreview:     hasPreview,
		})
	}

	return c.JSON(http.StatusOK, result)
}

// GetRecordingPreview serves the latest frame for a recording
func (h *Handler) GetRecordingPreview(c echo.Context) error {
	idParam := c.Param("id")
	var recordingID int64
	fmt.Sscanf(idParam, "%d", &recordingID)

	// Get recording to get taskID
	// Note: We need a GetRecording query - for now we'll use a workaround
	recs, err := h.Queries.ListRecordings(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var taskID int64
	found := false
	for _, rec := range recs {
		if rec.ID == recordingID {
			taskID = rec.TaskID
			found = true
			break
		}
	}

	if !found {
		return c.NoContent(http.StatusNotFound)
	}

	// Get frame from cache
	frame := h.Recorder.GetLatestFrame(taskID)
	if frame == nil {
		return c.NoContent(http.StatusNotFound)
	}

	// Serve as JPEG with no-cache headers
	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	return c.Blob(http.StatusOK, "image/jpeg", frame)
}
