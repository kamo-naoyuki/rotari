package main

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/resolve"
)

func TestWaitOutputTagsOnlyMultipleRuns(t *testing.T) {
	oldCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return false }
	t.Cleanup(func() { terminalCheck = oldCheck })
	target := resolve.Run{BaseDir: "/state", ProjectName: "build", RunID: "run-a12f"}
	for _, multiple := range []bool{false, true} {
		var buffer bytes.Buffer
		output := newWaitOutput(target, multiple, &sync.Mutex{})
		output.stdout = &buffer
		_, _ = output.stdoutWriter().Write([]byte("Job failed:\n  Command: false\n"))
		text := buffer.String()
		if strings.Contains(text, "[build/…a12f]") != multiple || strings.Contains(text, "\033[") {
			t.Fatalf("multiple=%t output=%q", multiple, text)
		}
	}
}

func TestWaitOutputColorsOnlyLabelAndRespectsStreamTTY(t *testing.T) {
	oldCheck := terminalCheck
	terminalCheck = func(file *os.File) bool { return file == os.Stdout }
	t.Cleanup(func() { terminalCheck = oldCheck })
	var stdout, stderr bytes.Buffer
	output := newWaitOutput(resolve.Run{ProjectName: "build", RunID: "run-a12f"}, true, &sync.Mutex{})
	output.stdout, output.stderr = &stdout, &stderr
	_, _ = output.stdoutWriter().Write([]byte("progress: 1/3\n"))
	_, _ = output.stderrWriter().Write([]byte("error\n"))
	if !strings.Contains(stdout.String(), "\033[38;5;") || !strings.Contains(stdout.String(), "] \033[0mprogress: 1/3") {
		t.Fatalf("color was not confined to the label: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "\033[") {
		t.Fatalf("redirected stderr was colored: %q", stderr.String())
	}
}

func TestWaitOutputSerializesWholeEvents(t *testing.T) {
	oldCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return false }
	t.Cleanup(func() { terminalCheck = oldCheck })
	var buffer bytes.Buffer
	var mu sync.Mutex
	var workers sync.WaitGroup
	for _, name := range []string{"alpha", "beta"} {
		output := newWaitOutput(resolve.Run{ProjectName: name, RunID: name}, true, &mu)
		output.stdout = &buffer
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 20 {
				_, _ = output.stdoutWriter().Write([]byte(name + "\n  details " + name + "\n"))
			}
		}()
	}
	workers.Wait()
	lines := strings.Split(strings.TrimSuffix(buffer.String(), "\n"), "\n")
	if len(lines) != 80 {
		t.Fatalf("event lines = %d", len(lines))
	}
	for index := 0; index < len(lines); index += 2 {
		name := strings.Fields(lines[index])[1]
		if !strings.HasSuffix(lines[index+1], "details "+name) {
			t.Fatalf("interleaved event: %q / %q", lines[index], lines[index+1])
		}
	}
}

func TestWaitFlushJSONKeepsCompletedResults(t *testing.T) {
	var first, second bytes.Buffer
	first.WriteString("{\"run_id\":\"first\"}\n")
	second.WriteString("{\"run_id\":\"second\"}\n")
	_, text := captureWorkflowStdout(t, func() int {
		flushWaitJSON([]*waitOutput{{stdout: &first}, {stdout: &second}}, true)
		return 0
	})
	if string(text) != first.String()+second.String() {
		t.Fatalf("completed JSON lost or reordered: %s", text)
	}
}

func TestWaitIdentityColorsAvoidCollisionsUntilPaletteExhausted(t *testing.T) {
	used := make(map[int]bool)
	for range len(waitIdentityColors) {
		color := availableWaitColor(waitIdentityColors[0], used)
		if used[color] {
			t.Fatalf("reused identity color %d before palette exhaustion", color)
		}
		used[color] = true
	}
	if color := availableWaitColor(waitIdentityColors[0], used); color != waitIdentityColors[0] {
		t.Fatalf("exhausted palette fallback = %d", color)
	}
}
