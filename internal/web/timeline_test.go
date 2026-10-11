package web

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTimelinePointUsesBrowserJSONKeys(t *testing.T) {
	data, err := json.Marshal(TimelinePoint{At: "start", Pending: 1, Running: 2, Finished: 3, Success: 4, Failed: 5})
	if err != nil {
		t.Fatal(err)
	}
	var point map[string]any
	if err := json.Unmarshal(data, &point); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"at", "pending", "running", "finished", "success", "failed"} {
		if _, ok := point[key]; !ok {
			t.Fatalf("JSON point does not contain %q: %s", key, data)
		}
	}
}

func TestBuildTimelineCountsCarriedJobsAtStart(t *testing.T) {
	points := BuildTimeline("start", "30", []JobTimelineInput{
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
	points := BuildTimeline("start", "", []JobTimelineInput{
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

func TestBuildTimelineUsesRunEndWhenFinishedJobHasNoTimestamps(t *testing.T) {
	points := BuildTimeline("start", "finish", []JobTimelineInput{
		{Finished: true, Success: true},
	})
	if len(points) != 2 {
		t.Fatalf("points = %#v, want start and run-finish points", points)
	}
	if points[0].At != "start" || points[0].Pending != 1 || points[0].Finished != 0 {
		t.Fatalf("initial point = %#v, want one pending job", points[0])
	}
	if points[1].At != "finish" || points[1].Pending != 0 || points[1].Finished != 1 || points[1].Success != 1 {
		t.Fatalf("finish point = %#v, want one finished success", points[1])
	}
}

// Each job gets a lane in job order: an executed job spans its submission to
// its finish (the run's finish when it recorded none), a running job has no
// end, a job never submitted no span, and a carried job is marked instead.
func TestBuildJobLanes(t *testing.T) {
	lanes := BuildJobLanes("run-end", []JobTimelineInput{
		{JobID: "done", Finished: true, SubmittedAt: "10", FinishedAt: "20", Success: true},
		{JobID: "running", SubmittedAt: "11"},
		{JobID: "pending"},
		{JobID: "carried", Finished: true, Carried: true, Success: true},
		{JobID: "no-finish-time", Finished: true, SubmittedAt: "12"},
		{JobID: "never-submitted", Finished: true},
	})
	want := []JobLane{
		{JobID: "done", Start: "10", End: "20"},
		{JobID: "running", Start: "11"},
		{JobID: "pending"},
		{JobID: "carried", Carried: true},
		{JobID: "no-finish-time", Start: "12", End: "run-end"},
		{JobID: "never-submitted"},
	}
	if !reflect.DeepEqual(lanes, want) {
		t.Fatalf("lanes = %#v, want %#v", lanes, want)
	}
}
