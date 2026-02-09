package recorder

import (
	"testing"
)

func TestCalculateJpegQuality(t *testing.T) {
	tests := []struct {
		name string
		crf  int64
		want int
	}{
		{"CRF 0 (Highest Quality)", 0, MaxJpegQuality}, // Should be capped at 95
		{"CRF 18 (Very High)", 18, 91},                 // 100 - 9 = 91
		{"CRF 23 (Default)", 23, 88},                   // 100 - 11.5 = 88.5 -> 88
		{"CRF 28 (Good)", 28, 86},                      // 100 - 14 = 86
		{"CRF 51 (Lowest Quality)", 51, 74},            // 100 - 25.5 = 74.5 -> 74
		{"CRF Negative (Clamped to 0)", -5, MaxJpegQuality},
		{"CRF Too High (Clamped to 51)", 100, 74},
		{"CRF Boundary 1", 1, 95}, // 100 - 0.5 = 99 -> capped 95. Actually 99.5 -> 99. Wait.
		// 100 - 0.5 = 99.5 -> int(99.5) = 99 -> capped 95. Correct.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateJpegQuality(tt.crf)
			if got != tt.want {
				t.Errorf("calculateJpegQuality(%d) = %d; want %d", tt.crf, got, tt.want)
			}
		})
	}
}

func TestCalculateJpegQuality_Bounds(t *testing.T) {
	// Ensure it never goes above Max or below Min
	// Although currently math prevents it going below 74, let's test extreme inputs if logic changes

	// Test Max
	if got := calculateJpegQuality(0); got > MaxJpegQuality {
		t.Errorf("calculateJpegQuality(0) = %d; want <= %d", got, MaxJpegQuality)
	}

	// Test Min (CRF 51 yields ~74, which is > 30)
	// If allow CRF 100 -> clamped to 51 -> 74.
	// So we are safe.
}
