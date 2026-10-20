package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandExecutable(t *testing.T) {
	t.Parallel()
	for command, want := range map[string]string{
		`tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT"`:   "tensorboard",
		`PYTHONPATH=/x CUDA_VISIBLE_DEVICES=0 python3 -m tb`: "python3",
		`conda run -n tb tensorboard --logdir x`:             "conda",
		`/opt/tools/plot "$ROTARI_LAUNCHER_ARTIFACT"`:        "/opt/tools/plot",
		`cd "$ROTARI_LAUNCHER_CWD" && make plot`:             "",
		`exec "$TOOL"`:                                       "",
		`"$TOOL" --flag`:                                     "",
		`wc -l "$ROTARI_LAUNCHER_ARTIFACT" | sort`:           "wc",
		`if true; then`:                                      "",
	} {
		if got := (Command{Command: command}).Executable(); got != want {
			t.Errorf("Executable(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestCommandUnavailable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExecutable := filepath.Join(dir, "data")
	if err := os.WriteFile(notExecutable, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for command, want := range map[string]string{
		"sh -c true":                                   "",
		"rotari-surely-missing-tool --x":               "rotari-surely-missing-tool was not found on PATH",
		tool + " x":                                    "",
		notExecutable + " x":                           "is not an executable file",
		filepath.Join(dir, "missing") + " x":           "is not an executable file",
		"./relative-tool x":                            "",
		"~/bin/tool x":                                 "",
		`cd "$ROTARI_LAUNCHER_CWD" && missing-tool-xy`: "",
	} {
		got := (Command{Command: command}).Unavailable()
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("Unavailable(%q) = %q, want %q", command, got, want)
		}
	}
}
