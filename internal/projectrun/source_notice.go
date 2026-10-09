package projectrun

import (
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
)

// SourceNotice tells a run or retry preview that it uses the non-empty
// queue instead of the latest run sourceRunID, naming that run's failed or
// unfinished jobs the queue leaves out and the copy command that adds them.
// location is the --basedir/--project-name part of the command, as the
// caller can render it.
func SourceNotice(sourceRunID string, omitted []string, location string) string {
	notice := fmt.Sprintf("Retry source: the current queue, not latest run %s.", sourceRunID)
	if len(omitted) == 0 {
		return notice + " Every failed or unfinished job of that run is in the queue.\n"
	}
	return fmt.Sprintf("%s These failed or unfinished jobs of that run are not in the queue: %s\nInclude them with: rotari copy %s --run-id %s --failed --unfinished --append, then retry\n",
		notice, strings.Join(omitted, ", "), location, executor.ShellQuote(sourceRunID))
}
