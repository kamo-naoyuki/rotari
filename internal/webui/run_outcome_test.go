package webui

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Browser run notifications decide success as webhooks do: by a zero exit
// code. A cancelled or failed run, or one without an exit code, is a failure.
func TestBrowserRunNotificationsUseTheExitCode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	source, err := os.ReadFile("assets/web_app_notifications.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "function runSucceeded(")
	end := strings.Index(text, "function collectRunStatuses(")
	if start < 0 || end <= start {
		t.Fatal("runSucceeded not found before collectRunStatuses")
	}
	code, err := json.Marshal(text[start:end])
	if err != nil {
		t.Fatal(err)
	}
	script := `
const vm = require('vm');
const context = {};
vm.createContext(context);
vm.runInContext(` + string(code) + `, context);
const cases = [
  [{status: 'finished', exit_code: 0}, true],
  [{status: 'failed', exit_code: 1}, false],
  [{status: 'cancelled', exit_code: 1}, false],
  [{status: 'unreadable'}, false],
  [undefined, false],
];
for (const [run, want] of cases) {
  if (context.runSucceeded(run) !== want) throw new Error(JSON.stringify(run) + ' -> ' + !want);
}
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("run outcome: %v\n%s", err, output)
	}
}
