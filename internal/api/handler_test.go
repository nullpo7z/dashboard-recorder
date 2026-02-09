package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/nullpo7z/dashboard-recorder/internal/config"
	"github.com/stretchr/testify/assert"
)

// Mock dependencies would be needed for full integration tests,
// but here we just want to test validation logic if possible.
// However, the handler methods explicitly call h.Queries.CreateTask.
// To test validation *before* DB call, we might need to mock DB or structure code differently.
//
// But wait, the validation happens BEFORE the DB call.
// If I pass a nil DB/Queries, it should panic ONLY IF validation passes and it proceeds to DB.
// If validation fails, it returns 400 before touching DB.
// So I can test "Invalid Request" scenarios safely without mocks.

func TestCreateTask_Validation(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"name": "Test",
		"target_url": "http://example.com",
		"time_overlay_config": "invalid-position"
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := &Handler{
		Config: &config.Config{MaxFpsLimit: 60},
	}

	// This should fail validation and return 400
	if assert.NoError(t, h.CreateTask(c)) {
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid time_overlay_config")
	}
}

func TestCreateTask_Validation_Success(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"name": "Test",
		"target_url": "http://example.com",
		"time_overlay_config": "top-left"
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := &Handler{
		Config: &config.Config{MaxFpsLimit: 60},
		// No DB mock, so it will panic when trying to call h.Queries.CreateTask
	}

	// We expect a panic because validation passes and it hits the nil Queries
	assert.Panics(t, func() {
		h.CreateTask(c)
	})
}
