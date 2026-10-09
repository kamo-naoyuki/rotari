package attachment

import (
	"os"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCurrentLaunchOriginIsTheParentProcess(t *testing.T) {
	origin := CurrentLaunchOrigin()
	host, _ := os.Hostname()
	if origin.Host != host || origin.PID != os.Getppid() || origin.ProcessStart != state.ProcessStart(os.Getppid()) {
		t.Fatalf("origin = %+v, want this process's parent on %s", origin, host)
	}
}

func TestSameLaunchOrigin(t *testing.T) {
	current := model.LaunchOrigin{Host: "node1", PID: 42, ProcessStart: "100"}
	for _, test := range []struct {
		name   string
		origin *model.LaunchOrigin
		want   bool
	}{
		{"same process", &model.LaunchOrigin{Host: "node1", PID: 42, ProcessStart: "100"}, true},
		{"reused PID", &model.LaunchOrigin{Host: "node1", PID: 42, ProcessStart: "99"}, false},
		{"other process", &model.LaunchOrigin{Host: "node1", PID: 43, ProcessStart: "100"}, false},
		{"other host", &model.LaunchOrigin{Host: "node2", PID: 42, ProcessStart: "100"}, false},
		{"start unknown on one side", &model.LaunchOrigin{Host: "node1", PID: 42}, true},
		{"no recorded origin", nil, false},
	} {
		if got := SameLaunchOrigin(test.origin, current); got != test.want {
			t.Errorf("%s: SameLaunchOrigin = %t, want %t", test.name, got, test.want)
		}
	}
}
