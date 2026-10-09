package interfaces

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// rotariInShell runs rotari as a child of a separate shell, so its parent is
// not the test process: like a run started from another terminal or script.
func rotariInShell(t *testing.T, e *support.Env, args ...string) string {
	t.Helper()
	cmd := e.Command(args...)
	// The trailing commands keep the shell from replacing itself with rotari.
	shell := exec.Command("sh", append([]string{"-c", `"$0" "$@"; status=$?; exit $status`, cmd.Path}, cmd.Args[1:]...)...)
	shell.Env, shell.Dir = cmd.Env, cmd.Dir
	output, err := shell.CombinedOutput()
	if err != nil {
		t.Fatalf("rotari %v in a shell: %v\n%s", args, err, output)
	}
	return string(output)
}

// TestWaitWithoutSelectorWaitsForRunsThisProcessStarted checks that wait
// without a selector, like a shell's wait, follows the active runs started
// by its own parent process, whether or not another client follows them, and
// leaves runs started elsewhere to --all or an explicit selector.
func TestWaitWithoutSelectorWaitsForRunsThisProcessStarted(t *testing.T) {
	covers(t, "RES-16", "CLI-19")
	e := support.NewEnv(t)
	mine := e.StartRun("mine", 1, true)
	rotariInShell(t, e, "add", "-p", "elsewhere", "--", "sleep", "300")
	rotariInShell(t, e, "run", "-p", "elsewhere", "--async", "--quiet")
	t.Cleanup(func() { _ = e.Rotari("cancel", "-p", "elsewhere", "--wait") })

	// Another client following mine does not hide it from this wait.
	follower := e.Command("wait", "-p", "mine", "--timeout", "30s")
	if err := follower.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = follower.Process.Kill(); _ = follower.Wait() })

	implicit := e.Rotari("wait", "--timeout", "1s")
	out := implicit.Stdout + implicit.Stderr
	if implicit.Code == 0 || !strings.Contains(implicit.Stderr, "timed out waiting for run "+mine.RunID) || strings.Contains(out, "elsewhere") {
		t.Fatalf("wait without a selector should follow only this process's run %s: %s", mine.RunID, implicit)
	}

	all := e.Rotari("wait", "--all", "--timeout", "1s")
	if all.Code == 0 || !strings.Contains(all.Stdout+all.Stderr, "elsewhere") || !strings.Contains(all.Stderr, mine.RunID) {
		t.Fatalf("wait --all should follow every active run: %s", all)
	}
	for _, args := range [][]string{{"wait", "--all", "mine"}, {"wait", "--all", "-p", "mine"}, {"wait", "--all", "--run-id", mine.RunID}} {
		if r := e.Rotari(args...); r.Code == 0 || !strings.Contains(r.Stderr, "--all") {
			t.Errorf("%v = %s; want --all rejected with a selector", args, r)
		}
	}

	// A process that started no run is told where the active runs are.
	other := rotariInShell(t, e, "wait", "--timeout", "1s")
	if !strings.Contains(other, "no active run was started from this shell") || !strings.Contains(other, "rotari wait --all") {
		t.Fatalf("wait from another process = %q; want a pointer to --all", other)
	}
}
