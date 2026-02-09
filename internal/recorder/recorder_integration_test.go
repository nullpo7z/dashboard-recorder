package recorder

import (
	"strings"
	"testing"
)

// TestCapturePreview_SecurityIntegration verifies that CapturePreview
// correctly integrates the validation logic and blocks private IPs.
// This test does not require a running browser because the validation checks happens first.
func TestCapturePreview_SecurityIntegration(t *testing.T) {
	// We mocking the worker structs is hard, but since validation is the first step,
	// we can call CapturePreview on a Worker with nil browser,
	// expecting it to fail at validation step before reaching browser logic.

	w := &Worker{} // Nil browser is fine as validation runs first

	tests := []struct {
		name      string
		url       string
		wantError string
	}{
		{"Localhost Blocked", "http://localhost:8080", "access to private IP 127.0.0.1"},
		{"Private IP Blocked", "http://192.168.1.1", "access to private IP 192.168.1.1"},
		{"File Scheme Blocked", "file:///etc/passwd", "invalid protocol: file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := w.CapturePreview(tt.url, "")
			if err == nil {
				t.Errorf("CapturePreview(%q) expected error, got nil", tt.url)
				return
			}
			if !strings.Contains(err.Error(), tt.wantError) && !strings.Contains(err.Error(), "access to private IP") {
				t.Errorf("CapturePreview(%q) error = %v, want substring %q", tt.url, err, tt.wantError)
			}
		})
	}
}
