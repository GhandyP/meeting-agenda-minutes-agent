package meeting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Case struct {
	Name             string           `json:"name"`
	Spec             string           `json:"spec"`
	Input            MeetingPlanInput `json:"input"`
	Output           AgendaOrMinutes  `json:"output"`
	WantProblemCodes []string         `json:"wantProblemCodes"`
	WantTopicMinutes int              `json:"wantTopicMinutes"`
	WantHasErrors    bool             `json:"wantHasErrors"`
}

// LoadCases reads every JSON fixture directly in dir, in filename order.
func LoadCases(dir string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixture directory %q: %w", dir, err)
	}

	cases := make([]Case, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read fixture %q: %w", path, err)
		}
		if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
			return nil, fmt.Errorf("decode fixture %q: expected a JSON object", path)
		}

		var testCase Case
		if err := json.Unmarshal(data, &testCase); err != nil {
			return nil, fmt.Errorf("decode fixture %q: %w", path, err)
		}
		cases = append(cases, testCase)
	}

	return cases, nil
}
