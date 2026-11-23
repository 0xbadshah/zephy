package main

import (
	"testing"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
)

// The run uses whichever copy the source generated later: a fresh
// download beats the embedded copy, and an old cache never replaces a
// newer embedded one after an upgrade.
func TestSelectDatasetNewerWins(t *testing.T) {
	t.Cleanup(func() { data.Use(nil) })
	embedded := data.Embedded()
	newer := &data.Dataset{Generated: embedded.Generated.Add(time.Hour)}
	older := &data.Dataset{Generated: embedded.Generated.Add(-time.Hour)}

	if got := selectDataset(older); got != embedded {
		t.Error("an older cache replaced the embedded copy")
	}
	if got := selectDataset(nil); got != embedded {
		t.Error("no cache must keep the embedded copy")
	}
	if got := selectDataset(newer); got != newer {
		t.Error("a newer cache was not used")
	}
}
