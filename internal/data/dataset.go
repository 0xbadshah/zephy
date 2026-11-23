// Package data holds zephy's lifecycle dataset: every product's release
// cycles and when each reaches end-of-life. A copy is embedded at build
// time, so zephy works offline from the first run; Use swaps in a newer
// copy at run time.
package data

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"
)

//go:generate go run ./gen

// Schema is the version of the dataset format. A file with another
// version is rejected, and zephy falls back to the embedded copy.
const Schema = 1

// Dataset is zephy's own lifecycle data format. The embedded copy, the
// cache and every feed (internal/data/feed) produce this one shape.
type Dataset struct {
	Schema    int                `json:"schema"`
	Source    string             `json:"source"`    // where the data came from
	Generated time.Time          `json:"generated"` // when the source produced it
	Products  map[string][]Cycle `json:"products"`  // product slug -> cycles, newest first
}

// Cycle is one release line of a product.
type Cycle struct {
	Cycle string  `json:"cycle"`
	EOL   EOLDate `json:"eol"`
}

// EOLDate is an end-of-life date, "true" (ended, no published date) or
// "false" (no end date yet).
type EOLDate struct {
	Date  time.Time
	Set   bool // a real date was present
	Ended bool // past end-of-life, no date given
}

func (e *EOLDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return nil // unparseable date: not set, never a failed cycle
		}
		e.Date, e.Set = t, true
		return nil
	}
	var f bool
	if err := json.Unmarshal(b, &f); err == nil {
		e.Ended = f
		return nil
	}
	return fmt.Errorf("unrecognized eol value: %s", string(b))
}

func (e EOLDate) MarshalJSON() ([]byte, error) {
	if e.Set {
		return json.Marshal(e.Date.Format(time.DateOnly))
	}
	return json.Marshal(e.Ended)
}

// Decode reads a dataset. One malformed cycle is skipped, never the
// product around it; a product left with no cycles is dropped.
func Decode(b []byte) (*Dataset, error) {
	var raw struct {
		Schema    int                          `json:"schema"`
		Source    string                       `json:"source"`
		Generated time.Time                    `json:"generated"`
		Products  map[string][]json.RawMessage `json:"products"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw.Schema != Schema {
		return nil, fmt.Errorf("dataset schema %d, want %d", raw.Schema, Schema)
	}
	ds := &Dataset{Schema: raw.Schema, Source: raw.Source, Generated: raw.Generated,
		Products: make(map[string][]Cycle, len(raw.Products))}
	for slug, items := range raw.Products {
		var cycles []Cycle
		for _, item := range items {
			var c Cycle
			if json.Unmarshal(item, &c) == nil && c.Cycle != "" {
				cycles = append(cycles, c)
			}
		}
		if len(cycles) > 0 {
			ds.Products[slug] = cycles
		}
	}
	if len(ds.Products) == 0 {
		return nil, fmt.Errorf("dataset has no products")
	}
	return ds, nil
}

// Encode writes a dataset with products sorted and one cycle per line,
// so the same data always encodes to the same bytes and diffs stay small.
func Encode(ds *Dataset) ([]byte, error) {
	var b bytes.Buffer
	gen, err := json.Marshal(ds.Generated)
	if err != nil {
		return nil, err
	}
	src, _ := json.Marshal(ds.Source)
	fmt.Fprintf(&b, "{\"schema\":%d,\"source\":%s,\"generated\":%s,\"products\":{", Schema, src, gen)
	slugs := make([]string, 0, len(ds.Products))
	for slug := range ds.Products {
		slugs = append(slugs, slug)
	}
	slices.Sort(slugs)
	for i, slug := range slugs {
		if i > 0 {
			b.WriteByte(',')
		}
		name, _ := json.Marshal(slug)
		fmt.Fprintf(&b, "\n%s:[", name)
		for j, c := range ds.Products[slug] {
			line, err := json.Marshal(c)
			if err != nil {
				return nil, err
			}
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("\n")
			b.Write(line)
		}
		b.WriteString("]")
	}
	b.WriteString("\n}}\n")
	return b.Bytes(), nil
}

//go:embed dataset.json
var embedded []byte

var (
	embeddedOnce sync.Once
	embeddedDS   *Dataset
	active       *Dataset
)

// Embedded returns the dataset built into the binary.
func Embedded() *Dataset {
	embeddedOnce.Do(func() {
		ds, err := Decode(embedded)
		if err != nil {
			panic("embedded dataset: " + err.Error()) // dataset_test.go guards every build
		}
		embeddedDS = ds
	})
	return embeddedDS
}

// Use makes ds the dataset Load reads. nil restores the embedded copy.
func Use(ds *Dataset) { active = ds }

// Current is the dataset Load reads.
func Current() *Dataset {
	if active != nil {
		return active
	}
	return Embedded()
}

// Load returns the cycles for a product slug; nil means the product is
// not tracked.
func Load(slug string) []Cycle {
	return Current().Products[slug]
}

// Tracked reports whether a slug exists in the dataset, or names a family
// whose members all do.
func Tracked(slug string) bool {
	for _, m := range Members(slug) {
		if Load(m) == nil {
			return false
		}
	}
	return true
}

// families are products no single vendor lifecycle covers: a Java
// language level is judged against the first distribution that ships that
// release line (Temurin's LTS lines, Oracle's OpenJDK builds for the
// short-lived ones, Oracle JDK for the pre-8 releases).
var families = map[string][]string{
	"java": {"eclipse-temurin", "openjdk-builds-from-oracle", "oracle-jdk"},
}

// Members lists the dataset products a slug stands for: itself, or a
// family's members in preference order.
func Members(slug string) []string {
	if m, ok := families[slug]; ok {
		return m
	}
	return []string{slug}
}
