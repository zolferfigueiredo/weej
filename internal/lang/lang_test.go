package lang

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

func baseKey(k string) string {
	for _, suf := range []string{".one", ".few", ".many", ".other"} {
		if strings.HasSuffix(k, suf) {
			return strings.TrimSuffix(k, suf)
		}
	}
	return k
}

var placeholderRe = regexp.MustCompile(`\{[a-zA-Z_]+\}`)

func placeholders(s string) []string {
	seen := make(map[string]bool)
	for _, m := range placeholderRe.FindAllString(s, -1) {
		seen[m] = true
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func TestEveryLanguageHasEveryBaseKey(t *testing.T) {
	enBase := make(map[string]bool)
	for k := range catalogs["en"] {
		enBase[baseKey(k)] = true
	}

	for _, l := range Languages {
		base := make(map[string]bool)
		for k := range catalogs[l.Code] {
			base[baseKey(k)] = true
		}
		for k := range enBase {
			if !base[k] {
				t.Errorf("%s: missing base key %q", l.Code, k)
			}
		}
		for k := range base {
			if !enBase[k] {
				t.Errorf("%s: has extra base key %q not present in English", l.Code, k)
			}
		}
	}
}

func TestPlaceholdersMatchEnglish(t *testing.T) {
	en := catalogs["en"]
	for _, l := range Languages {
		if l.Code == "en" {
			continue
		}
		for k, v := range catalogs[l.Code] {
			enKey := k
			if _, ok := en[enKey]; !ok {
				enKey = baseKey(k) + ".other"
			}
			enVal, ok := en[enKey]
			if !ok {
				t.Errorf("%s: key %q has no English counterpart (tried %q)", l.Code, k, enKey)
				continue
			}
			got, want := placeholders(v), placeholders(enVal)
			if !slices.Equal(got, want) {
				t.Errorf("%s.%s: placeholders %v, want %v (English %s = %q)", l.Code, k, got, want, enKey, enVal)
			}
		}
	}
}

func TestDistinctNamesStayApart(t *testing.T) {
	for _, l := range Languages {
		vals := []string{
			T(l.Code, "short.builtin_display", nil),
			T(l.Code, "short.screen", map[string]string{"n": "1"}),
			T(l.Code, "short.warmth", nil),
			T(l.Code, "short.external", nil),
			T(l.Code, "job.master", nil),
			T(l.Code, "job.microphone", nil),
			T(l.Code, "job.system_sounds", nil),
			T(l.Code, "job.focused_app", nil),
			T(l.Code, "job.other_apps", nil),
		}
		seen := make(map[string]bool)
		for _, v := range vals {
			if seen[v] {
				t.Errorf("%s: %q is not distinct among %v", l.Code, v, vals)
			}
			seen[v] = true
		}
	}
}

func TestPluralsMatchTheeJ(t *testing.T) {
	if got, want := Plural("en", "seconds_left", 1, nil), "1 second left"; got != want {
		t.Errorf("en seconds_left 1 = %q, want %q", got, want)
	}
	if got, want := Plural("en", "seconds_left", 20, nil), "20 seconds left"; got != want {
		t.Errorf("en seconds_left 20 = %q, want %q", got, want)
	}

	ru := map[int]string{
		1:  "Найдена 1 ручка",
		3:  "Найдено 3 ручки",
		5:  "Найдено 5 ручек",
		21: "Найдена 21 ручка",
	}
	for n, want := range ru {
		if got := Plural("ru", "knobs_found", n, nil); got != want {
			t.Errorf("ru knobs_found %d = %q, want %q", n, got, want)
		}
	}

	pl := map[int]string{
		1:  "Została 1 sekunda",
		2:  "Zostały 2 sekundy",
		5:  "Zostało 5 sekund",
		22: "Zostały 22 sekundy",
	}
	for n, want := range pl {
		if got := Plural("pl", "seconds_left", n, nil); got != want {
			t.Errorf("pl seconds_left %d = %q, want %q", n, got, want)
		}
	}

	if got, want := Plural("ja", "knobs_found", 1, nil), "1 個のノブが見つかりました"; got != want {
		t.Errorf("ja knobs_found 1 = %q, want %q", got, want)
	}
}

func TestLanguagesAreUnique(t *testing.T) {
	if len(Languages) != 12 {
		t.Fatalf("len(Languages) = %d, want 12", len(Languages))
	}
	codes := make(map[string]bool)
	names := make(map[string]bool)
	for _, l := range Languages {
		if codes[l.Code] {
			t.Errorf("duplicate language code %q", l.Code)
		}
		codes[l.Code] = true
		if names[l.Name] {
			t.Errorf("duplicate language name %q", l.Name)
		}
		names[l.Name] = true
	}
}

func TestNoEmOrEnDash(t *testing.T) {
	for _, l := range Languages {
		for k, v := range catalogs[l.Code] {
			if strings.ContainsRune(v, '—') || strings.ContainsRune(v, '–') {
				t.Errorf("%s.%s contains an em or en dash: %q", l.Code, k, v)
			}
		}
	}
}

func TestNoLeftoverTerms(t *testing.T) {
	leftovers := []string{"TheeJ", "Night Shift", "macOS", "m1ddc", "⌘", "⌃"}
	for _, l := range Languages {
		for k, v := range catalogs[l.Code] {
			for _, bad := range leftovers {
				if strings.Contains(v, bad) {
					t.Errorf("%s.%s contains leftover %q: %q", l.Code, k, bad, v)
				}
			}
		}
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		preferred []string
		want      string
	}{
		{[]string{"pt-BR"}, "pt"},
		{[]string{"zh-Hans-CN"}, "zh"},
		{[]string{"xx"}, "en"},
		{nil, "en"},
		{[]string{}, "en"},
		{[]string{"xx", "fr-FR"}, "fr"},
	}
	for _, c := range cases {
		if got := Detect(c.preferred); got != c.want {
			t.Errorf("Detect(%v) = %q, want %q", c.preferred, got, c.want)
		}
	}
}

func TestValid(t *testing.T) {
	if !Valid("en") {
		t.Error("Valid(\"en\") = false, want true")
	}
	if Valid("xx") {
		t.Error("Valid(\"xx\") = true, want false")
	}
}

func TestTFallback(t *testing.T) {
	if got, want := T("en", "cancel", nil), "Cancel"; got != want {
		t.Errorf("T(en, cancel) = %q, want %q", got, want)
	}
	if got, want := T("xx", "cancel", nil), "Cancel"; got != want {
		t.Errorf("T(xx, cancel) = %q, want %q (fallback to English)", got, want)
	}
	if got, want := T("en", "no_such_key", nil), "no_such_key"; got != want {
		t.Errorf("T(en, no_such_key) = %q, want %q (fallback to key)", got, want)
	}
}

func TestCatalogMergesOverEnglish(t *testing.T) {
	full := Catalog("en")
	if len(full) != len(catalogs["en"]) {
		t.Fatalf("Catalog(en) has %d keys, want %d", len(full), len(catalogs["en"]))
	}
	de := Catalog("de")
	if de["cancel"] != catalogs["de"]["cancel"] {
		t.Errorf("Catalog(de)[cancel] = %q, want %q", de["cancel"], catalogs["de"]["cancel"])
	}
	fallback := Catalog("xx")
	if !slices.Equal(sortedValues(fallback), sortedValues(catalogs["en"])) {
		t.Errorf("Catalog(xx) did not fall back to the full English table")
	}
}

func sortedValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func TestJoin(t *testing.T) {
	if got, want := Join("en", "A.", "B."), "A. B."; got != want {
		t.Errorf("Join(en, ...) = %q, want %q", got, want)
	}
	if got, want := Join("zh", "A", "B"), "AB"; got != want {
		t.Errorf("Join(zh, ...) = %q, want %q", got, want)
	}
	if got, want := Join("ja", "A", "B"), "AB"; got != want {
		t.Errorf("Join(ja, ...) = %q, want %q", got, want)
	}
	if got, want := Join("ko", "A", "B"), "A B"; got != want {
		t.Errorf("Join(ko, ...) = %q, want %q", got, want)
	}
}
