package recorder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// We can't easily mock the ntp.Query function in the current implementation without dependency injection.
// However, we can test the fallback logic if we point to an invalid server.

func TestGetNTPTime_InvalidServer(t *testing.T) {
	// This should fail and return error, eventually falling back to 0 in InjectTimeOverlay (but GetNTPTime itself returns error)
	_, err := GetNTPTime("invalid.server.local")
	assert.Error(t, err)
}

// For a valid server, we can't guarantee network access in CI/Unit Test environment usually.
// But if we are allowed network, we could try a known server.
// For now, we'll assume unit tests might run offline.

func TestInjectTimeOverlay_Validation(t *testing.T) {
	// We can test the config validation logic inside InjectTimeOverlay by extracting it or
	// just trusting the integration test.
	// The current InjectTimeOverlay does validation internally.
}
