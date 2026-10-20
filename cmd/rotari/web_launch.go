package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/launcher"
	"github.com/kamo-naoyuki/rotari/internal/webui"
)

// webLauncherFile is the only launchers.toml rotari reads: the one in the
// global config directory, never a workspace, basedir, or project copy.
func webLauncherFile() (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, launcher.FileName), nil
}

// newWebLaunches loads the launcher file and starts the manager whose
// launches run under this executable's __launch wrapper.
func newWebLaunches() (*webui.LaunchOptions, error) {
	path, err := webLauncherFile()
	if err != nil {
		return nil, fmt.Errorf("locate %s: %w", launcher.FileName, err)
	}
	registry, err := launcher.LoadRegistry(path)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate the rotari executable: %w", err)
	}
	manager, err := launcher.NewManager(launcher.ManagerConfig{
		Wrapper:     []string{executable, launcher.WrapperCommand},
		Env:         launchBaseEnvironment(os.Environ()),
		ServicePath: webui.LaunchProxyPath,
	})
	if err != nil {
		return nil, err
	}
	return &webui.LaunchOptions{Manager: manager, Registry: registry}, nil
}

// launchBaseEnvironment is the Web server's environment without its own
// token, which a launched command has no use for.
func launchBaseEnvironment(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, envWebAuthToken+"=") {
			result = append(result, entry)
		}
	}
	return result
}

func newWebAuthToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func printLaunchRegistry(registry launcher.Registry) {
	names := make([]string, 0, len(registry.Commands))
	for _, command := range registry.Commands {
		name := command.Name
		if command.Builtin {
			name += " (built in)"
		}
		if reason := command.Unavailable(); reason != "" {
			name += " (unavailable: " + reason + ")"
		}
		names = append(names, name)
	}
	fmt.Fprintf(os.Stderr, "Launching is enabled with %d command(s): %s\n", len(names), strings.Join(names, ", "))
	if registry.Missing {
		fmt.Fprintf(os.Stderr, "Register your own in %s; `rotari config --launchers` writes a template.\n", registry.Path)
	}
}

// execAllowedHosts are the Host names accepted besides loopback names: the
// listen host when it names one specific address, and --allowed-host.
func execAllowedHosts(flags webCommandFlags) []string {
	hosts := append([]string{}, flags.allowedHosts...)
	if ip := net.ParseIP(strings.Trim(flags.host, "[]")); ip == nil || !ip.IsUnspecified() {
		hosts = append(hosts, flags.host)
	}
	return hosts
}

// validAllowedHost accepts a host name or an IP address, without a port.
func validAllowedHost(name string) bool {
	trimmed := strings.Trim(name, "[]")
	if net.ParseIP(trimmed) != nil {
		return true
	}
	if trimmed == "" || len(trimmed) > 253 || trimmed != name {
		return false
	}
	for _, label := range strings.Split(trimmed, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// writeLauncherTemplate is `rotari config --launchers`: it writes the
// launcher template to the global launchers.toml, or to --output. The file
// is created private (0600, directory 0700) so it passes the trust check.
func writeLauncherTemplate(fs *flag.FlagSet, list, notifications bool, format, output string) int {
	for _, conflict := range []struct {
		set  bool
		name string
	}{{list, "--list"}, {notifications, "--notifications"}, {cliOptionSet(fs, "basedir"), "--basedir"}, {cliOptionSet(fs, "project-name"), "--project-name"}} {
		if conflict.set {
			printErrorf("--launchers cannot be combined with %s; rotari reads launchers.toml only from the global config directory", conflict.name)
			return 1
		}
	}
	if format != "" && format != "toml" {
		printError("launcher configuration only supports TOML")
		return 1
	}
	global, err := webLauncherFile()
	if err != nil {
		printErrorf("failed to locate the global config directory: %v", err)
		return 1
	}
	if output == "-" {
		_, _ = os.Stdout.Write(launcher.Template())
		return 0
	}
	if output == "" {
		output = global
	} else if filepath.Ext(output) != ".toml" {
		printError("launcher configuration output must use the .toml extension")
		return 1
	}
	if _, err := os.Lstat(output); err == nil {
		printErrorf("refusing to overwrite existing launcher file %s", output)
		return 1
	} else if !errors.Is(err, os.ErrNotExist) {
		printErrorf("failed to inspect launcher file %s: %v", output, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		printErrorf("failed to create directory for %s: %v", output, err)
		return 1
	}
	if err := os.WriteFile(output, launcher.Template(), 0o600); err != nil {
		printErrorf("failed to write launcher file %s: %v", output, err)
		return 1
	}
	absolute, _ := filepath.Abs(output)
	if absolute != global {
		fmt.Fprintf(os.Stderr, "Wrote %s. rotari web reads launchers only from %s.\n", output, global)
		return 0
	}
	if _, err := launcher.LoadRegistry(global); err != nil {
		printWarningf("wrote %s, but rotari web --allow-exec will refuse it: %v", global, err)
		return 0
	}
	fmt.Fprintf(os.Stderr, "Wrote %s. Edit it, then start rotari web --allow-exec.\n", global)
	return 0
}

// withLauncherFile adds the global launchers.toml, when it exists, after
// the other global config files.
func withLauncherFile(common []string) []string {
	path, err := webLauncherFile()
	if err != nil {
		return common
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return common
	}
	globalDir := filepath.Dir(path) + string(filepath.Separator)
	insert := 0
	for insert < len(common) && strings.HasPrefix(common[insert], globalDir) {
		insert++
	}
	return slices.Insert(common, insert, path)
}
