package bot

import (
	"testing"
	"time"
)

func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"15m", 15 * time.Minute},
		{"3600s", 3600 * time.Second},
		{"12h", 12 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"60 s", 60 * time.Second},
		{"10 m", 10 * time.Minute},
	}

	for _, tc := range tests {
		got, err := ParseDurationString(tc.input)
		if err != nil {
			t.Errorf("ParseDurationString(%q) error: %v", tc.input, err)
			continue
		}
		if got != tc.expected {
			t.Errorf("ParseDurationString(%q) = %v; expected %v", tc.input, got, tc.expected)
		}
	}
}
