package web

import "testing"

func TestBuildTimelineCountsCarriedJobsAtStart(t *testing.T) {
	points := BuildTimeline("start", []JobTimelineInput{
		{Finished: true, Carried: true, Success: true},
		{SubmittedAt: "10", FinishedAt: "20", Finished: true, Success: false},
	})
	if len(points) != 3 {
		t.Fatalf("points = %#v, want initial plus two events", points)
	}
	if points[0].Finished != 1 || points[0].Success != 1 || points[0].Pending != 1 {
		t.Fatalf("initial = %#v, want carried finished and pending job", points[0])
	}
	if points[1].Running != 1 || points[1].Pending != 0 {
		t.Fatalf("submitted point = %#v", points[1])
	}
	if points[2].Finished != 2 || points[2].Failed != 1 || points[2].Running != 0 {
		t.Fatalf("finished point = %#v", points[2])
	}
}

func TestBuildTimelineCombinesEventsAtSameTime(t *testing.T) {
	points := BuildTimeline("start", []JobTimelineInput{
		{SubmittedAt: "10", FinishedAt: "20", Finished: true, Success: true},
		{SubmittedAt: "10", FinishedAt: "20", Finished: true, Success: false},
	})
	if len(points) != 3 {
		t.Fatalf("points = %#v, want one point per timestamp", points)
	}
	if points[1].Running != 2 || points[1].Pending != 0 {
		t.Fatalf("same-time submission = %#v", points[1])
	}
	if points[2].Running != 0 || points[2].Finished != 2 || points[2].Success != 1 || points[2].Failed != 1 {
		t.Fatalf("same-time completion = %#v", points[2])
	}
}
