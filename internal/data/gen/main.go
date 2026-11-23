// Command gen rebuilds the embedded dataset from the default feed:
//
//	go generate ./internal/data
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/0xbadshah/zephy/internal/data"
	"github.com/0xbadshah/zephy/internal/data/feed"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ds, _, err := feed.Default.Fetch(ctx, "")
	if err == nil && ds == nil {
		err = fmt.Errorf("feed returned no data")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	b, err := data.Encode(ds)
	if err == nil {
		err = os.WriteFile("dataset.json", b, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	fmt.Printf("dataset.json: %d products from %s, generated %s\n",
		len(ds.Products), ds.Source, ds.Generated.Format(time.DateOnly))
}
