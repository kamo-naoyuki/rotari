package interfaces

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// printedCommands returns the rotari commands printed on lines of output,
// each split into its arguments after "rotari".
func printedCommands(output string) [][]string {
	var commands [][]string
	for _, line := range strings.Split(output, "\n") {
		if _, command, ok := strings.Cut(line, "rotari "); ok && !strings.Contains(command, "...") {
			command, _, _ = strings.Cut(command, ", then ")
			commands = append(commands, shellFields(strings.TrimSpace(command)))
		}
	}
	return commands
}

// TestRefusalAndSourceHintsNameOnlyANonImplicitBaseDir checks CLI-22 for the
// commands printed when a command refuses an interrupted project and when a
// retry uses the queue instead of the latest run: they name --basedir only
// when the state directory is not the implicit one, and work as printed.
func TestRefusalAndSourceHintsNameOnlyANonImplicitBaseDir(t *testing.T) {
	covers(t, "CLI-22")
	for _, test := range []struct {
		name     string
		implicit bool
	}{{"implicit", true}, {"other", false}} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			// Commands run where the implicit state directory is e.Base, or,
			// for "other", where it is elsewhere and -b names e.Base.
			client, location := e, []string(nil)
			if !test.implicit {
				client, location = e.WithVar("ROTARI_BASEDIR", filepath.Join(e.Root, "elsewhere")), []string{"-b", e.Base}
			}
			check := func(what, output string) {
				t.Helper()
				commands := printedCommands(output)
				if len(commands) == 0 {
					t.Fatalf("%s printed no commands:\n%s", what, output)
				}
				for _, command := range commands {
					if got := strings.Contains(strings.Join(command, " "), "--basedir"); got == test.implicit {
						t.Errorf("%s: %q names --basedir = %v", what, command, got)
					}
				}
			}

			orphan := e.OrphanRun("broken", "sleep 2; exit 7")
			t.Cleanup(func() { e.JobExitStatus("broken", orphan) })
			refused := client.Rotari(append(append([]string{"delete"}, location...), "-p", "broken", "--all")...)
			if refused.Code == 0 {
				t.Fatalf("delete of an interrupted project succeeded: %s", refused)
			}
			check("delete refusal", refused.Stderr)
			for _, command := range printedCommands(refused.Stderr) {
				if command[0] == "show" {
					if r := client.Rotari(command...); r.Code != 0 {
						t.Errorf("printed %q does not work: %s", command, r)
					}
				}
			}

			args := func(command string, rest ...string) []string {
				return append(append(append([]string{command}, location...), "-p", "source"), rest...)
			}
			client.MustRotari(args("add", "--job-name", "broken", "--", "sh", "-c", "exit 3")...)
			client.Rotari(args("run", "--quiet")...)
			client.MustRotari(args("add", "--job-name", "next", "--", "true")...)
			retried := client.Rotari(args("retry")...)
			var notice string
			for _, line := range strings.Split(retried.Stdout, "\n") {
				if strings.HasPrefix(line, "Include them with: ") {
					notice = line
				}
			}
			check("retry source notice", notice)
			copyCommand := printedCommands(notice)[0]
			if r := client.Rotari(append(copyCommand[:len(copyCommand):len(copyCommand)], "--dry-run")...); r.Code != 0 {
				t.Errorf("printed %q does not work: %s", copyCommand, r)
			}
		})
	}
}
