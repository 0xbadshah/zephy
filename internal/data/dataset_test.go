package data

import (
	"bytes"
	"testing"
)

// The embedded copy must decode and hold real data: a broken
// dataset.json would panic every run.
func TestEmbeddedDecodes(t *testing.T) {
	ds := Embedded()
	if len(ds.Products) < 400 {
		t.Fatalf("embedded dataset has %d products, want the full set", len(ds.Products))
	}
	if ds.Generated.IsZero() || ds.Source == "" {
		t.Fatalf("embedded dataset lacks generated/source: %q %v", ds.Source, ds.Generated)
	}
	for _, c := range Load("nodejs") {
		if c.Cycle == "20" {
			if !c.EOL.Set {
				t.Fatalf("nodejs 20: eol not parsed, got %+v", c.EOL)
			}
			return
		}
	}
	t.Fatal("nodejs cycle 20 missing from the embedded dataset")
}

// dataset.json is exactly what Encode writes, so go generate produces no
// diff unless the data changed.
func TestEncodeRoundTrip(t *testing.T) {
	b, err := Encode(Embedded())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, embedded) {
		t.Fatal("re-encoding the embedded dataset changed it; run go generate ./internal/data")
	}
}

func TestDecodeRejectsOtherSchema(t *testing.T) {
	if _, err := Decode([]byte(`{"schema":99,"products":{"x":[{"cycle":"1","eol":false}]}}`)); err == nil {
		t.Fatal("a dataset with another schema version must be rejected")
	}
}

// One malformed cycle is skipped, never the product around it; an
// unparseable date decodes as not set.
func TestDecodeIsolatesMalformedCycles(t *testing.T) {
	ds, err := Decode([]byte(`{"schema":1,"products":{"p":[
		{"cycle":"3","eol":{"weird":1}},
		{"cycle":"2","eol":"not-a-date"},
		{"cycle":"1","eol":true}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	cs := ds.Products["p"]
	if len(cs) != 2 || cs[0].Cycle != "2" || cs[0].EOL.Set || !cs[1].EOL.Ended {
		t.Fatalf("got %+v", cs)
	}
}

func TestEOLDateEncoding(t *testing.T) {
	for _, in := range []string{`"2026-04-30"`, `true`, `false`} {
		var e EOLDate
		if err := e.UnmarshalJSON([]byte(in)); err != nil {
			t.Fatal(err)
		}
		out, _ := e.MarshalJSON()
		if string(out) != in {
			t.Errorf("%s round-tripped to %s", in, out)
		}
	}
}

func TestUntrackedSlugIsNil(t *testing.T) {
	if Load("this-product-does-not-exist") != nil {
		t.Fatal("untracked product must load as nil")
	}
}
