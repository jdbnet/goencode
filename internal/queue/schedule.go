package queue

import (
	"fmt"
	"strings"
	"time"
)

const (
	configQueuePaused = "queue_paused"
	configWindowStart = "encode_window_start"
	configWindowEnd   = "encode_window_end"
)

type ScheduleState struct {
	Paused      bool   `json:"paused"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	InWindow    bool   `json:"in_window"`
	Allowed     bool   `json:"allowed"`
	Timezone    string `json:"timezone"`
	Reason      string `json:"reason"`
}

func parseClock(s string) (hour, min int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	var h, m, sec int
	n, err := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec)
	if err != nil || n < 2 {
		n, err = fmt.Sscanf(s, "%d:%d", &h, &m)
		if err != nil || n != 2 {
			return 0, 0, false
		}
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

func normalizeClock(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	h, m, ok := parseClock(s)
	if !ok {
		return "", fmt.Errorf("invalid time %q, use HH:MM", s)
	}
	return fmt.Sprintf("%02d:%02d", h, m), nil
}

func inEncodeWindow(now time.Time, start, end string) bool {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" && end == "" {
		return true
	}
	sh, sm, okStart := parseClock(start)
	eh, em, okEnd := parseClock(end)
	if !okStart || !okEnd {
		return true
	}
	startM := sh*60 + sm
	endM := eh*60 + em
	nowM := now.Hour()*60 + now.Minute()
	if startM == endM {
		return true
	}
	if startM < endM {
		return nowM >= startM && nowM < endM
	}
	return nowM >= startM || nowM < endM
}
