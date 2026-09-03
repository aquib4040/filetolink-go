package bot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Matches tokens like "30d", "12h", "45m", "60s", "2w", "1mo"
	durationTokenRegex = regexp.MustCompile(`(?i)(\d+)\s*([a-z]+)`)
)

// ParseDurationString parses duration strings in d, m, s, h, w, mo format.
// Examples: "30d", "15m", "3600s", "12h", "2w", "1mo", "1d12h30m", or separated "30 d"
func ParseDurationString(input string) (time.Duration, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	matches := durationTokenRegex.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		// Try raw integer as days
		if val, err := strconv.Atoi(input); err == nil && val > 0 {
			return time.Duration(val) * 24 * time.Hour, nil
		}
		return 0, fmt.Errorf("invalid duration format (use e.g. 30d, 12h, 45m, 3600s)")
	}

	var total time.Duration
	for _, m := range matches {
		val, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, err
		}
		unit := strings.ToLower(m[2])

		switch unit {
		case "s", "sec", "second", "seconds":
			total += time.Duration(val) * time.Second
		case "m", "min", "minute", "minutes":
			total += time.Duration(val) * time.Minute
		case "h", "hr", "hour", "hours":
			total += time.Duration(val) * time.Hour
		case "d", "day", "days":
			total += time.Duration(val) * 24 * time.Hour
		case "w", "week", "weeks":
			total += time.Duration(val) * 7 * 24 * time.Hour
		case "mo", "mon", "month", "months":
			total += time.Duration(val) * 30 * 24 * time.Hour
		case "y", "yr", "year", "years":
			total += time.Duration(val) * 365 * 24 * time.Hour
		default:
			return 0, fmt.Errorf("unknown time unit: %s", unit)
		}
	}

	if total <= 0 {
		return 0, fmt.Errorf("duration must be greater than zero")
	}

	return total, nil
}

// FormatTimeLeft formats remaining duration into "X d Y h Z m W s left"
func FormatTimeLeft(expiresAt time.Time) string {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return "Expired"
	}

	days := int(remaining.Hours()) / 24
	hours := int(remaining.Hours()) % 24
	minutes := int(remaining.Minutes()) % 60
	seconds := int(remaining.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds left", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds left", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds left", minutes, seconds)
	}
	return fmt.Sprintf("%ds left", seconds)
}
