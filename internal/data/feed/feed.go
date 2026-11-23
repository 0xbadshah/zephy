// Package feed brings fresh lifecycle data in. A Feed turns one upstream
// source into zephy's dataset format; Update keeps a cached copy current
// with one conditional request per run.
package feed

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
)

// A Feed supplies the current lifecycle data. etag names the version the
// caller already holds ("" for none); when it is still current, Fetch
// returns a nil dataset.
type Feed interface {
	Fetch(ctx context.Context, etag string) (ds *data.Dataset, newETag string, err error)
}

// Default is the feed zephy updates from, and the one go generate builds
// the embedded copy from.
var Default Feed = EndOfLifeDate{URL: "https://endoflife.date/api/v1/products/full"}

// client fails fast on a dead network and still honors proxy settings.
var client = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	t.TLSHandshakeTimeout = 5 * time.Second
	t.ResponseHeaderTimeout = 10 * time.Second
	return &http.Client{Transport: t}
}()

// maxBody caps a download; the full dataset is a few megabytes.
const maxBody = 64 << 20

// getIfChanged GETs url unless etag still names its current version. It
// returns a nil body when nothing changed.
func getIfChanged(ctx context.Context, url, etag string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "zephy")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, etag, nil
	case http.StatusOK:
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		return b, resp.Header.Get("ETag"), err
	}
	return nil, "", fmt.Errorf("%s: %s", url, resp.Status)
}
