package pairweb

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func webArgs(t *testing.T, f pairFixture, output string, flags []pairFlag) []string {
	t.Helper()
	values := map[string]string{
		"config": f.Config, "basedir": f.E.Base, "host": "127.0.0.1",
		"port": "23456", "static-dir": output, "auth-token": "static-token",
		"artifact-root": f.E.Root,
	}
	args := []string{"web"}
	for _, flag := range flags {
		switch flag.Name {
		case "allow-control":
			args = append(args, "--allow-control=false")
		case "notifications":
			args = append(args, "--notifications=false")
		case "static-artifact-contents":
			args = append(args, "--static-artifact-contents")
		default:
			value, ok := values[flag.Name]
			if !ok {
				t.Fatalf("no web sample for --%s", flag.Name)
			}
			args = append(args, "--"+flag.Name, value)
		}
	}
	if !webHasFlag(flags, "static-dir") {
		args = append(args, "--static-dir", output)
	}
	return args
}

func webHasFlag(flags []pairFlag, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

// webFiles compares the relative asset layout and permissions; index.html's
// volatile notification-session token is normalized for exact content parity.
func webFiles(t *testing.T, output string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(output, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(output, path)
		if err != nil {
			return err
		}
		files[relative] = fmt.Sprintf("%o", info.Mode().Perm())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func webSourceState(t *testing.T, root, output string) map[string]string {
	t.Helper()
	tree := support.SavePairTree(t, root).Raw()
	relative, err := filepath.Rel(root, output)
	if err != nil {
		t.Fatal(err)
	}
	prefix := relative + string(filepath.Separator)
	for path := range tree {
		if path == relative || strings.HasPrefix(path, prefix) {
			delete(tree, path)
		}
	}
	return tree
}

func webServerOnlyFlags(flags []pairFlag) []string {
	var names []string
	for _, flag := range flags {
		switch flag.Name {
		case "allow-control", "artifact-root", "auth-token", "host", "port":
			names = append(names, flag.Name)
		}
	}
	sort.Strings(names)
	return names
}

func invokeStaticWeb(t *testing.T, f pairFixture, flags []pairFlag, output string) (support.Result, map[string]string) {
	t.Helper()
	if err := os.RemoveAll(output); err != nil {
		t.Fatal(err)
	}
	before := webSourceState(t, f.E.Root, output)
	result := pairInvoke(t, f.E, webArgs(t, f, output, flags)...)
	serverOnly := webServerOnlyFlags(flags)
	if len(serverOnly) > 0 {
		want := "--" + strings.Join(serverOnly, ", --") + " cannot be combined with --static-dir"
		if result.Code != 1 || result.Stdout != "" || !strings.Contains(result.Stderr, want) {
			t.Fatalf("server-only web flags were silently ignored by static export: %s", result)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatalf("rejected static export created output: %v", err)
		}
		if after := webSourceState(t, f.E.Root, output); !reflect.DeepEqual(before, after) {
			t.Fatalf("rejected static export changed source state: %s", result)
		}
		return result, map[string]string{}
	}
	if result.Code != 0 || result.Stderr != "" {
		t.Fatalf("static export failed: %s", result)
	}
	for _, required := range []string{"index.html", "jobs/index.html", ".nojekyll"} {
		if _, err := os.Stat(filepath.Join(output, required)); err != nil {
			t.Fatalf("static export omitted %s: %v", required, err)
		}
	}
	if after := webSourceState(t, f.E.Root, output); !reflect.DeepEqual(before, after) {
		t.Fatalf("static export changed source state: %s", result)
	}
	return result, webFiles(t, output)
}

func TestCLIFlagPairWebSamples(t *testing.T) {
	f := newPairFixture(t)
	for _, command := range readPairSchema(t, f.E) {
		if command.Name != "web" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(flag.Name, func(t *testing.T) {
				output := filepath.Join(f.E.Root, "static-sample")
				_, _ = invokeStaticWeb(t, f, []pairFlag{flag}, output)
			})
		}
	}
}

func TestCLIFlagPairWeb(t *testing.T) {
	f := newPairFixture(t)
	var command pairCommand
	for _, candidate := range readPairSchema(t, f.E) {
		if candidate.Name == "web" {
			command = candidate
			break
		}
	}
	if command.Name == "" {
		t.Fatal("web is missing from schema")
	}
	pairs := commandFlagPairs(command)
	for _, pair := range pairs {
		t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
			output := filepath.Join(f.E.Root, "static-pair")
			ab, filesAB := invokeStaticWeb(t, f, []pairFlag{pair.A, pair.B}, output)
			ba, filesBA := invokeStaticWeb(t, f, []pairFlag{pair.B, pair.A}, output)
			if ab.Code != ba.Code || ab.Stdout != ba.Stdout || ab.Stderr != ba.Stderr || !reflect.DeepEqual(filesAB, filesBA) {
				t.Fatalf("order-dependent static web pair:\n%s\n%s\nfile content equal=%t", ab, ba, reflect.DeepEqual(filesAB, filesBA))
			}
		})
	}
	t.Logf("static web pairs=%d invocations=%d; live-server-only options explicitly rejected with static export", len(pairs), 2*len(pairs))
}

func TestCLIFlagPairWebStaticServerOptions(t *testing.T) {
	covers(t, "WEB-3")
	f := newPairFixture(t)
	for _, name := range []string{"allow-control", "artifact-root", "auth-token", "host", "port"} {
		t.Run(name, func(t *testing.T) {
			result, _ := invokeStaticWeb(t, f, []pairFlag{{Name: name}}, filepath.Join(f.E.Root, "static-rejected"))
			if !strings.Contains(result.Stderr, "only apply to the live web server") {
				t.Fatalf("missing mode-specific diagnostic: %s", result)
			}
		})
	}
	config := filepath.Join(f.E.Root, "web-live-options.yaml")
	if err := os.WriteFile(config, []byte("web:\n  host: 0.0.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertWebServerOptionSourceRejected(t, f.E, []string{"web", "--config", config, "--static-dir", filepath.Join(f.E.Root, "config-rejected")}, "host")
	assertWebServerOptionSourceRejected(t, f.E.WithVar("ROTARI_WEB_AUTH_TOKEN", "secret"), []string{"web", "--static-dir", filepath.Join(f.E.Root, "env-rejected")}, "auth-token")
}

func assertWebServerOptionSourceRejected(t *testing.T, e *support.Env, args []string, option string) {
	t.Helper()
	before := support.SavePairTree(t, e.Root)
	result := pairInvoke(t, e, args...)
	if result.Code != 1 || result.Stdout != "" || !strings.Contains(result.Stderr, "--"+option+" cannot be combined with --static-dir") {
		t.Fatalf("%s from config/environment was not diagnosed for static export: %s", option, result)
	}
	if after := support.SavePairTree(t, e.Root); !reflect.DeepEqual(before.Raw(), after.Raw()) {
		t.Fatalf("rejected %s static web mode changed state", option)
	}
}

func TestCLIFlagPairWebNotificationsEffect(t *testing.T) {
	covers(t, "WEB-4")
	f := newPairFixture(t)
	output := filepath.Join(f.E.Root, "static-notification-witness")
	_, _ = invokeStaticWeb(t, f, nil, output)
	enabled, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = invokeStaticWeb(t, f, []pairFlag{{Name: "notifications"}}, output)
	disabled, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enabled, disabled) {
		t.Fatal("--notifications had no observable effect on static assets")
	}
	if !bytes.Contains(enabled, []byte("const notificationsDefaultOn = true")) || !bytes.Contains(disabled, []byte("const notificationsDefaultOn = false")) {
		t.Fatal("generated web bootstrap does not match the notifications option")
	}
}
