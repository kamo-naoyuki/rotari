package executor

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

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
