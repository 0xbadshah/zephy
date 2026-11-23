package catalog

import "testing"

func TestSpecVersion(t *testing.T) {
	cases := []struct {
		spec  string
		style specStyle
		want  string // "" = no pin
	}{
		{"1.2.3", styleNPM, "1.2.3"},
		{"^16.14.0", styleNPM, "16"},
		{"^0.2.3", styleNPM, "0.2"},
		{"~2.6.11", styleNPM, "2.6"},
		{"2.x", styleNPM, "2"},
		{"12.x", styleNPM, "12"},
		{">=8", styleNPM, ""},
		{">=8 <11", styleNPM, "10"},
		{"^8.0 || ^10.0", styleNPM, "10"},
		{"^8 || >=14", styleNPM, ""},
		{"*", styleNPM, ""},
		{"latest", styleNPM, ""},
		{"1.2 - 1.4", styleNPM, "1.4"},
		{"~1.2", styleComposer, "1"},
		{"~1.2.3", styleComposer, "1.2"},
		{"^7.2|^8.0", styleComposer, "8"},
		{">=7.2", styleComposer, ""},
		{"7.2.*", styleComposer, "7.2"},
		{"==2.2.1", stylePEP440, "2.2.1"},
		{"~=2.2", stylePEP440, "2"},
		{"~=2.2.0", stylePEP440, "2.2"},
		{">=2.2,<3.0", stylePEP440, "2"},
		{">=1.11,<2", stylePEP440, "1"},
		{">=2.2,<2.3", stylePEP440, "2.2"},
		{"==3.6.*", stylePEP440, "3.6"},
		{">=3.6", stylePEP440, ""},
		{"~> 5.2", styleGem, "5"},
		{"~> 5.2.0", styleGem, "5.2"},
		{"~> 5.2, >= 5.2.4", styleGem, "5"},
		{">= 0.12", styleGem, ""},
		{"= 0.12.29", styleGem, "0.12.29"},
		{"==1.1.1k", stylePEP440, "1.1.1"},
		{">=3.9,<3.11", styleNPM, "3.10"}, // poetry python bounds
		{"==2.4.0rc1", stylePEP440, "2.4.0"},
	}
	for _, c := range cases {
		got, ok := specVersion(c.spec, c.style)
		if c.want == "" {
			if ok {
				t.Errorf("specVersion(%q) = %q, want no pin", c.spec, got)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("specVersion(%q) = %q,%v want %q", c.spec, got, ok, c.want)
		}
	}
}
