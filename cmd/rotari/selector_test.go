package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
)

// Helpers for the in-process tests that still use newSelectorFixture. The
// selector table itself is checked through the binary in
// conformance/selector_test.go.

func (fixture selectorFixture) expand(args string) []string {
	var expanded []string
	for _, arg := range strings.Fields(args) {
		arg = strings.ReplaceAll(arg, "{B}", fixture.BaseDir)
		arg = strings.ReplaceAll(arg, "{OB}", fixture.OtherBaseDir)
		for key, value := range fixture.Runs {
			arg = strings.ReplaceAll(arg, "{run:"+key+"}", value)
		}
		for key, value := range fixture.Attempts {
			arg = strings.ReplaceAll(arg, "{att:"+key+"}", value)
		}
		for key, value := range fixture.Jobs {
			arg = strings.ReplaceAll(arg, "{job:"+key+"}", value)
		}
		expanded = append(expanded, arg)
	}
	return expanded
}

// symbolic replaces the fixture's generated IDs and directories in text with
// their keys.
func (fixture selectorFixture) symbolic(text string) string {
	text = strings.ReplaceAll(text, fixture.BaseDir, "{B}")
	text = strings.ReplaceAll(text, fixture.OtherBaseDir, "{OB}")
	for key, value := range fixture.Attempts {
		text = strings.ReplaceAll(text, value, "{att:"+key+"}")
	}
	for key, value := range fixture.Runs {
		text = strings.ReplaceAll(text, value, "{run:"+key+"}")
	}
	for key, value := range fixture.Jobs {
		text = strings.ReplaceAll(text, value, "{job:"+key+"}")
	}
	return text
}

// startServer runs the base directory's supervisor in process for a run
// case and stops it when the test ends.
func (fixture selectorFixture) startServer(t *testing.T) {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- runServer(fixture.BaseDir) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if response, err := serverinternal.SendRequest(fixture.BaseDir, serverinternal.Request{Op: serverinternal.OpPing}); err == nil && response.OK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() {
		_, _ = serverinternal.SendRequest(fixture.BaseDir, serverinternal.Request{Op: serverinternal.OpShutdown})
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Log("server did not stop")
		}
	})
}

func captureSelectorOutput(run func() int) (int, string) {
	stdout, stderr := os.Stdout, os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout, os.Stderr = writer, writer
	var output bytes.Buffer
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(&output, reader)
		close(copied)
	}()
	code := run()
	writer.Close()
	<-copied
	os.Stdout, os.Stderr = stdout, stderr
	return code, output.String()
}
