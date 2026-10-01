package notification

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAppliesDefaultsAndOverrides(t *testing.T) {
	settings, err := Parse([]byte("[webhook]\njob_success = true\nfields = []\n\n[browser]\nrun_success = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Webhook.JobFailure || !settings.Webhook.JobSuccess || len(settings.Webhook.Fields) != 0 {
		t.Fatalf("webhook settings = %#v", settings.Webhook)
	}
	if settings.Browser.RunSuccess || !settings.Browser.RunFailure {
		t.Fatalf("browser settings = %#v", settings.Browser)
	}
}

func TestParseRejectsUnknownSettingsAndFields(t *testing.T) {
	for _, data := range []string{
		"[browser]\nunknown = true\n",
		"[browser]\nfields = [\"unknown\"]\n",
		"[webhook]\nfields = [\"link\"]\n",
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Fatalf("Parse(%q) succeeded", data)
		}
	}
}

func TestLoadUsesOnlyHighestPriorityFile(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "projects", "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(configHome, "rotari", FileName), "[browser]\nmax_jobs = 1\n")
	write(filepath.Join(baseDir, FileName), "[browser]\nmax_jobs = 2\n")
	write(filepath.Join(projectDir, FileName), "[browser]\nmax_jobs = 3\n")

	loaded, err := Load(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Path != filepath.Join(projectDir, FileName) || loaded.Settings.Browser.MaxJobs != 3 {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestTemplateParses(t *testing.T) {
	if _, err := Parse(Template()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultNotificationFieldsIncludeRunAndAttemptIDs(t *testing.T) {
	settings := Defaults()
	for channel, fields := range map[string][]string{
		"webhook": settings.Webhook.Fields,
		"browser": settings.Browser.Fields,
	} {
		selected := make(map[string]bool, len(fields))
		for _, field := range fields {
			selected[field] = true
		}
		for _, field := range []string{"run_id", "attempt_id"} {
			if !selected[field] {
				t.Errorf("%s default fields do not include %q: %v", channel, field, fields)
			}
		}
	}
}

func TestMarshalRoundTripsSettings(t *testing.T) {
	want := Defaults()
	want.Webhook.URL = "https://example.invalid/hook?token=secret"
	want.Browser.JobSuccess = true
	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Webhook.URL != want.Webhook.URL || !got.Browser.JobSuccess {
		t.Fatalf("settings = %#v", got)
	}
}
