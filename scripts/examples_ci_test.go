package scripts_test

import (
	"os"
	"os/exec"
	"testing"

	"gopkg.in/yaml.v3"
)

// Exercise the workflow's shell rather than a copy of its docker exec command.
// Scheduler startup is mocked; this test checks the non-root client's state
// directories, not Slurm execution (which requires the container integration).
func TestSlurmExampleStateDirectories(t *testing.T) {
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
	var script string
	for _, step := range workflow.Jobs["examples"].Steps {
		if step.Name == "Run Slurm example in a container" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("Slurm example step not found")
	}
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
        echo "Success: 2"
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
