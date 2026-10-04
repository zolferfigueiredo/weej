package lang

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

//go:embed catalogs/*.json
var catalogFS embed.FS

type Language struct {
	Code string
	Name string
}

// Windows can't render flag emoji, so unlike TheeJ there are none.
var Languages = []Language{
	{Code: "de", Name: "Deutsch"},
	{Code: "en", Name: "English"},
	{Code: "es", Name: "Español"},
	{Code: "fr", Name: "Français"},
	{Code: "it", Name: "Italiano"},
	{Code: "pl", Name: "Polski"},
	{Code: "pt", Name: "Português"},
	{Code: "ru", Name: "Русский"},
	{Code: "uk", Name: "Українська"},
	{Code: "zh", Name: "中文"},
	{Code: "ja", Name: "日本語"},
	{Code: "ko", Name: "한국어"},
}

var catalogs map[string]map[string]string

func init() {
	catalogs = make(map[string]map[string]string, len(Languages))
	for _, l := range Languages {
		data, err := catalogFS.ReadFile("catalogs/" + l.Code + ".json")
		if err != nil {
			panic(fmt.Sprintf("lang: missing catalog for %q: %v", l.Code, err))
		}
		var table map[string]string
		if err := json.Unmarshal(data, &table); err != nil {
			panic(fmt.Sprintf("lang: invalid catalog for %q: %v", l.Code, err))
		}
		catalogs[l.Code] = table
	}
}

func Valid(code string) bool {
	_, ok := catalogs[code]
	return ok
}

func Detect(preferred []string) string {
	for _, tag := range preferred {
		if len(tag) < 2 {
			continue
		}
		code := strings.ToLower(tag[:2])
		if Valid(code) {
			return code
		}
	}
	return "en"
}

func lookup(lang, key string) (string, bool) {
	table, ok := catalogs[lang]
	if !ok {
		return "", false
	}
	s, ok := table[key]
	return s, ok
}

func applyVars(s string, vars map[string]string) string {
	if len(vars) == 0 {
		return s
	}
	for name, value := range vars {
		s = strings.ReplaceAll(s, "{"+name+"}", value)
	}
	return s
}

func T(lang, key string, vars map[string]string) string {
	s, ok := lookup(lang, key)
	if !ok {
		s, ok = lookup("en", key)
	}
	if !ok {
		s = key
	}
	return applyVars(s, vars)
}

func pluralForm(lang string, n int) string {
	switch lang {
	case "zh", "ja", "ko":
		return "other"
	case "fr":
		if n < 2 {
			return "one"
		}
		return "other"
	case "pl":
		if n == 1 {
			return "one"
		}
		if isFewRange(n) {
			return "few"
		}
		return "many"
	case "ru", "uk":
		mod10, mod100 := n%10, n%100
		if mod10 == 1 && mod100 != 11 {
			return "one"
		}
		if isFewRange(n) {
			return "few"
		}
		return "many"
	default:
		if n == 1 {
			return "one"
		}
		return "other"
	}
}

func isFewRange(n int) bool {
	mod10, mod100 := n%10, n%100
	return mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)
}

func Plural(lang, key string, n int, vars map[string]string) string {
	form := pluralForm(lang, n)

	s, ok := lookup(lang, key+"."+form)
	if !ok {
		s, ok = lookup(lang, key+".other")
	}
	if !ok {
		s, ok = lookup("en", key+"."+form)
	}
	if !ok {
		s, ok = lookup("en", key+".other")
	}
	if !ok {
		s = key
	}

	all := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		all[k] = v
	}
	all["n"] = strconv.Itoa(n)
	return applyVars(s, all)
}

func Catalog(lang string) map[string]string {
	en := catalogs["en"]
	merged := make(map[string]string, len(en))
	for k, v := range en {
		merged[k] = v
	}
	for k, v := range catalogs[lang] {
		merged[k] = v
	}
	return merged
}

// zh and ja run sentences together, as in TheeJ's calibration text.
func Join(lang string, sentences ...string) string {
	if lang == "zh" || lang == "ja" {
		return strings.Join(sentences, "")
	}
	return strings.Join(sentences, " ")
}
