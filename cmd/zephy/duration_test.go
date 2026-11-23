package main

import (
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"30d":  30 * day,
		"6w":   42 * day,
		"1y":   365 * day,
		"2y":   730 * day,
		"720h": 720 * time.Hour,
		"90m":  90 * time.Minute,
		"0d":   0,
	} {
		got, err := parseWindow(in)
		if err != nil || got != want {
			t.Errorf("parseWindow(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "1.5d", "-3d", "-1h", "1x", "y1"} {
		if _, err := parseWindow(bad); err == nil {
			t.Errorf("parseWindow(%q) accepted, want an error", bad)
		}
	}
}

func TestWindowString(t *testing.T) {
	for d, want := range map[time.Duration]string{
		365 * day:        "1y",
		42 * day:         "42d",
		720 * time.Hour:  "30d",
		90 * time.Minute: "1h30m0s",
	} {
		w := window(d)
		if got := w.String(); got != want {
			t.Errorf("window(%v).String() = %q, want %q", d, got, want)
		}
	}
}
