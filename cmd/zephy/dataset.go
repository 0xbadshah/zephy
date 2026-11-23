package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
	"github.com/0xbadshah/zephy/internal/data/feed"
)

// Lifecycle data for a run is the embedded copy or the cached download,
// whichever the source generated later. Before each scan, one conditional
// request to the feed refreshes the cache; --offline skips it.

const updateTimeout = 20 * time.Second

// cacheDir holds the downloaded dataset: os.UserCacheDir()/zephy
// (~/Library/Caches/zephy on macOS, ~/.cache/zephy on Linux).
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "zephy"), nil
}

// selectDataset makes the newer of cached and the embedded copy the
// dataset for this run, and returns it.
func selectDataset(cached *data.Dataset) *data.Dataset {
	if cached != nil && cached.Generated.After(data.Embedded().Generated) {
		data.Use(cached)
	}
	return data.Current()
}

// loadDataset picks this run's data. Unless offline, it first updates the
// cache; a failed update prints one line and the scan goes on.
func loadDataset(offline bool) {
	dir, err := cacheDir()
	if err != nil {
		return // no cache location: the embedded copy stays
	}
	if offline {
		selectDataset(feed.Cached(dir))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	cached, _, err := feed.Update(ctx, dir, feed.Default)
	ds := selectDataset(cached)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zephy: lifecycle data not updated (%v); using data from %s\n",
			err, ds.Generated.Format(time.DateOnly))
	}
}

// refresh updates the cache now and says what it holds.
func refresh() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	ds, changed, err := feed.Update(ctx, dir, feed.Default)
	if err != nil {
		return "", err
	}
	if ds == nil {
		return "", fmt.Errorf("feed returned no data")
	}
	state := "already current"
	if changed {
		state = "updated"
	}
	return fmt.Sprintf("lifecycle data %s: %d products from %s, generated %s",
		state, len(ds.Products), ds.Source, ds.Generated.Format(time.DateOnly)), nil
}
