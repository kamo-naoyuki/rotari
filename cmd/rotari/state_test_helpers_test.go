package main

import "github.com/kamo-naoyuki/rotari/internal/state"

func defaultMeta() Meta {
	return Meta(state.DefaultMeta())
}

func loadRunSummary(path string) (RunSummary, error) {
	summary, err := state.LoadRunSummary(path)
	return RunSummary(summary), err
}

func loadQueue(path string) (Queue, error) {
	queue, err := state.LoadQueue(path)
	return Queue(queue), err
}

func writeJSON(path string, value any) error {
	return state.WriteJSON(path, value)
}
