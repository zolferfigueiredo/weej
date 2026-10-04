package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Import struct {
	Name    string
	Columns []int
	Jobs    [][]Job
	Invert  bool
	// Baud is deej's baud_rate, 0 when the file has none. Its com_port is left out on purpose:
	// WeeJ finds the board by itself, and a fixed port breaks when Windows renumbers it.
	Baud    int
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
// Knobs follow the order slider_mapping lists them in, which is how people write their
// boards top to bottom; the slider indexes say which input each knob is on, not its letter.
func ImportDeej(yamlBytes []byte) (Import, error) {
	var doc struct {
		SliderMapping yaml.Node `yaml:"slider_mapping"`
		InvertSliders bool      `yaml:"invert_sliders"`
		BaudRate      int       `yaml:"baud_rate"`
	}
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return Import{}, fmt.Errorf("core: deej import: %w", err)
	}
	if doc.SliderMapping.Kind != yaml.MappingNode {
		return Import{}, fmt.Errorf("core: deej import: slider_mapping is missing")
	}

	imp := Import{Name: "deej", Invert: !doc.InvertSliders, Baud: doc.BaudRate}
	pairs := doc.SliderMapping.Content
	for i := 0; i+1 < len(pairs); i += 2 {
		col, err := strconv.Atoi(strings.TrimSpace(pairs[i].Value))
		if err != nil {
			imp.Skipped = append(imp.Skipped, pairs[i].Value)
			continue
		}
		var entries []string
		switch v := pairs[i+1]; v.Kind {
		case yaml.ScalarNode:
			entries = []string{v.Value}
		case yaml.SequenceNode:
			for _, e := range v.Content {
				if e.Kind == yaml.ScalarNode {
					entries = append(entries, e.Value)
				}
			}
		}

		seenApps := map[string]bool{}
		jobs := []Job{}
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
		imp.Columns = append(imp.Columns, col)
		imp.Jobs = append(imp.Jobs, jobs)
	}

	return imp, nil
}
