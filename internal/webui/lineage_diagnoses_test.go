package webui

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The run page's lineage summary lists rule diagnoses but not no_match, as
// the CLI text summary does.
func TestRunPageLineageDiagnosesOmitNoMatch(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	source, err := os.ReadFile("assets/web_app_core.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "function lineageDiagnosisText(")
	end := strings.Index(text[start+1:], "\nfunction ")
	if start < 0 || end < 0 {
		t.Fatal("lineageDiagnosisText not found")
	}
	code, err := json.Marshal(text[start : start+1+end])
	if err != nil {
		t.Fatal(err)
	}
	script := `
const vm = require('vm');
const context = {};
vm.createContext(context);
vm.runInContext(` + string(code) + `, context);
const got = context.lineageDiagnosisText([{name: 'OOM', count: 2}, {name: 'no_match', count: 3}]);
if (got !== 'OOM 2') throw new Error('got ' + JSON.stringify(got));
if (context.lineageDiagnosisText([{name: 'no_match', count: 1}]) !== '') throw new Error('no_match only');
if (context.lineageDiagnosisText(undefined) !== '') throw new Error('missing list');
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("lineage diagnoses: %v\n%s", err, output)
	}
}
