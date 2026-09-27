package interfaces

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

func TestCheckJSONMatchesText(t *testing.T) {
	covers(t, "CLI-1")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "check", "--", "true")

	text := e.MustRotari("check", "check").Stdout
	var machine struct {
		Project  string `json:"project"`
		State    string `json:"state"`
		Runnable bool   `json:"runnable"`
		Queued   *int   `json:"queued"`
		Lock     string `json:"lock"`
		RunID    string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("check", "check", "--json").Stdout), &machine); err != nil {
		t.Fatal(err)
	}
	fields := make(map[string]string)
	for _, field := range strings.Fields(text) {
		key, value, ok := strings.Cut(field, "=")
		if ok {
			fields[key] = value
		}
	}
	if machine.Project != fields["project"] || machine.State != fields["state"] ||
		(machine.Runnable && fields["runnable"] != "true") ||
		(!machine.Runnable && fields["runnable"] != "false") ||
		machine.Lock != fields["lock"] {
		t.Fatalf("check projections disagree: text=%q json=%+v", text, machine)
	}
	if machine.Queued == nil || *machine.Queued != 1 || fields["queued"] != "1" {
		t.Fatalf("queued count disagrees: text=%q json=%+v", text, machine)
	}
}
