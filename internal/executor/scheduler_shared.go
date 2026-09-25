package executor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ExpandShellOptions splits shell-quoted option strings (e.g. "-p short --cpus-per-task=2")
// into individual CLI arguments. Shared by any executor that accepts free-form option strings,
// and by cmd/rotari's queue validation, which checks option quoting before a run starts.
func ExpandShellOptions(options []string) ([]string, error) {
	expanded := make([]string, 0, len(options))
	for _, option := range options {
		words, err := splitShellWords(option)
		if err != nil {
			return nil, fmt.Errorf("invalid executor option %q: %w", option, err)
		}
		expanded = append(expanded, words...)
	}
	return expanded, nil
}

func splitShellWords(input string) ([]string, error) {
	var words []string
	var word strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	hasContent := false

	flush := func() {
		if hasContent {
			words = append(words, word.String())
			word.Reset()
			hasContent = false
		}
	}

	for _, char := range input {
		if escaped {
			word.WriteRune(char)
			hasContent = true
			escaped = false
			continue
		}
		if inSingleQuote {
			if char == '\'' {
				inSingleQuote = false
			} else {
				word.WriteRune(char)
				hasContent = true
			}
			continue
		}
		if inDoubleQuote {
			switch char {
			case '"':
				inDoubleQuote = false
			case '\\':
				escaped = true
			default:
				word.WriteRune(char)
				hasContent = true
			}
			continue
		}
		switch {
		case char == '\\':
			escaped = true
		case char == '\'':
			inSingleQuote = true
			hasContent = true
		case char == '"':
			inDoubleQuote = true
			hasContent = true
		case char == ' ' || char == '\t' || char == '\n':
			flush()
		default:
			word.WriteRune(char)
			hasContent = true
		}
	}

	if escaped {
		return nil, errors.New("trailing escape")
	}
	if inSingleQuote || inDoubleQuote {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return words, nil
}

// SchedulerCommandHint clarifies two common causes of an opaque scheduler
// control command failure: the scheduler's client tools (scontrol/qsig/
// bstop/...) not being installed on this host -- e.g. a web/CLI host outside
// the cluster that only shares the state directory over NFS, where the raw
// "executable file not found in $PATH" is easy to mistake for the job itself
// not running -- and the command running fine but being rejected by the
// scheduler itself (e.g. suspending a job that is still queued/pending
// rather than actually running), where Go's generic "exit status 1" hides
// the scheduler's own explanation unless the command's output is folded in.
func SchedulerCommandHint(binary string, output []byte, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%q is not installed on this host; run this command from a host with that scheduler's client tools (%w)", binary, err)
	}
	if text := strings.TrimSpace(string(output)); text != "" {
		return fmt.Errorf("%w: %s", err, text)
	}
	return err
}

type schedulerSubmissionFailureKind uint8

const (
	schedulerSubmissionPermanent schedulerSubmissionFailureKind = iota
	schedulerSubmissionTransient
	schedulerSubmissionAmbiguous
)

func classifySchedulerSubmissionFailure(err error, output []byte) schedulerSubmissionFailureKind {
	if errors.Is(err, exec.ErrNotFound) {
		return schedulerSubmissionPermanent
	}
	message := strings.ToLower(err.Error() + " " + string(output))
	if strings.Contains(message, "timed out after") || strings.Contains(message, "returned an empty job id") || strings.Contains(message, "returned no job id") {
		return schedulerSubmissionAmbiguous
	}
	for _, pattern := range []string{
		"permission denied", "not authorized", "unauthorized", "access denied",
		"invalid partition", "invalid queue", "invalid account", "invalid qos",
		"invalid resource", "invalid option", "unrecognized option", "illegal option",
	} {
		if strings.Contains(message, pattern) {
			return schedulerSubmissionPermanent
		}
	}
	for _, pattern := range []string{
		"controller unavailable", "unable to contact", "temporarily unavailable",
		"try again", "connection refused", "connection reset", "broken pipe",
		"service unavailable", "server unavailable", "not responding",
	} {
		if strings.Contains(message, pattern) {
			return schedulerSubmissionTransient
		}
	}
	return schedulerSubmissionPermanent
}

type schedulerSubmissionRetryPolicy struct {
	RetryLimit   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Timing       schedulerTiming
}

var schedulerSubmissionRetries = schedulerSubmissionRetryPolicy{
	RetryLimit: 2, InitialDelay: time.Second, MaxDelay: 30 * time.Second,
}

var schedulerSubmissionSpacing = newSchedulerSubmissionGate(100 * time.Millisecond)

type schedulerSubmissionGate struct {
	mu       sync.Mutex
	interval time.Duration
	timing   schedulerTiming
	next     map[string]time.Time
}

func newSchedulerSubmissionGate(interval time.Duration) *schedulerSubmissionGate {
	return &schedulerSubmissionGate{interval: interval, next: make(map[string]time.Time)}
}

func (gate *schedulerSubmissionGate) wait(scheduler string, interval time.Duration) {
	if interval <= 0 {
		interval = gate.interval
	}
	if interval <= 0 {
		return
	}
	timing := gate.timing.withDefaults()
	gate.mu.Lock()
	now := timing.Now()
	scheduled := now
	if next := gate.next[scheduler]; next.After(scheduled) {
		scheduled = next
	}
	gate.next[scheduler] = scheduled.Add(interval)
	gate.mu.Unlock()
	if delay := scheduled.Sub(now); delay > 0 {
		timing.Sleep(delay)
	}
}

func (policy schedulerSubmissionRetryPolicy) submit(logf func(string, ...any), scheduler string, submit func() ([]byte, error)) ([]byte, error) {
	timing := policy.Timing.withDefaults()
	for retry := 0; ; retry++ {
		output, err := submit()
		if err == nil || classifySchedulerSubmissionFailure(err, output) != schedulerSubmissionTransient || retry >= policy.RetryLimit {
			return output, err
		}
		delay := policy.retryDelay(retry, timing)
		if logf != nil {
			logf("retry scheduler=%s attempt=%d delay=%s\n", scheduler, retry+1, delay)
		}
		timing.Sleep(delay)
	}
}

func (policy schedulerSubmissionRetryPolicy) retryDelay(retry int, timing schedulerTiming) time.Duration {
	delay := policy.InitialDelay
	for attempt := 0; attempt < retry && delay < policy.MaxDelay; attempt++ {
		delay *= 2
	}
	if delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}
	delay = timing.Jitter(delay)
	if delay < 0 {
		return 0
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// RejectArraySchedulerOptions rejects executor options that would conflict
// with the array-index flags rotari itself supplies (e.g. Slurm's --array),
// used both by array submission and by cmd/rotari's queue validation.
func RejectArraySchedulerOptions(options []string, names ...string) error {
	expanded, err := ExpandShellOptions(options)
	if err != nil {
		return err
	}
	for _, option := range expanded {
		for _, name := range names {
			if option == name || strings.HasPrefix(option, name+"=") {
				return fmt.Errorf("executor options must not include %s when rotari --array is used", name)
			}
		}
	}
	return nil
}

func schedulerArrayWrapperScript(jobs []model.JobSpec, taskVariable string) string {
	quoted := make([]string, 0, len(jobs[0].Command))
	for _, arg := range jobs[0].Command {
		quoted = append(quoted, ShellQuote(arg))
	}
	caseLines := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if caseLine, ok := schedulerArrayCaseLine(job); ok {
			caseLines = append(caseLines, caseLine)
		}
	}
	return "#!/bin/sh\nset +e\ncase \"$" + taskVariable + "\" in\n" + strings.Join(caseLines, "\n") + "\n    *) exit 1 ;;\nesac\nexec >\"$job_dir/output\" 2>&1\nstatus_path=\"$job_dir/status.json\"\nhostname=$(hostname 2>/dev/null || true)\nwrite_status() {\n    phase=$1\n    code=$2\n    tmp=\"${status_path}.tmp.$$\"\n    now=$(date -u +%Y-%m-%dT%H:%M:%SZ)\n    if [ \"$phase\" = \"running\" ]; then\n        printf '{\"phase\":\"running\",\"hosts\":[\"%s\"],\"started_at\":\"%s\"}\n' \"$hostname\" \"$now\" > \"$tmp\"\n    else\n        printf '{\"phase\":\"%s\",\"hosts\":[\"%s\"],\"exit_code\":%s,\"finished_at\":\"%s\"}\n' \"$phase\" \"$hostname\" \"$code\" \"$now\" > \"$tmp\"\n    fi\n    mv -f \"$tmp\" \"$status_path\"\n}\nwrite_status running 0\ntrap 'write_status cancelled 143; exit 143' TERM\ntrap 'write_status cancelled 130; exit 130' INT\n" + strings.Join(quoted, " ") + "\ncode=$?\nwrite_status finished \"$code\"\nexit \"$code\"\n"
}

func schedulerArrayCaseLine(job model.JobSpec) (string, bool) {
	if !state.IsValidPathElement(job.ID) || job.ArrayTaskID == nil {
		return "", false
	}
	exports := make([]string, 0, len(job.Environment))
	jobDir := ""
	for _, entry := range job.Environment {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] == model.EnvJobDir {
			jobDir = parts[1]
			continue
		}
		exports = append(exports, "export "+parts[0]+"="+ShellQuote(parts[1]))
	}
	if jobDir == "" {
		jobDir = "$ROTARI_RUN_DIR/" + job.ID
	}
	changeDirectory := ""
	if job.WorkingDirectory != "" {
		changeDirectory = "        cd " + ShellQuote(job.WorkingDirectory) + " || exit 1\n"
	}
	return fmt.Sprintf("    %d)\n        %s\n        job_dir=%s\n        export %s=%s\n        mkdir -p \"$job_dir\" || exit 1\n%s        ;;", *job.ArrayTaskID, strings.Join(exports, "\n        "), ShellQuote(jobDir), model.EnvJobDir, ShellQuote(jobDir), changeDirectory), true
}

type schedulerPollingPolicy struct {
	AccountingWait   time.Duration
	PollInterval     time.Duration
	UnavailableError string
	JobState         func() schedulerQuery
	Accounting       func() schedulerAccounting
	Timing           schedulerTiming
}

const schedulerPollingBackoffMax = 30 * time.Second

type schedulerQuery struct {
	State  string
	Failed bool
}

type schedulerAccounting struct {
	Status   WrapperStatus
	Resolved bool
	Failed   bool
}

type schedulerTiming struct {
	Now    func() time.Time
	Sleep  func(time.Duration)
	Jitter func(time.Duration) time.Duration
}

func (timing schedulerTiming) withDefaults() schedulerTiming {
	if timing.Now == nil {
		timing.Now = time.Now
	}
	if timing.Sleep == nil {
		timing.Sleep = time.Sleep
	}
	if timing.Jitter == nil {
		timing.Jitter = func(delay time.Duration) time.Duration { return delay }
	}
	return timing
}

// waitForSchedulerResult implements the scheduler-independent polling contract.
// Each scheduler supplies only its native queue and accounting queries.
func waitForSchedulerResult(store state.Store, jobDir, jobID string, command []string, policy schedulerPollingPolicy) model.JobResult {
	statusPath := filepath.Join(jobDir, "status.json")
	timing := policy.Timing.withDefaults()
	var accountingDeadline time.Time
	failures := 0
	for {
		if result, finished := wrapperJobResult(store, statusPath, jobID, command); finished {
			return result
		}
		query := policy.JobState()
		if query.State != "" {
			failures = 0
			WriteSchedulerStatus(store, jobDir, query.State, timing.Now())
		} else {
			result, resolved, accountingFailed := policy.resolveMissingSchedulerState(store, jobDir, statusPath, jobID, command, timing, &accountingDeadline)
			if resolved {
				return result
			}
			failures = nextSchedulerPollingFailures(failures, query.Failed || accountingFailed)
		}
		timing.Sleep(policy.pollingDelay(failures, timing))
	}
}

func nextSchedulerPollingFailures(failures int, failed bool) int {
	if failed {
		return failures + 1
	}
	return 0
}

func (policy schedulerPollingPolicy) pollingDelay(failures int, timing schedulerTiming) time.Duration {
	delay := policy.PollInterval
	for attempt := 1; attempt < failures && delay < schedulerPollingBackoffMax; attempt++ {
		delay *= 2
	}
	if delay > schedulerPollingBackoffMax {
		delay = schedulerPollingBackoffMax
	}
	delay = timing.Jitter(delay)
	if delay < 0 {
		return 0
	}
	if delay > schedulerPollingBackoffMax {
		return schedulerPollingBackoffMax
	}
	return delay
}

func wrapperJobResult(store state.Store, statusPath, jobID string, command []string) (model.JobResult, bool) {
	status, ok := LoadWrapperStatus(store, statusPath)
	if !ok || status.Phase != "finished" {
		return model.JobResult{}, false
	}
	return jobResultFromStatus(jobID, command, status), true
}

func (policy schedulerPollingPolicy) resolveMissingSchedulerState(store state.Store, jobDir, statusPath, jobID string, command []string, timing schedulerTiming, accountingDeadline *time.Time) (model.JobResult, bool, bool) {
	// Nudge NFS clients to discard stale cache entries before reading the
	// scheduler-independent wrapper result a second time.
	_, _ = os.ReadDir(jobDir)
	if result, finished := wrapperJobResult(store, statusPath, jobID, command); finished {
		return result, true, false
	}
	if accountingDeadline.IsZero() {
		*accountingDeadline = timing.Now().Add(policy.AccountingWait)
	}
	accounting := policy.Accounting()
	if accounting.Resolved {
		_ = state.WriteJSON(statusPath, accounting.Status)
		return jobResultFromStatus(jobID, command, accounting.Status), true, accounting.Failed
	}
	if timing.Now().After(*accountingDeadline) {
		return model.JobResult{ID: jobID, Command: command, ExitCode: 1, Error: policy.UnavailableError}, true, accounting.Failed
	}
	return model.JobResult{}, false, accounting.Failed
}
