package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const day = 24 * time.Hour

// window is a --lookahead or --stale-after value. It takes whole days, weeks or years
// (90d, 6w, 1y; a year is 365 days) as well as any Go duration (720h).
type window time.Duration

func (w *window) Set(s string) error {
	d, err := parseWindow(s)
	if err != nil {
		return err
	}
	*w = window(d)
	return nil
}

func (w *window) String() string {
	d := time.Duration(*w)
	switch {
	case d > 0 && d%(365*day) == 0:
		return fmt.Sprintf("%dy", d/(365*day))
	case d > 0 && d%day == 0:
		return fmt.Sprintf("%dd", d/day)
	default:
		return d.String()
	}
}

func parseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	units := map[byte]time.Duration{'d': day, 'w': 7 * day, 'y': 365 * day}
	if len(s) > 1 {
		if unit, ok := units[s[len(s)-1]]; ok {
			n, err := strconv.Atoi(s[:len(s)-1])
			if err != nil || n < 0 {
				return 0, fmt.Errorf("invalid window %q: want a whole number of days (90d), weeks (6w) or years (1y), or a Go duration (720h)", s)
			}
			return time.Duration(n) * unit, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid window %q: want a whole number of days (90d), weeks (6w) or years (1y), or a Go duration (720h)", s)
	}
	return d, nil
}
