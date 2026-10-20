package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestLocalGroupEvidence(t *testing.T) {
	for _, test := range []struct {
		name, recorded, current string
		err                     error
		state                   string
		possibly                bool
	}{
		{"matching live leader", "123", "123", nil, "active", false},
		{"gone group", "123", "", syscall.ESRCH, "stopped", false},
		{"gone legacy leader identity", "", "", syscall.ESRCH, "stopped", false},
		{"leader exited but children remain", "123", "", nil, "unknown", true},
		{"legacy live group", "", "123", nil, "unknown", true},
		{"reused leader", "123", "456", nil, "unknown", true},
		{"permission denied", "123", "123", syscall.EPERM, "unknown", true},
		{"inspection error", "123", "123", syscall.EIO, "unknown", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, reason, possibly := localGroupEvidence(test.recorded, test.current, test.err)
			if got != test.state || possibly != test.possibly || reason == "" {
				t.Fatalf("got %s %q %t", got, reason, possibly)
			}
		})
	}
}

func TestLocalExecutionIdentityAndScope(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if state.PIDNamespace() == "" {
		t.Skip("PID namespace unavailable")
	}
	for _, test := range []struct {
		name   string
		change func(*localIdentity)
		want   string
	}{
		{"same location gone group", func(*localIdentity) {}, "stopped"},
		{"remote", func(id *localIdentity) { id.Host += "-remote" }, "unknown"},
		{"missing host", func(id *localIdentity) { id.Host = "" }, "unknown"},
		{"other namespace", func(id *localIdentity) { id.PIDNamespace = "pid:[other]" }, "unknown"},
		{"missing namespace", func(id *localIdentity) { id.PIDNamespace = "" }, "unknown"},
		{"partial launch", func(id *localIdentity) { id.PID = 0 }, "unknown"},
		{"invalid PID range", func(id *localIdentity) { pid := int64(1) << 32; id.PID = int(pid) }, "unknown"},
		{"backend mismatch", func(id *localIdentity) { id.Executor = "ssh" }, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			identity := localIdentity{Executor: "local", Host: host, PID: 1 << 30, PIDNamespace: state.PIDNamespace(), ProcessStart: "1"}
			test.change(&identity)
			if err := testStore().WriteJSON(filepath.Join(dir, "job.json"), identity); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(dir, "job.json"))
			if err != nil {
				t.Fatal(err)
			}
			got := NewExecutionProber(testStore(), context.Background()).Assess(dir, "local")
			after, err := os.ReadFile(filepath.Join(dir, "job.json"))
			if err != nil {
				t.Fatal(err)
			}
			if got.State != test.want || got.Reason == "" || got.ObservedAt == "" || got.Scope == "" || string(before) != string(after) {
				t.Fatalf("assessment=%+v", got)
			}
		})
	}
}

func TestLocalSubmitPublishesProbeIdentity(t *testing.T) {
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", Executor: "local", Command: []string{"sleep", "30"}}
	local := NewLocal(testStore(), testLogf)
	handle, err := local.Submit(runDir, job, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Cancel(dir); local.Wait(runDir, handle) }()
	var identity localIdentity
	if err := testStore().ReadJSON(filepath.Join(dir, "job.json"), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.PIDNamespace == "" || identity.ProcessStart == "" {
		t.Skip("process identity unavailable")
	}
	got := NewExecutionProber(testStore(), context.Background()).Assess(dir, "local")
	if got.State != "active" || identity.PID <= 1 {
		t.Fatalf("identity=%+v assessment=%+v", identity, got)
	}
}

func TestSchedulerExecutionEvidence(t *testing.T) {
	for _, test := range []struct {
		name, backend, queue, accounting, want string
		queueErr, accountingErr                bool
	}{
		{"slurm active beats finished", "slurm", "42|RUNNING", "42|COMPLETED|0:0", "active", false, false},
		{"slurm pending", "slurm", "42|PENDING", "", "active", false, false},
		{"slurm suspended", "slurm", "42|SUSPENDED", "", "active", false, false},
		{"slurm stopped", "slurm", "", "42|COMPLETED|0:0", "stopped", false, false},
		{"slurm failed stopped", "slurm", "42|FAILED", "42|FAILED|9:0", "stopped", false, false},
		{"slurm absent", "slurm", "", "", "unknown", false, false},
		{"slurm queue error", "slurm", "", "42|COMPLETED|0:0", "unknown", true, false},
		{"slurm active accounting with queue error", "slurm", "", "42|RUNNING|0:0", "active", true, false},
		{"slurm active accounting conflicts with terminal queue", "slurm", "42|COMPLETED", "42|RUNNING|0:0", "active", false, false},
		{"slurm accounting error", "slurm", "", "42|COMPLETED|0:0", "unknown", false, true},
		{"slurm wrong queue id", "slurm", "43|RUNNING", "42|COMPLETED|0:0", "unknown", false, false},
		{"slurm wrong accounting id", "slurm", "", "43|COMPLETED|0:0", "unknown", false, false},
		{"slurm several allocation records", "slurm", "", "42|COMPLETED|0:0\n43|RUNNING|0:0", "unknown", false, false},
		{"pbs queued", "pbs", "Job Id: 42\n job_state = Q", "", "active", false, false},
		{"pbs terminal", "pbs", "Job Id: 42\n job_state = F", "Job Id: 42\n job_state = F\n Exit_status = 9", "stopped", false, false},
		{"pbs terminal without state", "pbs", "", "Job Id: 42\n Exit_status = 0", "unknown", false, false},
		{"pbs wrong id", "pbs", "Job Id: 43\n job_state = R", "Job Id: 42\n job_state = F\n Exit_status = 0", "unknown", false, false},
		{"sge running", "sge", `<job_info><queue_info><job_list state="r"><JB_job_number>42</JB_job_number></job_list></queue_info></job_info>`, "", "active", false, false},
		{"sge pending section", "sge", `<job_info><job_info><job_list state="qw"><JB_job_number>42</JB_job_number></job_list></job_info></job_info>`, "", "active", false, false},
		{"sge stopped", "sge", `<job_info><queue_info/></job_info>`, "jobnumber 42\nexit_status 0\nfailed 0", "stopped", false, false},
		{"sge stale wrong accounting", "sge", `<job_info/>`, "jobnumber 43\nexit_status 0\nfailed 0", "unknown", false, false},
		{"sge no accounting", "sge", `<job_info/>`, "", "unknown", false, false},
		{"sge malformed listing", "sge", `<garbage/>`, "jobnumber 42\nexit_status 0\nfailed 0", "unknown", false, false},
		{"sge incomplete queue entry", "sge", `<job_info><queue_info><job_list><JB_job_number>42</JB_job_number></job_list></queue_info></job_info>`, "jobnumber 42\nexit_status 0\nfailed 0", "unknown", false, false},
		{"sge unknown state", "sge", `<job_info><queue_info><job_list state="future"><JB_job_number>42</JB_job_number></job_list></queue_info></job_info>`, "jobnumber 42\nexit_status 0\nfailed 0", "unknown", false, false},
		{"sge unrecorded native array task", "sge", `<job_info/>`, "jobnumber 42\ntaskid 1\nexit_status 0\nfailed 0", "unknown", false, false},
		{"sge duplicate accounting", "sge", `<job_info/>`, "jobnumber 42\njobnumber 42\nexit_status 0\nfailed 0", "unknown", false, false},
		{"lsf suspended", "lsf", "42 SSUSP", "", "active", false, false},
		{"lsf stopped", "lsf", "42 DONE", "42 DONE 0", "stopped", false, false},
		{"lsf wrong id", "lsf", "43 RUN", "42 DONE 0", "unknown", false, false},
		{"lsf unknown queue state", "lsf", "42 UNKWN", "42 DONE 0", "unknown", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			metadata := executionMetadata{Executor: test.backend, SlurmID: "42", PBSID: "42", SGEID: "42", LSFID: "42"}
			if err := testStore().WriteJSON(filepath.Join(dir, "job.json"), metadata); err != nil {
				t.Fatal(err)
			}
			probe := NewExecutionProber(testStore(), context.Background())
			count := 0
			probe.run = func(context.Context, []string) ([]byte, error) {
				count++
				if count == 1 {
					if test.queueErr {
						return []byte(test.queue), errors.New("queue unavailable")
					}
					return []byte(test.queue), nil
				}
				if test.accountingErr {
					return []byte(test.accounting), errors.New("accounting unavailable")
				}
				return []byte(test.accounting), nil
			}
			got := probe.Assess(dir, test.backend)
			if got.State != test.want || got.NativeID != "42" || got.Reason == "" || got.ObservedAt == "" {
				t.Fatalf("assessment=%+v", got)
			}
			if test.want == "active" && got.Source == "scheduler_queue" && count != 1 {
				t.Fatalf("positive queue evidence should not depend on accounting: %d queries", count)
			}
			if test.queueErr && !strings.Contains(got.Reason, "queue unavailable") {
				t.Fatalf("queue failure was hidden: %+v", got)
			}
			if test.queue == "42|COMPLETED" && test.want == "active" && !got.Conflict {
				t.Fatalf("queue/accounting conflict was hidden: %+v", got)
			}
		})
	}
}

func TestSSHExecutionProbeUsesPrivateTokenAndBoundedQuery(t *testing.T) {
	dir := t.TempDir()
	token := "0123456789abcdef0123456789abcdef"
	if err := testStore().WriteJSON(filepath.Join(dir, "job.json"), executionMetadata{
		Executor: "ssh", Host: "builder@example.test", SSHOptions: []string{"-p", "2222"}, RemoteToken: token,
	}); err != nil {
		t.Fatal(err)
	}
	probe := NewExecutionProber(testStore(), context.Background())
	probe.runInput = func(_ context.Context, command []string, input []byte) ([]byte, error) {
		if strings.Join(command, " ") != "-o BatchMode=yes -o LogLevel=ERROR -p 2222 -- builder@example.test sh -s" {
			t.Fatalf("SSH probe command = %v", command)
		}
		if !strings.Contains(string(input), token) {
			t.Fatal("SSH probe script omitted the private token")
		}
		return []byte("ROTARI_EXECUTION:active\n"), nil
	}
	got := probe.Assess(dir, "ssh")
	if got.State != "active" || got.Source != "ssh_remote_token" || got.NativeID != "" || !strings.Contains(got.Scope, "builder@example.test") {
		t.Fatalf("SSH execution assessment = %+v", got)
	}
	for _, output := range []string{
		"ROTARI_EXECUTION:stopped\n",
		"ROTARI_EXECUTION:unknown\n",
		"banner\nROTARI_EXECUTION:stopped\n",
		"ROTARI_EXECUTION:active\nROTARI_EXECUTION:stopped\n",
	} {
		state, _, reason := parseSSHExecutionProbe([]byte(output))
		if output == "ROTARI_EXECUTION:stopped\n" && state != "stopped" {
			t.Fatalf("stopped output state=%s", state)
		}
		if output != "ROTARI_EXECUTION:stopped\n" && state == "stopped" {
			t.Fatalf("ambiguous output was trusted: %q (%s)", output, reason)
		}
	}
}

func TestSSHExecutionProbeClassifiesRemoteProcessGroup(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	token := "0123456789abcdef0123456789abcdef"
	stateDir := filepath.Join(runtimeDir, "rotari-"+strconv.Itoa(os.Getuid()), token)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	process := exec.Command("sleep", "30")
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	processFile := filepath.Join(stateDir, "process")
	if err := os.WriteFile(processFile, []byte(strconv.Itoa(process.Process.Pid)+" "+state.ProcessStart(process.Process.Pid)+"\n"), 0o600); err != nil {
		_ = process.Process.Kill()
		_ = process.Wait()
		t.Fatal(err)
	}
	run := func() string {
		command := exec.Command("sh", "-s")
		command.Stdin = strings.NewReader(sshExecutionProbeScript(token))
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("remote probe script: %v: %s", err, output)
		}
		return strings.TrimSpace(string(output))
	}
	if got := run(); got != "ROTARI_EXECUTION:active" {
		_ = process.Process.Kill()
		_ = process.Wait()
		t.Fatalf("live remote group = %q", got)
	}
	if err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL); err != nil {
		_ = process.Process.Kill()
		_ = process.Wait()
		t.Fatal(err)
	}
	_ = process.Wait()
	if err := os.WriteFile(filepath.Join(stateDir, "finished"), []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run(); got != "ROTARI_EXECUTION:stopped" {
		t.Fatalf("finished remote group = %q", got)
	}
}

func TestExecutionProbeSGEBatchAndBudget(t *testing.T) {
	dir := t.TempDir()
	probe := NewExecutionProber(testStore(), context.Background())
	queueQueries, accountingQueries := 0, 0
	probe.run = func(_ context.Context, command []string) ([]byte, error) {
		if command[0] == "qstat" {
			queueQueries++
			return []byte(`<job_info><queue_info><job_list state="r"><JB_job_number>44</JB_job_number></job_list></queue_info><job_info><job_list state="qw"><JB_job_number>42</JB_job_number></job_list></job_info></job_info>`), nil
		}
		accountingQueries++
		if strings.Join(command, " ") != "qacct -j 43" {
			t.Fatalf("queried another native job: %v", command)
		}
		return []byte("jobnumber 43\nexit_status 9\nfailed 0"), nil
	}
	for _, test := range []struct{ id, want string }{{"42", "active"}, {"43", "stopped"}, {"44", "active"}} {
		jobDir := filepath.Join(dir, test.id)
		if err := testStore().WriteJSON(filepath.Join(jobDir, "job.json"), executionMetadata{Executor: "sge", SGEID: test.id}); err != nil {
			t.Fatal(err)
		}
		if got := probe.Assess(jobDir, "sge"); got.State != test.want || got.NativeID != test.id {
			t.Fatalf("assessment=%+v", got)
		}
	}
	if queueQueries != 1 || accountingQueries != 1 {
		t.Fatalf("SGE batch queries: queue=%d accounting=%d", queueQueries, accountingQueries)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe = NewExecutionProber(testStore(), ctx)
	probe.run = func(context.Context, []string) ([]byte, error) {
		t.Fatal("ran query after budget expired")
		return nil, nil
	}
	if got := probe.Assess(dir, "sge"); got.State != "unknown" || !strings.Contains(got.Reason, "canceled") {
		t.Fatalf("assessment=%+v", got)
	}
}

func TestExecutionQueryBounds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := runExecutionQuery(ctx, []string{"sleep", "30"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}

	output := &probeOutput{}
	if _, err := output.Write(make([]byte, schedulerProbeOutputLimit+1)); err != nil {
		t.Fatal(err)
	}
	if !output.overflow || len(output.data) != schedulerProbeOutputLimit {
		t.Fatalf("output bound failed")
	}
}

func TestExecutionProbeInvalidIdentityDoesNotQuery(t *testing.T) {
	for _, backend := range []string{"slurm", "pbs", "sge", "lsf"} {
		t.Run(backend, func(t *testing.T) {
			dir := t.TempDir()
			if err := testStore().WriteJSON(filepath.Join(dir, "job.json"), executionMetadata{Executor: backend, SlurmID: "--all", PBSID: "42 other", SGEID: "42.3", LSFID: "42;echo"}); err != nil {
				t.Fatal(err)
			}
			probe := NewExecutionProber(testStore(), context.Background())
			probe.run = func(context.Context, []string) ([]byte, error) {
				t.Fatal("queried unvalidated identity")
				return nil, nil
			}
			if got := probe.Assess(dir, backend); got.State != "unknown" || !strings.Contains(got.Reason, "identity") {
				t.Fatalf("assessment=%+v", got)
			}
		})
	}
}

func TestLocalIdentityPublicationFailurePreventsLaunch(t *testing.T) {
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", Command: []string{"touch", filepath.Join(runDir, "launched")}}
	if err := os.MkdirAll(filepath.Join(runDir, "job", "job.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := RunLocalJob(runDir, job, testStore(), testLogf)
	if result.ExitCode == 0 || !strings.Contains(result.Error, "record local launch location") {
		t.Fatalf("publication failure was ignored: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(runDir, "launched")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("started without durable location: %v", err)
	}
}
