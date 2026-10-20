package artifact

import (
	"errors"
	"testing"
)

// TestLocalURIsFromEverySource gives every discovery source the same local
// URIs, as experiment trackers take them, and expects the same candidates:
// the path each names, resolved against the working directory, with the
// URI kept as the source's value. Remote URIs stay skipped everywhere.
func TestLocalURIsFromEverySource(t *testing.T) {
	const (
		sqlite   = "sqlite:///runs/mlflow.db"
		file     = "file:./mlruns"
		absolute = "file:///data/aim"
		remote   = "http://127.0.0.1:5000"
	)
	want := map[string]string{
		"/work/runs/mlflow.db": sqlite,
		"/work/mlruns":         file,
		"/data/aim":            absolute,
	}
	files := map[string]string{
		"/work/train.yaml": "tracking_uri: " + sqlite + "\nstore: " + file + "\nrepo: " + absolute + "\nserver: " + remote + "\n",
		"/work/train.toml": "tracking_uri = \"" + sqlite + "\"\nstore = \"" + file + "\"\nrepo = \"" + absolute + "\"\nserver = \"" + remote + "\"\n",
		"/work/train.json": `{"tracking_uri": "` + sqlite + `", "store": "` + file + `", "repo": "` + absolute + `", "server": "` + remote + `"}`,
		"/work/train.py": "import mlflow\nmlflow.set_tracking_uri(\"" + sqlite + "\")\nSTORE = \"" + file + "\"\nREPO = \"" + absolute +
			"\"\nSERVER = \"" + remote + "\"\n",
	}
	reader := func(path string) ([]byte, error) {
		if content, ok := files[path]; ok {
			return []byte(content), nil
		}
		return nil, errors.New("no such file")
	}
	sources := Sources{Config: ParseSources(reader), Script: reader}
	for name, test := range map[string]struct {
		job  Job
		kind string
	}{
		"argv": {Job{Command: []string{"train", "--tracking-uri", sqlite, "--store", file, "--repo", absolute, "--server", remote}}, KindArgument},
		"environment": {Job{Command: []string{"train"}, Environment: []string{
			"MLFLOW_TRACKING_URI=" + sqlite, "STORE=" + file, "AIM_REPO=" + absolute, "SERVER=" + remote}}, KindEnvironment},
		"shell": {Job{Command: []string{"sh", "-c", "MLFLOW_TRACKING_URI=" + sqlite + " STORE=" + file + " AIM_REPO=" + absolute +
			" SERVER=" + remote + " train"}}, KindShell},
		"yaml":   {Job{Command: []string{"train", "--config", "train.yaml"}}, KindConfig},
		"toml":   {Job{Command: []string{"train", "--config", "train.toml"}}, KindConfig},
		"json":   {Job{Command: []string{"train", "--config", "train.json"}}, KindConfig},
		"python": {Job{Command: []string{"python", "train.py"}}, KindPython},
	} {
		t.Run(name, func(t *testing.T) {
			test.job.WorkingDirectory = "/work"
			result := Discover(test.job, sources)
			got := map[string]string{}
			for _, candidate := range result.Candidates {
				for _, source := range candidate.Sources {
					if source.Kind != test.kind {
						continue
					}
					if source.Rule == RuleLocalURI {
						got[candidate.Path] = source.Value
					}
					if source.Value == remote {
						t.Errorf("remote URI %s became candidate %s", remote, candidate.Path)
					}
				}
			}
			if len(got) != len(want) {
				t.Fatalf("local URI candidates = %v, want %v", got, want)
			}
			for path, value := range want {
				if got[path] != value {
					t.Errorf("candidate %s from %q, want from %q (all: %v)", path, got[path], value, got)
				}
			}
		})
	}
}
