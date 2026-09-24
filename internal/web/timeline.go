package web

import "sort"

type JobTimelineInput struct {
	Finished    bool
	Carried     bool
	SubmittedAt string
	FinishedAt  string
	Success     bool
}

type TimelinePoint struct {
	At       string `json:"at"`
	Pending  int    `json:"pending"`
	Running  int    `json:"running"`
	Finished int    `json:"finished"`
	Success  int    `json:"success"`
	Failed   int    `json:"failed"`
}

func BuildTimeline(startedAt string, jobs []JobTimelineInput) []TimelinePoint {
	type event struct {
		at                                          string
		pending, running, finished, success, failed int
	}
	events := make([]event, 0, len(jobs)*2)
	initial := TimelinePoint{At: startedAt}
	for _, job := range jobs {
		if job.Finished && (job.Carried || (job.SubmittedAt == "" && job.FinishedAt == "")) {
			initial.Finished++
			if job.Success {
				initial.Success++
			} else {
				initial.Failed++
			}
			continue
		}
		initial.Pending++
		if job.SubmittedAt != "" {
			events = append(events, event{at: job.SubmittedAt, pending: -1, running: 1})
		}
		if job.FinishedAt != "" {
			finished := event{at: job.FinishedAt, running: -1, finished: 1}
			if job.Finished && job.Success {
				finished.success = 1
			} else {
				finished.failed = 1
			}
			events = append(events, finished)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
	points := []TimelinePoint{initial}
	pending, running, finished, success, failed := initial.Pending, initial.Running, initial.Finished, initial.Success, initial.Failed
	for i := 0; i < len(events); {
		at := events[i].at
		current := event{at: at}
		for i < len(events) && events[i].at == at {
			current.pending += events[i].pending
			current.running += events[i].running
			current.finished += events[i].finished
			current.success += events[i].success
			current.failed += events[i].failed
			i++
		}
		pending += current.pending
		running += current.running
		finished += current.finished
		success += current.success
		failed += current.failed
		points = append(points, TimelinePoint{At: at, Pending: pending, Running: running, Finished: finished, Success: success, Failed: failed})
	}
	return points
}
