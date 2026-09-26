package executor

import "testing"

func TestRunSettingsOverrideDispatchDefaults(t *testing.T) {
	settings := RunSettingsMap{
		"ssh": {Concurrency: 3, Options: []string{"builder@worker-01", "-p", "2222"}},
	}

	if got := settings.Concurrency("ssh", 8); got != 3 {
		t.Fatalf("SSH concurrency = %d, want 3", got)
	}
	if got := settings.Concurrency("slurm", 8); got != 8 {
		t.Fatalf("Slurm concurrency = %d, want common default 8", got)
	}
	if got := settings.Options("ssh", []string{"common"}); len(got) != 3 || got[0] != "builder@worker-01" {
		t.Fatalf("SSH options = %#v, want executor-specific options", got)
	}
	if got := settings.Options("slurm", []string{"common"}); len(got) != 1 || got[0] != "common" {
		t.Fatalf("Slurm options = %#v, want common options", got)
	}
}
