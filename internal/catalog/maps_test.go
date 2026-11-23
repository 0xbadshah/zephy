package catalog

import (
	"testing"

	"github.com/0xbadshah/zephy/internal/data"
)

// A mapped slug the dataset doesn't know makes every pin of that image
// unverifiable.
func TestEveryMappedSlugExistsInDataset(t *testing.T) {
	loadMaps()
	for name, table := range map[string]map[string]string{
		"images": maps.Images, "distros": maps.Distros, "packages": maps.Packages, "libraries": maps.Libraries,
	} {
		if len(table) == 0 {
			t.Errorf("maps.json %s table is empty", name)
		}
		for k, slug := range table {
			if !data.Tracked(slug) {
				t.Errorf("maps.json %s[%q] = %q: not in the dataset", name, k, slug)
			}
		}
	}
}
