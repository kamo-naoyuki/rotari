package executor

import (
	"fmt"
	"path/filepath"
	"strings"
)

func StatusWrapperScript(command []string, jobDir string, environment []string, workingDirectory string) string {
	statusPath := filepath.Join(jobDir, "status.json")
	quoted := make([]string, 0, len(command))
	for _, arg := range command {
		quoted = append(quoted, ShellQuote(arg))
	}
	commandLine := strings.Join(quoted, " ")
	exports := make([]string, 0, len(environment))
	for _, entry := range environment {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			exports = append(exports, "export "+parts[0]+"="+ShellQuote(parts[1]))
		}
	}
	changeDirectory := ""
	if workingDirectory != "" {
		changeDirectory = "cd " + ShellQuote(workingDirectory) + " || exit 1\n"
	}
	return fmt.Sprintf(`#!/bin/sh
set +e
status_path=%s
%s
%s
hostname=$(hostname 2>/dev/null || true)
write_status() {
    phase=$1
    code=$2
    tmp="${status_path}.tmp.$$"
    now=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)
    if [ "$phase" = "running" ]; then
	printf '{"phase":"running","hosts":["%%s"],"started_at":"%%s"}\n' "$hostname" "$now" > "$tmp"
    else
	printf '{"phase":"%%s","hosts":["%%s"],"exit_code":%%s,"finished_at":"%%s"}\n' "$phase" "$hostname" "$code" "$now" > "$tmp"
    fi
    mv -f "$tmp" "$status_path"
}
write_status running 0
trap 'write_status cancelled 143; exit 143' TERM
trap 'write_status cancelled 130; exit 130' INT
trap 'write_status cancelled 131; exit 131' QUIT
%s
code=$?
write_status finished "$code"
exit "$code"
`, ShellQuote(statusPath), strings.Join(exports, "\n"), changeDirectory, commandLine)
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
