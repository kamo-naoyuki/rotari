package web

import "sort"

type JobTimelineInput struct {
	JobID       string
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

func BuildTimeline(startedAt, runFinishedAt string, jobs []JobTimelineInput) []TimelinePoint {
	type event struct {
		at                                          string
		pending, running, finished, success, failed int
	}
	events := make([]event, 0, len(jobs)*2)
	initial := TimelinePoint{At: startedAt}
	for _, job := range jobs {
		if job.Finished && job.Carried {
			initial.Finished++
			if job.Success {
				initial.Success++
			} else {
				initial.Failed++
			}
			continue
		}
		if job.Finished && job.SubmittedAt == "" && job.FinishedAt == "" && runFinishedAt == "" {
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
		if finishedAt := jobFinishedAt(job, runFinishedAt); finishedAt != "" {
			finished := event{at: finishedAt, finished: 1}
			if job.SubmittedAt != "" {
				finished.running = -1
			} else {
				finished.pending = -1
			}
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

// jobFinishedAt is when a job executed in the run finished: its own finish
// time, or the run's for a finished job that recorded none.
func jobFinishedAt(job JobTimelineInput, runFinishedAt string) string {
	if job.FinishedAt == "" && job.Finished {
		return runFinishedAt
	}
	return job.FinishedAt
}

// JobLane is one job's span on the run page's job timeline, in RFC 3339:
// from its submission to its finish, by the same rules as BuildTimeline.
// Start is empty for a job not yet submitted and End for one still
// running. A carried job ran in an earlier run, so it has no span here.
type JobLane struct {
	JobID   string `json:"job_id"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
	Carried bool   `json:"carried,omitempty"`
}

// BuildJobLanes returns one lane per job, in the jobs' order.
func BuildJobLanes(runFinishedAt string, jobs []JobTimelineInput) []JobLane {
	lanes := make([]JobLane, 0, len(jobs))
	for _, job := range jobs {
		if job.Finished && job.Carried {
			lanes = append(lanes, JobLane{JobID: job.JobID, Carried: true})
			continue
		}
		end := jobFinishedAt(job, runFinishedAt)
		if job.SubmittedAt == "" {
			// A job never submitted has no span, though BuildTimeline counts
			// its finish.
			end = ""
		}
		lanes = append(lanes, JobLane{JobID: job.JobID, Start: job.SubmittedAt, End: end})
	}
	return lanes
}
