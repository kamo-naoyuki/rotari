package support

import (
	"fmt"
	"reflect"
	"testing"
)

// Process enumeration is not lifecycle order: a job can precede its
// supervisor, and separate projects can have interleaved processes.
func TestKillStrayGroupsStopsSupervisorsBeforeJobs(t *testing.T) {
	for _, test := range []struct {
		name      string
		processes []strayProcess
	}{
		{"job first", []strayProcess{{2, 2, false}, {1, 1, true}}},
		{"supervisor first", []strayProcess{{1, 1, true}, {2, 2, false}}},
		{"interleaved projects", []strayProcess{{2, 2, false}, {1, 1, true}, {4, 4, false}, {3, 3, true}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got, want []string
			for _, process := range test.processes {
				if process.supervisor {
					want = append(want, fmt.Sprintf("stop %d", process.pid))
				}
			}
			for _, process := range test.processes {
				want = append(want, fmt.Sprintf("kill group %d", process.pgid))
			}
			killStrayGroups(test.processes, func(pid int) {
				got = append(got, fmt.Sprintf("stop %d", pid))
			}, func(pgid int) {
				got = append(got, fmt.Sprintf("kill group %d", pgid))
			})
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shutdown order = %v, want %v", got, want)
			}
		})
	}
}
