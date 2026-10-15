package conformance

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// traceLine matches a traced call on one path: a file or directory open, or
// a stat. A directory listing is an open with O_DIRECTORY.
var traceLine = regexp.MustCompile(`^(?:\d+\s+)?(openat|newfstatat|statx|lstat|stat)\((?:AT_FDCWD|\d+), "([^"]+)"(.*)$`)

// TestReadCountsStayBounded runs read commands under strace. On a shared
// file system every open, stat, and directory listing is a round trip, so it
// fails when a command touches one state file more often than its bound,
// counted per path and reported per kind of file. Bounds are limits, not
// exact counts; lower them as reads are removed. See
// development/2026-10-15-nfs-read-performance/plan.md.
func TestReadCountsStayBounded(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("strace is Linux-only")
	}
	strace, err := exec.LookPath("strace")
	if err != nil {
		t.Skip("strace is not installed")
	}
	e := newEnv(t)
	e.mustRotari("add", "-p", "reads", "--array", "1-3", "--", "true")
	e.mustRotari("add", "-p", "reads", "--", "true")
	e.mustRotari("add", "-p", "reads", "--", "sh", "-c", "exit 3")
	if r := e.rotari("run", "-p", "reads", "--quiet"); r.code == 0 {
		t.Fatalf("the first run should fail: %s", r)
	}
	if r := e.rotari("retry", "-p", "reads", "--quiet"); r.code == 0 {
		t.Fatalf("the retry should fail again: %s", r)
	}
	runs, err := os.ReadDir(filepath.Join(e.base, "projects", "reads", "runs"))
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs = %v, %v, want an executed run and a retry", runs, err)
	}
	var latest struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", "reads", "--json").stdout), &latest); err != nil {
		t.Fatal(err)
	}
	// Run IDs made in the same second sort by their random suffix.
	executed := runs[0].Name()
	if executed == latest.RunID {
		executed = runs[1].Name()
	}
	jobDirs, err := os.ReadDir(filepath.Join(e.base, "projects", "reads", "runs", executed))
	if err != nil {
		t.Fatal(err)
	}
	oneJob := ""
	for _, entry := range jobDirs {
		if info, err := os.Stat(filepath.Join(e.base, "projects", "reads", "runs", executed, entry.Name(), "attempts")); err == nil && info.IsDir() {
			oneJob = entry.Name()
		}
	}
	if oneJob == "" {
		t.Fatalf("run %s has no job with an attempt", executed)
	}

	for _, test := range []struct {
		name   string
		args   []string
		bounds map[string]int
	}{
		{name: "show executed run", args: []string{"show", "-p", "reads", "-r", executed}, bounds: readBounds(map[string]int{
			"open project/meta.json": 5, "open project/running.lock": 5, "open run/commands.json": 3, "open run/summary.json": 5,
		})},
		{name: "show latest run", args: []string{"show", "-p", "reads"}, bounds: readBounds(map[string]int{
			"list runs": 3, "open project/meta.json": 8, "open project/queue.json": 3, "open project/running.lock": 7,
			"open run/commands.json": 3, "open run/summary.json": 6, "stat run": 9,
		})},
		{name: "jobs", args: []string{"jobs"}, bounds: readBounds(map[string]int{"stat run": 4})},
		{name: "jobs running only", args: []string{"jobs", "--since", "0"}, bounds: readBounds(nil)},
		{name: "runs", args: []string{"runs"}, bounds: readBounds(nil)},
		{name: "info", args: []string{"info", "-p", "reads"}, bounds: readBounds(nil)},
		{name: "lineage", args: []string{"lineage", "-p", "reads"}, bounds: readBounds(map[string]int{"stat run": 4})},
		{name: "show one job", args: []string{"show", "-p", "reads", "-r", executed, "-j", oneJob}, bounds: readBounds(map[string]int{
			"files attempts": 1, "files attempt/command.json": 1,
			"open attempt/command.json": 3, "stat attempt": 3, "open project/meta.json": 3, "open project/running.lock": 4, "open run/summary.json": 4,
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := traceStateReads(t, e.in(t), strace, test.args...)
			for _, kind := range sortedKeys(counts) {
				bound, ok := test.bounds[kind]
				if !ok && strings.HasPrefix(kind, "files ") {
					continue
				}
				if !ok {
					bound = test.bounds["*"]
				}
				t.Logf("%-40s %d (bound %d)", kind, counts[kind], bound)
				if counts[kind] > bound {
					t.Errorf("%s: %d per file, bound %d", kind, counts[kind], bound)
				}
			}
		})
	}
}

// readBounds is the bounds of one command: specific, over a default of two
// touches per file for every other kind.
func readBounds(specific map[string]int) map[string]int {
	bounds := map[string]int{"*": 2}
	for kind, bound := range specific {
		bounds[kind] = bound
	}
	return bounds
}

// traceStateReads runs rotari under strace and returns, for each kind of
// state file and each of list, open, and stat, the most times one file of
// that kind was touched; and, as "files KIND", how many files of that kind
// were touched at all.
func traceStateReads(t *testing.T, e *env, strace string, args ...string) map[string]int {
	t.Helper()
	output := filepath.Join(t.TempDir(), "trace")
	cmd := e.command(args...)
	cmd.Args = append([]string{strace, "-f", "-qq", "-e", "trace=openat,newfstatat,statx,lstat,stat", "-o", output, cmd.Path}, args...)
	cmd.Path = strace
	if out, err := cmd.CombinedOutput(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("strace rotari %q: %v\n%s", args, err, out)
		}
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	perPath := make(map[[2]string]int)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		match := traceLine.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		relative, err := filepath.Rel(e.base, match[2])
		if err != nil || strings.HasPrefix(relative, "..") {
			continue
		}
		verb := "stat"
		if match[1] == "openat" {
			verb = "open"
			if strings.Contains(match[3], "O_DIRECTORY") {
				verb = "list"
			}
		}
		perPath[[2]string{verb, relative}]++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	files := make(map[string]map[string]bool)
	for key, count := range perPath {
		kind := stateFileKind(key[1])
		counts[key[0]+" "+kind] = max(counts[key[0]+" "+kind], count)
		if files[kind] == nil {
			files[kind] = make(map[string]bool)
		}
		files[kind][key[1]] = true
	}
	for kind, paths := range files {
		counts["files "+kind] = len(paths)
	}
	return counts
}

// stateFileKind names a path relative to the base directory by its place in
// the state layout, with project, run, job, and attempt IDs left out.
func stateFileKind(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 2 || parts[0] != "projects" {
		return relative
	}
	parts = parts[2:]
	if len(parts) == 0 {
		return "project"
	}
	if parts[0] != "runs" {
		return "project/" + strings.Join(parts, "/")
	}
	switch {
	case len(parts) == 1:
		return "runs"
	case len(parts) == 2:
		return "run"
	case len(parts) == 3 && strings.Contains(parts[2], "."):
		return "run/" + parts[2]
	case len(parts) == 3:
		return "job"
	case len(parts) == 4 && parts[3] == "attempts":
		return "attempts"
	case len(parts) == 4:
		return "job/" + parts[3]
	case len(parts) == 5:
		return "attempt"
	default:
		return "attempt/" + strings.Join(parts[5:], "/")
	}
}

func sortedKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
