package projectrun

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ErrPlanChanged refuses a run whose plan differs from the preview whose
// revision it was given.
var ErrPlanChanged = errors.New("this run would execute other jobs than its preview")

// planRevisionSeparator joins the project's revision and the plan's hash in
// a run preview's revision.
const planRevisionSeparator = "."

// PlanRevision is the revision a run preview reports: the project's revision
// and, after a dot, a hash of the jobs the plan executes and carries and the
// run it builds from. A run given this revision starts only while both
// still hold, so a start whose options select other jobs than its preview
// fails instead of running them.
func PlanRevision(projectRevision string, planned PlannedRun) string {
	executed := make([]string, 0, len(planned.Plan.Execute))
	for id, execute := range planned.Plan.Execute {
		if execute {
			executed = append(executed, id)
		}
	}
	carried := make([]string, 0, len(planned.Plan.CarriedResults))
	for id := range planned.Plan.CarriedResults {
		carried = append(carried, id)
	}
	sort.Strings(executed)
	sort.Strings(carried)
	hash := sha256.New()
	fmt.Fprintf(hash, "source %s\nexecute %s\ncarry %s\n", planned.SourceRunID, strings.Join(executed, " "), strings.Join(carried, " "))
	return projectRevision + planRevisionSeparator + hex.EncodeToString(hash.Sum(nil))[:8]
}

// projectPart returns the project's revision within a run's ifRevision: all
// of a revision `check` reports, or the part before the plan's hash.
func projectPart(ifRevision string) string {
	revision, _, _ := strings.Cut(ifRevision, planRevisionSeparator)
	return revision
}

// CheckRunPlanRevision refuses planned when ifRevision is a preview's revision
// with another plan. A revision without a plan, such as `check` reports,
// guards only the project's state, which project.CheckRevision checked.
func CheckRunPlanRevision(ifRevision, projectRevision string, planned PlannedRun) error {
	if !strings.Contains(ifRevision, planRevisionSeparator) {
		return nil
	}
	if PlanRevision(projectRevision, planned) == ifRevision {
		return nil
	}
	executed := make([]string, 0, len(planned.Plan.Execute))
	for _, job := range model.QueueToJobs(planned.Queue.Commands) {
		if planned.Plan.Execute[job.ID] {
			label := job.ID
			if job.Name != "" {
				label += " " + job.Name
			}
			executed = append(executed, label)
		}
	}
	listed := executed
	if len(listed) > 10 {
		listed = listed[:10]
	}
	return fmt.Errorf("%w: it would execute %d job(s) (%s); preview it again with the same options and use that revision",
		ErrPlanChanged, len(executed), strings.Join(listed, ", "))
}

// CheckRunProjectRevision checks the project part of a run's ifRevision
// under the state lock, before the run is planned, and returns the project's
// revision; CheckRunPlanRevision checks the plan part after planning.
func CheckRunProjectRevision(paths state.ProjectPaths, ifRevision string) (string, error) {
	return project.CheckRevision(paths, project.Guard{IfRevision: projectPart(ifRevision)})
}
