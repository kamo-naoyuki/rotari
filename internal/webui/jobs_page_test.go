package webui

import (
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/internal/joblist"
)

// A Job activity row shows the command under the job name, as the run page
// does, and the attempt ID by its end with the whole ID kept in its text,
// title, and copy button.
func TestJobsHTMLShowsCommandUnderJobAndAttemptByItsEnd(t *testing.T) {
	attempt := "att_20261010-205222-3185eff0-c18f6264e-1-0"
	page := jobsHTML("/", nil, []joblist.Row{{State: "success", Project: "p", RunID: "r", JobName: "decode[1]", Command: "sh -c echo…", FullCommand: "sh -c echo hi", AttemptID: attempt}}, joblist.DefaultSinceText, false, false)
	for _, want := range []string{
		`decode[1]</a><div class="jobs-command"><code title="sh -c echo hi">sh -c echo…</code>`,
		`data-copy-value="sh -c echo hi"`,
		`<td class="jobs-attempt" data-sort-value="` + attempt + `"><span class="id-tail" title="` + attempt + `"><bdi>` + attempt + `</bdi></span>`,
		`data-copy-value="` + attempt + `"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("jobs page lacks %q", want)
		}
	}
	if strings.Contains(page, `data-sort="command"`) {
		t.Error("jobs page still has a separate Command column")
	}
}
