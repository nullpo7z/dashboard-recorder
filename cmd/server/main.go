package main

import (
	"database/sql"
	"log"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nullpo7z/dashboard-recorder/internal/api"
	"github.com/nullpo7z/dashboard-recorder/internal/config"
	"github.com/nullpo7z/dashboard-recorder/internal/database"
	"github.com/nullpo7z/dashboard-recorder/internal/recorder"
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

	// 3. Run migrations
	if _, err := db.Exec(database.Schema); err != nil {
		log.Fatalf("failed to apply schema: %v", err)
	}

	// 3.1 Apply manual migrations for new columns (idempotent-ish check)
	// We ignore errors here usually assuming they might mean column exists,
	// but better to check or just run and allow fail if exists?
	// Sqlite doesn't support IF NOT EXISTS in ADD COLUMN easily.
	// Hacky migration:
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN filename_template TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN custom_css TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE tasks ADD COLUMN fps INTEGER NOT NULL DEFAULT 5")

	// Manual migration for CRF
	// We check for "duplicate column" error specifically to be safe
	if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN crf INTEGER NOT NULL DEFAULT 23"); err != nil {
		// SQLite error for duplicate column usually contains "duplicate column name"
		if !strings.Contains(err.Error(), "duplicate column") && !strings.Contains(err.Error(), "no such table") {
			// Log warning but don't fail, as it might just exist
			log.Printf("Migration warning (crf): %v", err)
		}
	}

	queries := database.New(db)

	// 4. Recorder Worker
	worker, err := recorder.New(cfg, queries)
	if err != nil {
		log.Fatalf("failed to init recorder: %v", err)
	}
	defer worker.Stop()

	// 5. Echo Server
	e := EchoServer(queries, cfg, worker, db)

	// Start
	e.Logger.Fatal(e.Start(":" + cfg.Port))
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
