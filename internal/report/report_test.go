package report

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/0xbadshah/zephy/internal/match"
)

func TestSortFindings(t *testing.T) {
	want := []match.Finding{
		{Status: match.StatusEOL, Repo: "api", Product: "python", File: "Dockerfile:1"},
		{Status: match.StatusEOL, Repo: "api", Product: "postgresql", File: "docker-compose.yml:3"},
		{Status: match.StatusEOL, Repo: "web", Product: "nodejs", File: "package.json:3"},
		{Status: match.StatusExpiring, Repo: "api", Product: "ubuntu", File: ".github/workflows/ci.yml:3"},
		{Status: match.StatusDeprecated, Repo: "web", Product: "request", File: "package.json:9"},
		{Status: match.StatusDeprecated, Repo: "web", Product: "node-sass", File: "package.json:10"},
		{Status: match.StatusDeprecated, Repo: "web", Product: "left-pad", File: "pkg:npm/left-pad"},
		{Status: match.StatusAbandoned, Repo: "api", Product: "pycrypto", File: "requirements.txt:4"},
	}
	r := rand.New(rand.NewSource(1))
	for range 20 {
		in := slices.Clone(want)
		r.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
		got := sortFindings(in)
		for i := range want {
			if got[i].Product != want[i].Product {
				t.Fatalf("row %d: got %s, want %s (lines compare as numbers, worst status first)", i, got[i].Product, want[i].Product)
			}
		}
	}
}
