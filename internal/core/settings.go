package core

import (
	"encoding/json"
	"slices"
	"strconv"
)

// ButtonMap is a profile's button functions by button id; a button may do several things at once.
// Saved as an object of id to list of actions. A bad entry in a saved file is dropped on its own
// instead of failing the whole profile list, and a single action, as files before lists held,
// still reads.
type ButtonMap map[int][]ButtonAction

func (m ButtonMap) MarshalJSON() ([]byte, error) {
	out := map[string][]ButtonAction{}
	for id, actions := range m {
		if len(actions) > 0 {
			out[strconv.Itoa(id)] = actions
		}
	}
	return json.Marshal(out)
}

func (m *ButtonMap) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		*m = ButtonMap{}
		return nil
	}
	out := ButtonMap{}
	for k, v := range raw {
		var list []ButtonAction
		if err := json.Unmarshal(v, &list); err != nil {
			var one ButtonAction
			if json.Unmarshal(v, &one) != nil {
				continue
			}
			list = []ButtonAction{one}
		}
		out.set(k, list)
	}
	*m = out
	return nil
}

func (m ButtonMap) set(key string, actions []ButtonAction) {
	id, err := strconv.Atoi(key)
	if err != nil || id < 0 || id > 255 {
		return
	}
	var kept []ButtonAction
	for _, a := range actions {
		if a != ActionNone && a.Valid() && !slices.Contains(kept, a) {
			kept = append(kept, a)
		}
	}
	if len(kept) > 0 {
		m[id] = kept
	}
}

func normalizeJobs(jobs [][]Job) [][]Job {
	if jobs == nil {
		jobs = [][]Job{}
	}
	for i, row := range jobs {
		if row == nil {
			jobs[i] = []Job{}
		}
	}
	return jobs
}

func take[T any](raw map[string]json.RawMessage, key string) (T, bool) {
	var zero T
	v, ok := raw[key]
	if !ok {
		return zero, false
	}
	var out T
	if err := json.Unmarshal(v, &out); err != nil {
		return zero, false
	}
	return out, true
}
