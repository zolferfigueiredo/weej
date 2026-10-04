package core

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Import struct {
	Name    string
	Columns []int
	Jobs    [][]Job
	Invert  bool
	Skipped []string
}

var monitorBrightnessPattern = regexp.MustCompile(`^monitor\s+(\d+)\s*\(brightness\)$`)

func deejJob(entry string) (Job, bool) {
	lower := strings.ToLower(strings.TrimSpace(entry))
	switch lower {
	case "master":
		return Job{Kind: JobMaster}, true
	case "mic":
		return Job{Kind: JobMicrophone}, true
	case "system":
		return Job{Kind: JobSystemSounds}, true
	case "deej.unmapped":
		return Job{Kind: JobOtherApps}, true
	case "deej.current":
		return Job{Kind: JobFocusedApp}, true
	}
	if m := monitorBrightnessPattern.FindStringSubmatch(lower); m != nil {
		n, _ := strconv.Atoi(m[1])
		return Job{Kind: JobBrightness, Screen: n - 1}, true
	}
	if strings.HasSuffix(lower, ".exe") {
		return Job{Kind: JobApp, Exe: lower}, true
	}
	return Job{}, false
}

// deej's own default direction is raw; TheeJ's (and so WeeJ's) is 1 - raw. Invert undoes that
// difference, so it is the negation of deej's invert_sliders, not a copy of it.
func ImportDeej(yamlBytes []byte) (Import, error) {
	var doc struct {
		SliderMapping map[int]interface{} `yaml:"slider_mapping"`
		InvertSliders bool                `yaml:"invert_sliders"`
	}
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return Import{}, fmt.Errorf("core: deej import: %w", err)
	}

	cols := make([]int, 0, len(doc.SliderMapping))
	for k := range doc.SliderMapping {
		cols = append(cols, k)
	}
	sort.Ints(cols)

	imp := Import{
		Name:    "deej",
		Columns: cols,
		Invert:  !doc.InvertSliders,
		Jobs:    make([][]Job, len(cols)),
	}

	for i, col := range cols {
		var entries []string
		switch v := doc.SliderMapping[col].(type) {
		case string:
			entries = []string{v}
		case []interface{}:
			for _, e := range v {
				if str, ok := e.(string); ok {
					entries = append(entries, str)
				}
			}
		}

		seenApps := map[string]bool{}
		var jobs []Job
		for _, entry := range entries {
			job, ok := deejJob(entry)
			if !ok {
				imp.Skipped = append(imp.Skipped, entry)
				continue
			}
			if job.Kind == JobApp {
				if seenApps[job.Exe] {
					continue
				}
				seenApps[job.Exe] = true
			}
			jobs = append(jobs, job)
		}
		imp.Jobs[i] = jobs
	}

	return imp, nil
}
