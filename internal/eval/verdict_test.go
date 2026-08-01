package eval_test

import (
	"strings"
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/eval"
)

func TestParseVerdict(t *testing.T) {
	plain := `{"criteria":[{"name":"Correctness","score":4,"justification":"good"}],` +
		`"summary":"Nice.","next_steps":["reread"]}`
	cases := []struct {
		name, in string
		wantErr  string
	}{
		{"plain json", plain, ""},
		{"fenced json", "```json\n" + plain + "\n```", ""},
		{"prose wrapped", "Here you go:\n" + plain + "\nHope that helps!", ""},
		{"garbage", "I cannot evaluate this.", "no JSON object"},
		{"empty criteria", `{"criteria":[],"summary":"x","next_steps":[]}`, "no criteria"},
		{"score out of range", `{"criteria":[{"name":"C","score":9,"justification":"j"}],` +
			`"summary":"x","next_steps":[]}`, "out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := eval.ParseVerdict(tc.in)
			if tc.wantErr == "" {
				if err != nil || v.Criteria[0].Score != 4 || v.Summary != "Nice." {
					t.Fatalf("got %v %+v", err, v)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
