package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const v1Body = `{"schema_version":"1.2.1","generated_at":"2026-11-23T00:08:38+00:00","total":2,"result":[
 {"name":"python","releases":[
  {"name":"3.15","isEol":false,"eolFrom":"2031-10-31"},
  {"name":"2.7","isEol":true,"eolFrom":null},
  {"name":"","isEol":true},
  "not-a-release"]},
 {"name":"nix","releases":[{"name":"2.36","isEol":false,"eolFrom":null}]}]}`

func TestConvertEndOfLifeDate(t *testing.T) {
	ds, err := convertEndOfLifeDate([]byte(v1Body))
	if err != nil {
		t.Fatal(err)
	}
	if ds.Source != "endoflife.date" || ds.Generated.Format("2006-01-02") != "2026-11-23" {
		t.Fatalf("source/generated: %q %v", ds.Source, ds.Generated)
	}
	py := ds.Products["python"]
	if len(py) != 2 {
		t.Fatalf("python: malformed releases must be skipped, got %+v", py)
	}
	if py[0].Cycle != "3.15" || !py[0].EOL.Set || py[0].EOL.Date.Format("2006-01-02") != "2031-10-31" {
		t.Errorf("python 3.15: %+v", py[0])
	}
	if !py[1].EOL.Ended || py[1].EOL.Set {
		t.Errorf("python 2.7 is ended with no date: %+v", py[1])
	}
	if nix := ds.Products["nix"]; len(nix) != 1 || nix[0].EOL.Ended || nix[0].EOL.Set {
		t.Errorf("nix 2.36 has no end date yet: %+v", nix)
	}
}

// One conditional request per run: the first downloads and caches, the
// next sends the ETag and keeps the cache, and a failure leaves the cache
// as it was.
func TestUpdate(t *testing.T) {
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(v1Body))
	}))
	defer srv.Close()
	f := EndOfLifeDate{URL: srv.URL}
	dir := filepath.Join(t.TempDir(), "zephy")
	ctx := context.Background()

	ds, changed, err := Update(ctx, dir, f)
	if err != nil || !changed || ds == nil || len(ds.Products) != 2 {
		t.Fatalf("first update: changed=%v err=%v ds=%v", changed, err, ds)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, etagFile)); string(b) != "\"v1\"\n" {
		t.Fatalf("etag file = %q", b)
	}

	ds, changed, err = Update(ctx, dir, f)
	if err != nil || changed || ds == nil {
		t.Fatalf("unchanged update: changed=%v err=%v ds=%v", changed, err, ds)
	}

	fail = true
	ds, changed, err = Update(ctx, dir, f)
	if err == nil || changed || ds == nil {
		t.Fatalf("failed update must return the cache and the error: changed=%v err=%v ds=%v", changed, err, ds)
	}
	if Cached(dir) == nil {
		t.Fatal("a failed update damaged the cache")
	}
}

// Without a usable cache no ETag is sent, or a 304 would leave the run
// with nothing.
func TestUpdateSendsNoETagWithoutCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte(v1Body))
	}))
	defer srv.Close()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, etagFile), []byte(`"stale"`), 0o644)
	if ds, _, err := Update(context.Background(), dir, EndOfLifeDate{URL: srv.URL}); err != nil || ds == nil {
		t.Fatalf("ds=%v err=%v", ds, err)
	}
}

// The cache is replaced by rename: the final file has the new content
// and normal permissions, and no temp file is left beside it.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, cacheFile)
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "new" {
		t.Fatalf("content = %q, %v", b, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*.tmp")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}
