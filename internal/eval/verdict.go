package eval

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseVerdict extracts the JSON verdict from raw LLM output, tolerating
// markdown fences and surrounding prose.
func ParseVerdict(raw string) (Verdict, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return Verdict{}, fmt.Errorf("no JSON object in model output")
	}
	var v Verdict
	if err := json.Unmarshal([]byte(raw[start:end+1]), &v); err != nil {
		return Verdict{}, fmt.Errorf("decode verdict: %w", err)
	}
	if len(v.Criteria) == 0 {
		return Verdict{}, fmt.Errorf("verdict has no criteria")
	}
	for _, c := range v.Criteria {
		if c.Score < 1 || c.Score > 5 {
			return Verdict{}, fmt.Errorf("criterion %q score %d out of range 1-5", c.Name, c.Score)
		}
	}
	return v, nil
}
