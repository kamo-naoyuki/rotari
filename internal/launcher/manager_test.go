package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func newTestManager(t *testing.T, config ManagerConfig) *Manager {
	t.Helper()
	config.Wrapper = []string{os.Args[0]}
	if config.Env == nil {
		config.Env = os.Environ()
	}
	config.Env = append(config.Env, testRole+"=wrapper")
	manager, err := NewManager(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close() })
	return manager
}

func testCommand(name, command string) Command {
	return Command{Name: name, Command: command, Targets: []TargetKind{TargetFile, TargetDirectory},
		Mode: ModeCommand, CWD: CWDTarget, StopGrace: 300 * time.Millisecond}
}

func waitEnded(t *testing.T, manager *Manager, id string) Launch {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		launch, err := manager.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if !launch.State.live() {
			return launch
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("launch %s did not end", id)
	return Launch{}
}

func readAllLog(t *testing.T, manager *Manager, id string) LogChunk {
	t.Helper()
	chunk, err := manager.ReadLog(id, 0, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return chunk
}

func TestManagerRecordsCommandOutcome(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{})
	target := t.TempDir()
	for _, test := range []struct {
		command  string
		exitCode *int
		signal   string
	}{
		{"echo out; echo err >&2; exit 3", ptr(3), ""},
		{"kill -USR1 $$", nil, syscall.SIGUSR1.String()},
	} {
		launch, err := manager.Start(StartRequest{Command: testCommand(test.command, test.command), Target: target, Kind: TargetDirectory,
			Labels: map[string]string{"job": "j1"}})
		if err != nil || launch.State != StateRunning || launch.CWD != target || launch.Labels["job"] != "j1" {
			t.Fatalf("Start() = %+v, %v", launch, err)
		}
		ended := waitEnded(t, manager, launch.ID)
		if ended.State != StateExited || ended.EndedAt == nil || ended.Signal != test.signal ||
			(test.exitCode == nil) != (ended.ExitCode == nil) || (test.exitCode != nil && *ended.ExitCode != *test.exitCode) {
			t.Fatalf("%q ended as %+v", test.command, ended)
		}
	}
	first := manager.List()[0]
	if log := readAllLog(t, manager, first.ID); string(log.Data) != "out\nerr\n" || log.Dropped {
		t.Fatalf("log = %+v", log)
	}
}

func ptr(value int) *int { return &value }

func TestManagerPassesLaunchEnvironment(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{Env: append(os.Environ(),
		"NO_PROXY=corp.example", EnvPort+"=1234", EnvArtifact+"=/stale")})
	dir := filepath.Join(t.TempDir(), "run $(touch pwned) 'x'")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "a b.txt")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	command := testCommand("env", `printf '%s|%s|%s|%s|%s|%s' "$ROTARI_LAUNCHER_ARTIFACT" "$ROTARI_LAUNCHER_CWD" "$(pwd -P)" "$NO_PROXY" "$no_proxy" "${ROTARI_LAUNCHER_PORT-unset}"`)
	launch, err := manager.Start(StartRequest{Command: command, Target: target, Kind: TargetFile})
	if err != nil {
		t.Fatal(err)
	}
	waitEnded(t, manager, launch.ID)
	realDir, _ := filepath.EvalSymlinks(dir)
	want := strings.Join([]string{target, dir, realDir, "corp.example," + noProxyLoopback, noProxyLoopback, "unset"}, "|")
	if got := string(readAllLog(t, manager, launch.ID).Data); got != want {
		t.Fatalf("command saw\n%q\nwant\n%q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("the target path was executed as shell code")
	}
}

func TestManagerReusesOnlyLiveMatchingLaunches(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{})
	target := t.TempDir()
	starts := filepath.Join(t.TempDir(), "starts")
	command := testCommand("sleeper", `echo started >> "$STARTS"; sleep 300`)
	manager.config.Env = append(manager.config.Env, "STARTS="+starts)
	request := StartRequest{Command: command, Target: target, Kind: TargetDirectory}

	// Concurrent duplicate clicks spawn once.
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			launch, err := manager.Start(request)
			if err != nil {
				t.Error(err)
			}
			ids[i] = launch.ID
		}()
	}
	wg.Wait()
	if unique := slices.Compact(slices.Sorted(slices.Values(ids))); len(unique) != 1 {
		t.Fatalf("duplicate starts returned launches %v", ids)
	}
	first := ids[0]
	again, err := manager.Start(request)
	if err != nil || again.ID != first || !again.Reused {
		t.Fatalf("repeated Start() = %+v, %v; want reuse of %s", again, err, first)
	}

	// Another target, cwd, or command name is another launch.
	other := t.TempDir()
	for _, different := range []StartRequest{
		{Command: command, Target: other, Kind: TargetDirectory},
		{Command: func() Command { c := command; c.CWD = CWDParent; return c }(), Target: target, Kind: TargetDirectory},
		{Command: func() Command { c := command; c.Name = "other"; return c }(), Target: target, Kind: TargetDirectory},
		{Command: command, Target: target, Kind: TargetDirectory, NewInstance: true},
	} {
		launch, err := manager.Start(different)
		if err != nil || launch.ID == first || launch.Reused {
			t.Fatalf("Start(%+v) = %+v, %v; want a new launch", different, launch, err)
		}
	}

	// An ended launch is never reused.
	if _, err := manager.Stop(first); err != nil {
		t.Fatal(err)
	}
	for _, launch := range manager.List() {
		if launch.ID != first && launch.State == StateRunning && launch.CWD == target && launch.Name == command.Name {
			// The explicit extra instance is still live and matches.
			if _, err := manager.Stop(launch.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	fresh, err := manager.Start(request)
	if err != nil || fresh.ID == first || fresh.Reused {
		t.Fatalf("Start() after stop = %+v, %v; want a new launch", fresh, err)
	}
	// 1 deduplicated + 4 distinct + 1 after stop. The last one may not have
	// written yet.
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, _ := os.ReadFile(starts)
		got := strings.Count(string(data), "started")
		if got == 6 {
			break
		}
		if got > 6 || time.Now().After(deadline) {
			t.Fatalf("commands started %d times, want 6", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestManagerStopEndsProcessGroup(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{})
	pidFile := filepath.Join(t.TempDir(), "pid")
	manager.config.Env = append(manager.config.Env, "PID_FILE="+pidFile)
	launch, err := manager.Start(StartRequest{Command: testCommand("s", `sleep 300 & echo $! > "$PID_FILE"; wait`),
		Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	child := readNumberFile(t, pidFile)
	stopped, err := manager.Stop(launch.ID)
	if err != nil || stopped.State != StateStopped || stopped.Signal != syscall.SIGTERM.String() || stopped.EndedAt == nil {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}
	waitProcessGone(t, child, "after Stop")
	if again, err := manager.Stop(launch.ID); err != nil || again.State != StateStopped {
		t.Fatalf("second Stop() = %+v, %v", again, err)
	}
	if _, err := manager.Stop("unknown"); !errors.Is(err, ErrUnknownLaunch) {
		t.Fatalf("Stop(unknown) = %v", err)
	}
}

func TestManagerRecordsStartFailures(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{})
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	missingCWD := testCommand("fixed", "true")
	missingCWD.CWD = filepath.Join(dir, "missing")
	for name, test := range map[string]struct {
		request  StartRequest
		recorded bool
		want     string
	}{
		"missing cwd":    {StartRequest{Command: missingCWD, Target: dir, Kind: TargetDirectory}, true, "working directory"},
		"missing target": {StartRequest{Command: testCommand("t", "true"), Target: filepath.Join(dir, "gone"), Kind: TargetFile}, true, "target"},
		"file cwd":       {StartRequest{Command: func() Command { c := testCommand("f", "true"); c.CWD = file; return c }(), Target: dir, Kind: TargetDirectory}, true, "not a directory"},
		"relative":       {StartRequest{Command: testCommand("r", "true"), Target: "relative", Kind: TargetDirectory}, false, "absolute"},
		"wrong kind":     {StartRequest{Command: func() Command { c := testCommand("d", "true"); c.Targets = []TargetKind{TargetDirectory}; return c }(), Target: file, Kind: TargetFile}, false, "does not accept a file"},
		"service mode":   {StartRequest{Command: func() Command { c := testCommand("v", "true"); c.Mode = ModeService; return c }(), Target: dir, Kind: TargetDirectory}, false, "cannot serve services"},
	} {
		t.Run(name, func(t *testing.T) {
			before := len(manager.List())
			launch, err := manager.Start(test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Start() = %+v, %v; want an error containing %q", launch, err, test.want)
			}
			if recorded := len(manager.List()) > before; recorded != test.recorded {
				t.Fatalf("failure recorded = %v, want %v", recorded, test.recorded)
			}
			if test.recorded && (launch.State != StateFailed || !strings.Contains(launch.Error, test.want) || launch.EndedAt == nil) {
				t.Fatalf("recorded failure = %+v", launch)
			}
		})
	}
}

func TestManagerLimits(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{MaxLive: 1, MaxEnded: 1})
	sleeper, err := manager.Start(StartRequest{Command: testCommand("s", "sleep 300"), Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(StartRequest{Command: testCommand("t", "true"), Target: t.TempDir(), Kind: TargetDirectory}); !errors.Is(err, ErrTooManyLaunches) {
		t.Fatalf("Start() over the limit = %v", err)
	}
	if _, err := manager.Stop(sleeper.ID); err != nil {
		t.Fatal(err)
	}
	second, err := manager.Start(StartRequest{Command: testCommand("t", "true"), Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	waitEnded(t, manager, second.ID)
	launches := manager.List()
	if len(launches) != 1 || launches[0].ID != second.ID {
		t.Fatalf("after two ended launches with MaxEnded 1: %+v", launches)
	}
	if _, err := os.Stat(filepath.Join(manager.dir, sleeper.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the dropped launch's log directory remains: %v", err)
	}
	if _, err := manager.ReadLog(sleeper.ID, 0, 10); !errors.Is(err, ErrUnknownLaunch) {
		t.Fatalf("ReadLog(dropped) = %v", err)
	}
}

func TestManagerCloseStopsLaunchesAndCleansUp(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{})
	pidFile := filepath.Join(t.TempDir(), "pid")
	manager.config.Env = append(manager.config.Env, "PID_FILE="+pidFile)
	launch, err := manager.Start(StartRequest{Command: testCommand("s", `trap '' TERM; sleep 300 & echo $! > "$PID_FILE"; wait`),
		Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	child := readNumberFile(t, pidFile)
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	waitProcessGone(t, child, "after Close")
	if got, _ := manager.Get(launch.ID); got.State != StateStopped || got.Signal != syscall.SIGKILL.String() {
		t.Fatalf("launch after Close = %+v", got)
	}
	if _, err := os.Stat(manager.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime directory remains after Close: %v", err)
	}
	if _, err := manager.Start(StartRequest{Command: testCommand("t", "true"), Target: t.TempDir(), Kind: TargetDirectory}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start() after Close = %v", err)
	}
}

func TestManagerKeepsRecentOutput(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{LogSegmentBytes: 64})
	launch, err := manager.Start(StartRequest{Command: testCommand("p", `i=0; while [ $i -lt 100 ]; do printf '%03d\n' $i; i=$((i+1)); done`),
		Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	waitEnded(t, manager, launch.ID)
	log := readAllLog(t, manager, launch.ID)
	if log.Total != 400 || !log.Dropped || log.Next != 400 {
		t.Fatalf("log = offset %d next %d total %d dropped %v", log.Offset, log.Next, log.Total, log.Dropped)
	}
	// Two 64-byte segments keep the last 64 to 128 bytes. Lines and
	// segments are both multiples of 4 bytes, so the kept output is whole
	// lines from Offset/4 to 99.
	var want strings.Builder
	for i := log.Offset / 4; i < 100; i++ {
		fmt.Fprintf(&want, "%03d\n", i)
	}
	if string(log.Data) != want.String() || len(log.Data) < 64 || len(log.Data) > 128 {
		t.Fatalf("kept output from %d = %q", log.Offset, log.Data)
	}
	// Paging from the kept start returns the same bytes.
	page, err := manager.ReadLog(launch.ID, log.Offset, 10)
	if err != nil || page.Dropped || string(page.Data) != string(log.Data[:10]) || page.Next != log.Offset+10 {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

func TestRuntimeDirSweepRemovesOnlyAbandonedDirectories(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	live, lock, err := createRuntimeDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	abandoned, abandonedLock, err := createRuntimeDir(temp)
	if err != nil {
		t.Fatal(err)
	}
	abandonedLock.Close() // what the kernel does when its process dies
	unlocked := filepath.Join(temp, runtimeDirPattern+"unlocked")
	fresh := filepath.Join(temp, runtimeDirPattern+"fresh")
	other := filepath.Join(temp, "unrelated")
	for _, dir := range []string{unlocked, fresh, other} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * staleRuntimeAge)
	for _, dir := range []string{live, abandoned, unlocked, other} {
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
	}
	sweepRuntimeDirs(temp, time.Now())
	for dir, kept := range map[string]bool{live: true, abandoned: false, unlocked: false, fresh: true, other: true} {
		_, err := os.Stat(dir)
		if exists := err == nil; exists != kept {
			t.Errorf("%s exists = %v, want %v", filepath.Base(dir), exists, kept)
		}
	}
}
