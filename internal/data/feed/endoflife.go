package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
)

// EndOfLifeDate reads endoflife.date's v1 API: every product with its
// releases in one response.
type EndOfLifeDate struct {
	URL string
}

func (f EndOfLifeDate) Fetch(ctx context.Context, etag string) (*data.Dataset, string, error) {
	body, newETag, err := getIfChanged(ctx, f.URL, etag)
	if err != nil || body == nil {
		return nil, newETag, err
	}
	ds, err := convertEndOfLifeDate(body)
	if err != nil {
		return nil, "", fmt.Errorf("endoflife.date: %w", err)
	}
	return ds, newETag, nil
}

// convertEndOfLifeDate maps the v1 "products/full" response to zephy's
// format. A malformed release is skipped, never its product.
func convertEndOfLifeDate(body []byte) (*data.Dataset, error) {
	var doc struct {
		GeneratedAt string `json:"generated_at"`
		Result      []struct {
			Name     string            `json:"name"`
			Releases []json.RawMessage `json:"releases"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	generated, err := time.Parse(time.RFC3339, doc.GeneratedAt)
	if err != nil {
		generated = time.Now().UTC()
	}
	ds := &data.Dataset{Schema: data.Schema, Source: "endoflife.date", Generated: generated.UTC(),
		Products: map[string][]data.Cycle{}}
	for _, p := range doc.Result {
		var cycles []data.Cycle
		for _, raw := range p.Releases {
			var r struct {
				Name    string `json:"name"`
				IsEOL   bool   `json:"isEol"`
				EOLFrom string `json:"eolFrom"`
			}
			if json.Unmarshal(raw, &r) != nil || r.Name == "" {
				continue
			}
			c := data.Cycle{Cycle: r.Name, EOL: data.EOLDate{Ended: r.IsEOL}}
			if d, err := time.Parse(time.DateOnly, r.EOLFrom); err == nil {
				c.EOL = data.EOLDate{Date: d, Set: true}
			}
			cycles = append(cycles, c)
		}
		if p.Name != "" && len(cycles) > 0 {
			ds.Products[p.Name] = cycles
		}
	}
	if len(ds.Products) == 0 {
		return nil, fmt.Errorf("no products in response")
	}
	return ds, nil
}
