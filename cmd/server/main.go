package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nullpo7z/dashboard-recorder/internal/api"
	"github.com/nullpo7z/dashboard-recorder/internal/config"
	"github.com/nullpo7z/dashboard-recorder/internal/database"
	"github.com/nullpo7z/dashboard-recorder/internal/recorder"
	"golang.org/x/crypto/acme/autocert"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	// 1. Config
	cfg := config.Load()

	// 2. Database
	db, err := sql.Open("sqlite3", cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// 3. Run migrations (golang-migrate)
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		log.Fatalf("failed to create migration driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file:///app/db/migrations",
		"sqlite3", driver,
	)
	if err != nil {
		// Fallback for local development if /app/db/migrations doesn't exist
		if _, statErr := os.Stat("db/migrations"); statErr == nil {
			m, err = migrate.NewWithDatabaseInstance(
				"file://db/migrations",
				"sqlite3", driver,
			)
		}
		if err != nil {
			log.Fatalf("failed to init migration: %v", err)
		}
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %v", err)
	}
	log.Println("Database migrations applied successfully")

	queries := database.New(db)

	// 4. Recorder Worker
	worker, err := recorder.New(cfg, queries)
	if err != nil {
		log.Fatalf("failed to init recorder: %v", err)
	}
	defer worker.Stop()

	// 6. Security & Server Setup
	e := EchoServer(queries, cfg, worker, db)
	// Global Middleware for Security Headers (HSTS, CSP, etc.)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// CSP: Strict security policy
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' ws: wss:;")
			// Security Headers
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-Frame-Options", "DENY")
			if cfg.TLSDomain != "" {
				c.Response().Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			return next(c)
		}
	})

	// Start Server
	StartServer(e, cfg)
}

func EchoServer(q *database.Queries, cfg *config.Config, w *recorder.Worker, db *sql.DB) *echo.Echo {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	h := api.New(q, cfg, w, db)
	h.RegisterRoutes(e)

	// Serve Frontend (SPA)
	e.Static("/assets", "web/dist/assets")
	e.Static("/recordings", "/app/recordings") // Expose recordings
	e.File("/favicon.ico", "web/dist/favicon.ico")
	e.GET("/*", func(c echo.Context) error {
		return c.File("web/dist/index.html")
	})

	return e
}

func StartServer(e *echo.Echo, cfg *config.Config) {
	// Validate Config (Permissions check)
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Config validation failed: %v", err)
	}

	// Context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Default Timeouts for Slowloris Mitigation
	const (
		readTimeout       = 10 * time.Second
		writeTimeout      = 30 * time.Second // Longer for uploads/downloads if any
		readHeaderTimeout = 5 * time.Second
		idleTimeout       = 120 * time.Second
	)

	// HTTP Server
	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           e, // Default handler
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	// HTTPS Server (Optional)
	var httpsServer *http.Server

	if cfg.TLSDomain != "" {
		// Setup AutoTLS
		autoTLSManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.TLSDomain),
			Cache:      autocert.DirCache(cfg.TLSDataDir),
			Email:      cfg.TLSEmail,
		}

		// Configure HTTP server to handle ACME challenges + Redirect
		httpServer.Handler = autoTLSManager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Redirect to HTTPS
			target := "https://" + r.Host + r.URL.String()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}))

		// Configure HTTPS Server
		tlsConfig := autoTLSManager.TLSConfig()
		tlsConfig.MinVersion = tls.VersionTLS12

		httpsServer = &http.Server{
			Addr:              ":" + cfg.HTTPSPort,
			Handler:           e,
			TLSConfig:         tlsConfig,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			IdleTimeout:       idleTimeout,
		}

		// Start HTTPS
		go func() {
			log.Printf("Starting HTTPS server on %s", cfg.HTTPSPort)
			if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				e.Logger.Fatal("shutting down https server", err)
			}
		}()
	} else if cfg.Port != "8080" {
		// Fallback for legacy PORT env var if set and separate from HTTP_PORT
		// If user set PORT=8090, we respect it.
		// If HTTP_PORT is default 8080, and PORT is something else, we use PORT.
		if cfg.Port != "8080" && cfg.HTTPPort == "8080" {
			httpServer.Addr = ":" + cfg.Port
		}
	}

	// Start HTTP
	go func() {
		log.Printf("Starting HTTP server on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down http server", err)
		}
	}()

	// Wait for interrupt signal using the context
	<-ctx.Done()
	log.Println("Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		e.Logger.Fatal(err)
	}
	if httpsServer != nil {
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			e.Logger.Fatal(err)
		}
	}
}
