package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/launcher"
)

// The launch wrapper runs before configuration is loaded, so a broken
// workspace config cannot stop a launched command's wrapper. Without a
// command it only reports its usage; its behavior is tested in
// internal/launcher.
func TestLaunchWrapperRunsBeforeConfiguration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".rotari.toml"), []byte("not toml ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	code, stderr := captureStderr(t, func() int { return run([]string{launcher.WrapperCommand}) })
	if code != 2 || !strings.Contains(stderr, "rotari "+launcher.WrapperCommand+": usage:") || strings.Contains(stderr, "config") {
		t.Fatalf("run(%s) = %d, stderr %q", launcher.WrapperCommand, code, stderr)
	}
}

func TestWebLaunchFlagsRejectConflicts(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--allow-exec", "--allow-control=false"}, "--allow-exec requires job control"},
		{[]string{"--allowed-host", "gpu01"}, "--allowed-host only applies with --allow-exec"},
		{[]string{"--allow-exec", "--allowed-host", "gpu01:8765"}, "without a port"},
		{[]string{"--allow-exec", "--allowed-host", "a/b"}, "host name or IP address"},
		{[]string{"--allow-exec", "--static-dir", t.TempDir()}, "--allow-exec cannot be combined with --static-dir"},
		{[]string{"--allowed-host", "gpu01", "--static-dir", t.TempDir()}, "--allowed-host cannot be combined with --static-dir"},
	} {
		code, stderr := captureStderr(t, func() int {
			_, code := parseWebFlags(test.args)
			return code
		})
		if code == 0 || !strings.Contains(stderr, test.want) {
			t.Errorf("web %v = %d %q, want an error containing %q", test.args, code, stderr, test.want)
		}
	}
	flags, code := parseWebFlags([]string{"--allow-exec", "--allowed-host", "gpu01.example", "--allowed-host", "[fe80::1]"})
	if code != 0 || !flags.allowExec || len(flags.allowedHosts) != 2 {
		t.Fatalf("parseWebFlags = %+v, %d", flags, code)
	}
}

func TestExecAllowedHosts(t *testing.T) {
	for _, test := range []struct {
		host string
		want []string
	}{
		{"127.0.0.1", []string{"x", "127.0.0.1"}},
		{"gpu01.example", []string{"x", "gpu01.example"}},
		{"10.0.0.5", []string{"x", "10.0.0.5"}},
		// A wildcard listen address names no host a browser can use.
		{"0.0.0.0", []string{"x"}},
		{"::", []string{"x"}},
	} {
		got := execAllowedHosts(webCommandFlags{host: test.host, allowedHosts: []string{"x"}})
		if strings.Join(got, ",") != strings.Join(test.want, ",") {
			t.Errorf("execAllowedHosts(--host %s) = %v, want %v", test.host, got, test.want)
		}
	}
}

func TestLaunchBaseEnvironmentDropsWebToken(t *testing.T) {
	got := launchBaseEnvironment([]string{"PATH=/bin", envWebAuthToken + "=secret", "ROTARI_WEB_AUTH_TOKENS=kept"})
	if strings.Join(got, " ") != "PATH=/bin ROTARI_WEB_AUTH_TOKENS=kept" {
		t.Fatalf("launchBaseEnvironment = %v", got)
	}
}

func TestConfigLaunchersWritesTrustedGlobalFile(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	path := filepath.Join(configHome, "rotari", launcher.FileName)

	code, output := captureCommandOutput(func() int { return run([]string{"config", "--launchers"}) })
	if code != 0 || !strings.Contains(output, "Wrote "+path) {
		t.Fatalf("config --launchers = %d %q", code, output)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("launcher file mode = %v, %v; want 0600", info, err)
	}
	if dir, err := os.Stat(filepath.Dir(path)); err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("config directory = %v, %v; want 0700", dir, err)
	}
	registry, err := launcher.LoadRegistry(path)
	if err != nil {
		t.Fatalf("the written template is not accepted: %v", err)
	}
	if _, ok := registry.Lookup("TensorBoard"); !ok {
		t.Fatal("the written template has no TensorBoard entry")
	}

	if code, output := captureCommandOutput(func() int { return run([]string{"config", "--launchers"}) }); code == 0 || !strings.Contains(output, "refusing to overwrite") {
		t.Fatalf("second config --launchers = %d %q", code, output)
	}
	listed := captureConfigListOutput(t, []string{"--basedir", t.TempDir()})
	if !strings.Contains(listed, path) {
		t.Fatalf("config --list does not show %s:\n%s", path, listed)
	}
	// A launchers.toml in a basedir is never read, so it is not listed.
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, launcher.FileName), launcher.Template(), 0o600); err != nil {
		t.Fatal(err)
	}
	if listed := captureConfigListOutput(t, []string{"--basedir", baseDir}); strings.Contains(listed, filepath.Join(baseDir, launcher.FileName)) {
		t.Fatalf("config --list shows a basedir launchers.toml:\n%s", listed)
	}

	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--list"}, "cannot be combined with --list"},
		{[]string{"--notifications"}, "cannot be combined with --notifications"},
		{[]string{"--basedir", t.TempDir()}, "cannot be combined with --basedir"},
		{[]string{"--format", "yaml"}, "only supports TOML"},
		{[]string{"--output", filepath.Join(t.TempDir(), "l.yaml")}, ".toml extension"},
	} {
		code, output := captureCommandOutput(func() int { return run(append([]string{"config", "--launchers"}, test.args...)) })
		if code == 0 || !strings.Contains(output, test.want) {
			t.Errorf("config --launchers %v = %d %q, want %q", test.args, code, output, test.want)
		}
	}
	other := filepath.Join(t.TempDir(), "copy.toml")
	if code, output := captureCommandOutput(func() int { return run([]string{"config", "--launchers", "--output", other}) }); code != 0 || !strings.Contains(output, "reads launchers only from "+path) {
		t.Fatalf("config --launchers --output = %d %q", code, output)
	}
	if code, output := captureCommandOutput(func() int { return run([]string{"config", "--launchers", "--output", "-"}) }); code != 0 || !strings.Contains(output, "[[launcher]]") {
		t.Fatalf("config --launchers --output - = %d %q", code, output)
	}
}
