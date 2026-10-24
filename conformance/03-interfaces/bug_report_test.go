package interfaces

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

// TestBugReportNeverIncludesSecrets runs a failing job with a secret in its
// environment, a webhook URL in notifications.toml, and a Web auth token in
// the caller's environment, then checks the Markdown summary and every file
// of the bundle, with and without --no-redact.
func TestBugReportNeverIncludesSecrets(t *testing.T) {
	covers(t, "CLI-26")
	const (
		webhook   = "https://hooks.example.com/services/conformance-webhook"
		token     = "conformance-web-token"
		jobSecret = "conformance-job-secret"
	)
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "secrets", "--env", "API_TOKEN_VALUE="+jobSecret, "--", "sh", "-c", "echo failing-output; exit 3")
	if run := e.Rotari("run", "-p", "secrets"); run.Code != 1 {
		t.Fatalf("run of a failing job: %s", run)
	}
	if err := os.WriteFile(filepath.Join(e.Base, "notifications.toml"), []byte("[webhook]\nurl = \""+webhook+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.Setenv("ROTARI_WEB_AUTH_TOKEN", token)
	secrets := []string{webhook, token, jobSecret}

	for _, variant := range []struct {
		name  string
		extra []string
	}{{name: "redacted"}, {name: "no-redact", extra: []string{"--no-redact"}}} {
		t.Run(variant.name, func(t *testing.T) {
			summary := e.MustRotari(append([]string{"bug-report", "-p", "secrets"}, variant.extra...)...).Stdout
			assertNoSecrets(t, "summary", summary, secrets)
			for _, want := range []string{"## Run ", "failing-output", "ROTARI_WEB_AUTH_TOKEN=[REDACTED]", "not checked for secrets"} {
				if !strings.Contains(summary, want) {
					t.Fatalf("summary lacks %q:\n%s", want, summary)
				}
			}
			if keeps := strings.Contains(summary, e.Base); keeps != (variant.name == "no-redact") {
				t.Fatalf("summary contains the basedir: %v, want %v", keeps, variant.name == "no-redact")
			}

			bundle := filepath.Join(e.Root, variant.name+".tar.gz")
			e.MustRotari(append([]string{"bug-report", "-p", "secrets", "--output", bundle}, variant.extra...)...)
			files := readBugReportBundle(t, bundle)
			commandFound := false
			for name, data := range files {
				assertNoSecrets(t, name, data, secrets)
				if strings.HasSuffix(name, "/command.json") && strings.Contains(data, "API_TOKEN_VALUE=[REDACTED]") {
					commandFound = true
				}
			}
			if !commandFound {
				t.Fatalf("no command.json with the job's environment name kept and value replaced in %v", bundleNames(files))
			}
			if again := e.Rotari("bug-report", "-p", "secrets", "--output", bundle); again.Code == 0 {
				t.Fatalf("bug-report replaced an existing bundle: %s", again)
			}
		})
	}
}

func assertNoSecrets(t *testing.T, name, text string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			t.Fatalf("%s contains %q", name, secret)
		}
	}
}

func readBugReportBundle(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewReader(compressed)
	files := make(map[string]string)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = string(data)
	}
}

func bundleNames(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}
