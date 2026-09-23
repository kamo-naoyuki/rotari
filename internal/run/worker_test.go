package run

import (
	"strings"
	"testing"
)

func TestRunWorkerRunsLifecycleInOrder(t *testing.T) {
	order := []string{}
	exitCode, err := RunWorker(WorkerCallbacks{
		WriteContext: func() error { order = append(order, "context-start"); return nil },
		MarkRunning:  func() error { order = append(order, "running"); return nil },
		StartSampling: func() func() {
			order = append(order, "sampling-start")
			return func() { order = append(order, "sampling-stop") }
		},
		Execute:       func() int { order = append(order, "execute"); return 7 },
		FinishContext: func() error { order = append(order, "context-finish"); return nil },
		Finalize: func(code int) error {
			order = append(order, "finalize")
			if code != 7 {
				t.Fatalf("code = %d", code)
			}
			return nil
		},
		RemoveLock: func() error { order = append(order, "unlock"); return nil },
	})
	if err != nil || exitCode != 7 {
		t.Fatalf("RunWorker() = %d, %v", exitCode, err)
	}
	want := "context-start,running,sampling-start,execute,sampling-stop,context-finish,finalize,unlock"
	got := strings.Join(order, ",")
	if got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
}
