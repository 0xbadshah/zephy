package feed

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xbadshah/zephy/internal/data"
)

// The cache is two files: the dataset and the ETag it was served with.
const (
	cacheFile = "lifecycle.json"
	etagFile  = "lifecycle.etag"
)

// Cached returns the dataset cached in dir, or nil when there is none
// that decodes.
func Cached(dir string) *data.Dataset {
	b, err := os.ReadFile(filepath.Join(dir, cacheFile))
	if err != nil {
		return nil
	}
	ds, err := data.Decode(b)
	if err != nil {
		return nil
	}
	return ds
}

// Update asks f for data newer than the cache in dir and stores it. It
// returns the cached dataset (nil if there is none), whether it changed,
// and any error. On error the cache is left as it was.
func Update(ctx context.Context, dir string, f Feed) (*data.Dataset, bool, error) {
	cached := Cached(dir)
	etag := ""
	if cached != nil {
		b, _ := os.ReadFile(filepath.Join(dir, etagFile))
		etag = strings.TrimSpace(string(b))
	}
	ds, newETag, err := f.Fetch(ctx, etag)
	if err != nil || ds == nil {
		return cached, false, err
	}
	b, err := data.Encode(ds)
	if err != nil {
		return cached, false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return cached, false, err
	}
	// dataset first: a crash before the ETag is written only costs one
	// extra download next run
	if err := writeFileAtomic(filepath.Join(dir, cacheFile), b); err != nil {
		return cached, false, err
	}
	if err := writeFileAtomic(filepath.Join(dir, etagFile), []byte(newETag+"\n")); err != nil {
		return ds, true, err
	}
	return ds, true, nil
}

// writeFileAtomic replaces path by rename, so a reader never sees a
// half-written file.
func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
