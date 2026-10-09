package projectrun

import "testing"

func TestSourceNoticeNamesTheQueueAndOmittedJobs(t *testing.T) {
	for _, test := range []struct {
		omitted []string
		want    string
	}{
		{[]string{"a", "b"}, "Retry source: the current queue, not latest run run-1. These failed or unfinished jobs of that run are not in the queue: a, b\n" +
			"Include them with: rotari copy -p demo --run-id 'run-1' --failed --unfinished --append, then retry\n"},
		{nil, "Retry source: the current queue, not latest run run-1. Every failed or unfinished job of that run is in the queue.\n"},
	} {
		if got := SourceNotice("run-1", test.omitted, "-p demo"); got != test.want {
			t.Errorf("SourceNotice(%v) =\n%q\nwant\n%q", test.omitted, got, test.want)
		}
	}
}
