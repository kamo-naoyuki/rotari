package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func exampleWorkflowScript(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../.github/workflows/examples.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string
				Run  string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, step := range workflow.Jobs["examples"].Steps {
		if step.Name == name && step.Run != "" {
			return step.Run
		}
	}
	t.Fatalf("example step %q not found", name)
	return ""
}

// Run the actual local workflow against the built binary so its output checks
// track CLI presentation, including carried successes after retry.
func TestLocalExampleWorkflow(t *testing.T) {
	workspace, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "rotari"), "./cmd/rotari")
	build.Dir = workspace
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build rotari: %v\n%s", err, output)
	}
	cmd := exec.Command("bash", "-c", exampleWorkflowScript(t, "Run local examples"))
	cmd.Env = append(os.Environ(),
		"GITHUB_WORKSPACE="+workspace, "RUNNER_TEMP="+dir,
		"ROTARI_BASEDIR="+filepath.Join(dir, "state"),
		"ROTARI_MASTERDIR="+filepath.Join(dir, "master"),
		"XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
		"XDG_STATE_HOME="+filepath.Join(dir, "xdg-state"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("local example workflow: %v\n%s", err, output)
	}
}

// Exercise the workflow's shell rather than a copy of its docker exec command.
// Scheduler startup is mocked; this test checks the non-root client's state
// directories and current show output, not Slurm execution (which requires the
// container integration).
func TestSlurmExampleStateDirectories(t *testing.T) {
	script := exampleWorkflowScript(t, "Run Slurm example in a container")
	const mockDocker = `
docker() {
    if [[ "$1" != exec ]]; then
        return 0
    fi
    shift
    local user="" workdir="" basedir="" masterdir=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --user) user="$2"; shift 2 ;;
            --workdir) workdir="$2"; shift 2 ;;
            --env)
                case "$2" in
                    ROTARI_BASEDIR=*) basedir="${2#*=}" ;;
                    ROTARI_MASTERDIR=*) masterdir="${2#*=}" ;;
                esac
                shift 2
                ;;
            *) shift ;;
        esac
    done
    if [[ "$user" == slurm ]]; then
        if [[ "$workdir" != /state || "$basedir" != /state || "$masterdir" != /state/master ]]; then
            echo "Slurm example must keep both state directories on the writable /state volume" >&2
            return 1
        fi
		echo "Job status: success: 2, failed: 0, blocked: 0, cancelled: 0"
    fi
}
`
	cmd := exec.Command("bash", "-c", mockDocker+script)
	cmd.Env = append(os.Environ(),
		"GITHUB_RUN_ID=test", "GITHUB_RUN_ATTEMPT=1",
		"GITHUB_WORKSPACE=/workspace", "RUNNER_TEMP=/runner-temp")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Slurm example workflow: %v\n%s", err, output)
	}
}
