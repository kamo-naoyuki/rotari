package interfaces

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type pairMCPReply struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type pairMCPObservation struct {
	ServerName   string
	Capabilities json.RawMessage
	Tools        []string
}

func TestMCPStdinEOF(t *testing.T) {
	covers(t, "MCP-6")
	e := support.NewEnv(t)
	for _, test := range []struct {
		name, input, wantError string
	}{
		{name: "empty"},
		{name: "requests in flight", input: mcpEOFRequests(t)},
		{name: "truncated input", input: "{", wantError: "unexpected EOF"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := e.Command("mcp")
			command.Stdin = strings.NewReader(test.input)
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			if test.wantError != "" {
				if err == nil || !strings.Contains(stderr.String(), test.wantError) {
					t.Fatalf("MCP malformed input: err=%v stderr=%q, want %q", err, stderr.String(), test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("MCP stdin EOF: %v; stderr=%s", err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("MCP stdin EOF stderr = %q", stderr.String())
			}
		})
	}
}

func mcpEOFRequests(t *testing.T) string {
	t.Helper()
	var input bytes.Buffer
	if err := writePairMCPMessage(&input, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "eof-test", "version": "1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writePairMCPMessage(&input, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		t.Fatal(err)
	}
	// EOF may arrive before the queued responses are written. The client has
	// disconnected, so shutdown must not depend on whether those writes win.
	for id := 2; id < 130; id++ {
		if err := writePairMCPMessage(&input, map[string]any{
			"jsonrpc": "2.0", "id": id, "method": "tools/list", "params": map[string]any{},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return input.String()
}

func TestCLIFlagPairMCP(t *testing.T) {
	e := support.NewEnv(t)
	config := filepath.Join(e.Root, "mcp-pair.yaml")
	configMaster := filepath.Join(e.Root, "config-master")
	envMaster := filepath.Join(e.Root, "env-master")
	selectedMaster := filepath.Join(e.Root, "selected-master")
	for _, dir := range []string{configMaster, envMaster, selectedMaster} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(config, fmt.Appendf(nil, "masterdir: %q\n", configMaster), 0o600); err != nil {
		t.Fatal(err)
	}
	env := e.WithVar("ROTARI_MASTERDIR", envMaster)
	before := support.SavePairTree(t, e.Root)
	orders := [][]string{
		{"mcp", "--config", config, "--masterdir", selectedMaster},
		{"mcp", "--masterdir", selectedMaster, "--config", config},
	}
	observations := make([]pairMCPObservation, 0, len(orders))
	for _, args := range orders {
		observation, stderr := runPairMCPHandshake(t, env, args)
		if stderr != "" {
			t.Fatalf("MCP stderr for %v = %q", args, stderr)
		}
		observations = append(observations, observation)
		if after := support.SavePairTree(t, e.Root); !reflect.DeepEqual(before.Raw(), after.Raw()) {
			t.Fatalf("MCP discovery request changed state for %v", args)
		}
	}
	if !reflect.DeepEqual(observations[0], observations[1]) {
		t.Fatalf("config/masterdir order changed MCP discovery:\n%+v\n%+v", observations[0], observations[1])
	}
}

func TestCLIFlagPairMCPSamples(t *testing.T) {
	e := support.NewEnv(t)
	config := filepath.Join(e.Root, "mcp-pair.yaml")
	for _, sample := range []struct {
		name  string
		args  []string
		value string
	}{
		{name: "config", args: []string{"--config"}, value: filepath.Join(e.Root, "mcp-config.yaml")},
		{name: "masterdir", args: []string{"--masterdir"}, value: filepath.Join(e.Root, "mcp-master")},
	} {
		t.Run(sample.name, func(t *testing.T) {
			preparePairMCPSample(t, config, sample.name, sample.value)
			before := support.SavePairTree(t, e.Root)
			assertPairMCPStdinOnly(t, e, mcpSampleArgs(sample.name, sample.args, sample.value, config), before)
		})
	}
}

func preparePairMCPSample(t *testing.T, config, name, value string) {
	t.Helper()
	if err := os.WriteFile(config, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if name == "config" {
		if err := os.WriteFile(value, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.MkdirAll(value, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mcpSampleArgs(name string, option []string, value, config string) []string {
	args := append([]string{"mcp"}, option...)
	args = append(args, value)
	if name == "config" {
		return append(args, "--masterdir", filepath.Join(filepath.Dir(config), "mcp-master"))
	}
	return append(args, "--config", config)
}

func assertPairMCPStdinOnly(t *testing.T, e *support.Env, args []string, before support.PairSavedTree) {
	t.Helper()
	observation, stderr := runPairMCPHandshake(t, e, args)
	if stderr != "" || len(observation.Tools) == 0 {
		t.Fatalf("MCP handshake failed: stderr=%q observation=%+v", stderr, observation)
	}
	if after := support.SavePairTree(t, e.Root); !reflect.DeepEqual(before.Raw(), after.Raw()) {
		t.Fatalf("MCP handshake changed state for %v", args)
	}
}

func runPairMCPHandshake(t *testing.T, e *support.Env, args []string) (pairMCPObservation, string) {
	t.Helper()
	command := e.Command(args...)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = command.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
			<-done
		}
	})

	lines := make(chan pairMCPLine, 16)
	go readPairMCPOutput(bufio.NewReader(stdout), lines)
	if err := writePairMCPMessage(stdin, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "flag-pair", "version": "1"},
		},
	}); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}
	initialized, err := readPairMCPReply(t, lines, 1)
	if err != nil {
		t.Fatalf("read initialize reply: %v", err)
	}
	var init struct {
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(initialized, &init); err != nil || init.ServerInfo.Name == "" {
		t.Fatalf("invalid MCP initialize result: %s (%v)", initialized, err)
	}
	if err := writePairMCPMessage(stdin, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		t.Fatalf("write initialized notification: %v", err)
	}
	if err := writePairMCPMessage(stdin, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}}); err != nil {
		t.Fatalf("write tools/list request: %v", err)
	}
	toolResult, err := readPairMCPReply(t, lines, 2)
	if err != nil {
		t.Fatalf("read tools/list reply: %v", err)
	}
	var listed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(toolResult, &listed); err != nil || len(listed.Tools) == 0 {
		t.Fatalf("invalid tools/list result: %s (%v)", toolResult, err)
	}
	tools := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		tools = append(tools, tool.Name)
	}
	sort.Strings(tools)

	if err := stdin.Close(); err != nil {
		t.Fatalf("close MCP stdin: %v", err)
	}
	select {
	case <-done:
		if waitErr != nil {
			t.Fatalf("MCP process did not exit cleanly: %v; stderr=%s", waitErr, stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-done
		t.Fatalf("MCP process did not stop after stdin EOF; stderr=%s", stderr.String())
	}
	return pairMCPObservation{ServerName: init.ServerInfo.Name, Capabilities: init.Capabilities, Tools: tools}, stderr.String()
}

type pairMCPLine struct {
	data []byte
	err  error
}

func readPairMCPOutput(reader *bufio.Reader, lines chan<- pairMCPLine) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			lines <- pairMCPLine{err: err}
			return
		}
		lines <- pairMCPLine{data: line}
	}
}

func readPairMCPReply(t *testing.T, lines <-chan pairMCPLine, id int) (json.RawMessage, error) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case item := <-lines:
			if item.err != nil {
				return nil, item.err
			}
			var reply pairMCPReply
			if err := json.Unmarshal(item.data, &reply); err != nil {
				return nil, fmt.Errorf("parse JSON-RPC line %q: %w", item.data, err)
			}
			if reply.ID != id {
				continue // Ignore asynchronous notifications such as tools/list_changed.
			}
			if reply.Error != nil {
				return nil, fmt.Errorf("MCP request failed: %s", reply.Error)
			}
			return reply.Result, nil
		case <-timer.C:
			return nil, fmt.Errorf("timed out waiting for MCP reply id %d", id)
		}
	}
}

func writePairMCPMessage(writer io.Writer, message map[string]any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = writer.Write(append(data, '\n'))
	return err
}
