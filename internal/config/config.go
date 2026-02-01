package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           string
	TZ             string
	JWTSecret      string
	DatabasePath   string
	PlaywrightPath string
	MaxFpsLimit    int
}

func Load() *Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// CRITICAL SECURITY REQUIREMENT: Fail fast if no secret
		panic("CRITICAL ERROR: JWT_SECRET environment variable is not set. Refusing to start.")
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		TZ:             getEnv("TZ", "UTC"),
		JWTSecret:      jwtSecret,
		DatabasePath:   getEnv("DATABASE_PATH", "./data/app.db"),
		PlaywrightPath: getEnv("PLAYWRIGHT_PATH", ""),
		MaxFpsLimit:    getEnvInt("APP_MAX_FPS_LIMIT", 60),
	}
}

func getEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}
