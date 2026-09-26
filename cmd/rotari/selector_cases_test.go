package main

// Known deviations, named as in ISSUES.md.
const (
	knownArrayLookup  = "Array jobs are looked up per task"
	knownRetryJobID   = "retry --job-id does not also execute the named job"
	knownAttemptEdits = "Queue edits report an attempt ID as \"job not found\""
)

// selectorCases follows the tables of docs/contracts/06-selectors.md. See
// newSelectorFixture for the keys. Project sweep's latest run is
// sweep-second, which re-executed the failed train-SEED2 and eval-2.
var selectorCases = []selectorCase{
	// show: which run and job a view displays.
	{name: "job ID", cmd: "show", args: "-b {B} -p sweep --job-id {job:prep}", jobs: []string{"prep"}},
	{name: "job ID in any project", cmd: "show", args: "-b {B} --job-id {job:other-prep}", jobs: []string{"other-prep"}},
	{name: "job name", cmd: "show", args: "-b {B} -p sweep --job-name prep", jobs: []string{"prep"}},
	{name: "job name shared by projects", cmd: "show", args: "-b {B} --job-name prep", err: "ambiguous"},
	{name: "job ID and name", cmd: "show", args: "-b {B} -p sweep --job-id {job:prep} --job-name prep", err: "cannot be combined"},
	{name: "array command ID", cmd: "show", args: "-b {B} -p sweep --job-id {job:eval}", jobs: []string{"eval-1", "eval-2", "eval-3"}, known: knownArrayLookup},
	{name: "array job name", cmd: "show", args: "-b {B} -p sweep --job-name eval", jobs: []string{"eval-1", "eval-2", "eval-3"}, known: knownArrayLookup},
	{name: "array task ID", cmd: "show", args: "-b {B} -p sweep --job-id {job:eval}-2", jobs: []string{"eval-2"}},
	{name: "array task name", cmd: "show", args: "-b {B} -p sweep --job-name eval[2]", jobs: []string{"eval-2"}},
	{name: "attempt ID", cmd: "show", args: "-b {B} --job-id {att:train-SEED2/0}", jobs: []string{"train-SEED2"}, run: "sweep-first"},
	{name: "attempt ID positional", cmd: "show", args: "{att:train-SEED2/0}", jobs: []string{"train-SEED2"}, run: "sweep-first"},
	{name: "attempt ID of another run", cmd: "show", args: "-b {B} --run-id {run:sweep-second} --job-id {att:train-SEED2/0}", err: "belongs to run"},
	{name: "run ID positional through registry", cmd: "show", args: "{run:remote-run}", run: "remote-run"},
	{name: "run ID through registry", cmd: "show", args: "--run-id {run:remote-run}", run: "remote-run"},
	{name: "run ID under another basedir", cmd: "show", args: "-b {B} --run-id {run:remote-run}", err: "registered under basedir"},
	{name: "run ID under another project", cmd: "show", args: "-b {B} -p other --run-id {run:sweep-first}", err: "registered under project"},
	{name: "missing run ID", cmd: "show", args: "-b {B} -p sweep --run-id 20990101-000000-deadbeef", err: "not found"},
	{name: "latest run", cmd: "show", args: "-b {B} -p sweep --run-id latest", run: "sweep-second"},
	{name: "latest run without project", cmd: "show", args: "-b {B} --run-id latest", err: "multiple projects"},
	{name: "job in a given run", cmd: "show", args: "-b {B} -p sweep --run-id {run:sweep-first} --job-name train-SEED2", jobs: []string{"train-SEED2"}, run: "sweep-first"},
	{name: "run name", cmd: "show", args: "-b {B} second", run: "sweep-second"},
	{name: "run name shared by projects", cmd: "show", args: "-b {B} first", err: "ambiguous"},
	{name: "run name in a project", cmd: "show", args: "-b {B} -p sweep first", run: "sweep-first"},
	{name: "job name positional", cmd: "show", args: "-b {B} -p sweep prep", jobs: []string{"prep"}},
	{name: "job ID positional", cmd: "show", args: "-b {B} {job:prep}", jobs: []string{"prep"}},

	// copy: which commands are restored, and from which run.
	{name: "latest run", cmd: "copy", args: "-b {B} -p sweep", jobs: []string{"eval", "prep", "report", "train-SEED1", "train-SEED2"}},
	{name: "failed", cmd: "copy", args: "-b {B} -p sweep --failed", jobs: []string{"eval", "train-SEED2"}},
	{name: "given run", cmd: "copy", args: "-b {B} -p sweep --run-id {run:sweep-first} --failed", jobs: []string{"eval", "train-SEED2"}, run: "sweep-first"},
	{name: "run ID positional through registry", cmd: "copy", args: "{run:remote-run}", jobs: []string{"remote:solo"}, run: "remote-run"},
	{name: "job ID", cmd: "copy", args: "-b {B} -p sweep --job-id {job:prep}", jobs: []string{"prep"}},
	{name: "job ID in any project", cmd: "copy", args: "-b {B} --job-id {job:other-prep}", jobs: []string{"other:other-prep"}},
	{name: "job name", cmd: "copy", args: "-b {B} -p sweep --job-name prep", jobs: []string{"prep"}},
	{name: "job name shared by projects", cmd: "copy", args: "-b {B} --job-name prep", err: "ambiguous"},
	{name: "array command ID", cmd: "copy", args: "-b {B} -p sweep --job-id {job:eval}", jobs: []string{"eval"}, known: knownArrayLookup},
	{name: "array job name", cmd: "copy", args: "-b {B} -p sweep --job-name eval", jobs: []string{"eval"}, known: knownArrayLookup},
	{name: "array task ID", cmd: "copy", args: "-b {B} -p sweep --job-id {job:eval}-2", jobs: []string{"eval-2"}, known: knownArrayLookup},
	{name: "array task name", cmd: "copy", args: "-b {B} -p sweep --job-name eval[2]", jobs: []string{"eval-2"}, known: knownArrayLookup},
	{name: "attempt ID", cmd: "copy", args: "-b {B} --job-id {att:train-SEED2/0}", jobs: []string{"train-SEED2"}, run: "sweep-first"},
	{name: "attempt ID of another run", cmd: "copy", args: "-b {B} -p sweep --run-id {run:sweep-first} --job-id {att:train-SEED2/second}", err: "belongs to run"},
	{name: "stage", cmd: "copy", args: "-b {B} -p sweep --stage evaluation", jobs: []string{"eval"}},
	{name: "stage and failed", cmd: "copy", args: "-b {B} -p sweep --stage training --failed", jobs: []string{"train-SEED2"}},
	{name: "matrix", cmd: "copy", args: "-b {B} -p sweep --matrix train", jobs: []string{"train-SEED1", "train-SEED2"}},
	{name: "unknown stage", cmd: "copy", args: "-b {B} -p sweep --stage nope", err: `no jobs in stage "nope"`},
	{name: "stage without project", cmd: "copy", args: "-b {B} --stage training", err: "multiple projects"},
	{name: "stage and job ID", cmd: "copy", args: "-b {B} -p sweep --stage training --job-id {job:prep}", err: "cannot be combined"},

	// change: which commands get the new setting.
	{name: "empty queue", cmd: "change", args: "-b {B} -p sweep --job-name prep", err: "no queued jobs"},
	{name: "job name", cmd: "change", args: "-b {B} -p sweep --job-name prep", queued: true, jobs: []string{"prep"}},
	{name: "job ID", cmd: "change", args: "-b {B} -p sweep --job-id {job:prep}", queued: true, jobs: []string{"prep"}},
	{name: "given run", cmd: "change", args: "-b {B} -p sweep --run-id {run:sweep-first} --job-name prep", jobs: []string{"prep"}},
	{name: "run through registry", cmd: "change", args: "--run-id {run:remote-run} --all", jobs: []string{"remote:solo"}},
	{name: "stage of latest run", cmd: "change", args: "-b {B} -p sweep --run-id latest --stage training", jobs: []string{"train-SEED1", "train-SEED2"}},
	{name: "matrix", cmd: "change", args: "-b {B} -p sweep --matrix train", queued: true, jobs: []string{"train-SEED1", "train-SEED2"}},
	{name: "all", cmd: "change", args: "-b {B} -p sweep --all", queued: true, jobs: []string{"eval", "prep", "report", "train-SEED1", "train-SEED2"}},
	{name: "array command ID", cmd: "change", args: "-b {B} -p sweep --job-id {job:eval}", queued: true, jobs: []string{"eval"}},
	{name: "array job name", cmd: "change", args: "-b {B} -p sweep --job-name eval", queued: true, jobs: []string{"eval"}},
	{name: "array task ID", cmd: "change", args: "-b {B} -p sweep --job-id {job:eval}-2", queued: true, err: "task of array job {job:eval}"},
	{name: "array task name", cmd: "change", args: "-b {B} -p sweep --job-name eval[2]", queued: true, err: "task of array job {job:eval}"},
	{name: "attempt ID", cmd: "change", args: "-b {B} -p sweep --job-id {att:train-SEED2/0}", queued: true, err: "attempt ID", known: knownAttemptEdits},
	{name: "unknown matrix", cmd: "change", args: "-b {B} -p sweep --matrix nope", queued: true, err: `no matrix named "nope"`},
	{name: "two group selectors", cmd: "change", args: "-b {B} -p sweep --all --stage training", queued: true, err: "usage"},
	{name: "new command for a group", cmd: "change", args: "-b {B} -p sweep --stage training -- echo", queued: true, err: "single job"},

	// remove: which commands are removed.
	{name: "empty queue", cmd: "remove", args: "-b {B} -p sweep --job-name prep", err: "no queued jobs"},
	{name: "job name", cmd: "remove", args: "-b {B} -p sweep --job-name prep", queued: true, jobs: []string{"prep"}},
	{name: "job IDs", cmd: "remove", args: "-b {B} -p sweep -j {job:prep} -j {job:report}", queued: true, jobs: []string{"prep", "report"}},
	{name: "job IDs positional", cmd: "remove", args: "-b {B} -p sweep {job:prep} {job:report}", queued: true, jobs: []string{"prep", "report"}},
	{name: "job IDs both ways", cmd: "remove", args: "-b {B} -p sweep -j {job:prep} {job:report}", queued: true, err: "usage"},
	{name: "one job ID missing", cmd: "remove", args: "-b {B} -p sweep -j {job:prep} -j nope", queued: true, err: "one or more jobs not found: nope"},
	{name: "stage", cmd: "remove", args: "-b {B} -p sweep --stage evaluation", queued: true, jobs: []string{"eval"}},
	{name: "matrix of latest run", cmd: "remove", args: "-b {B} -p sweep --run-id latest --matrix train", jobs: []string{"train-SEED1", "train-SEED2"}},
	{name: "all", cmd: "remove", args: "-b {B} -p sweep --all", queued: true, jobs: []string{"eval", "prep", "report", "train-SEED1", "train-SEED2"}},
	{name: "array task ID", cmd: "remove", args: "-b {B} -p sweep -j {job:eval}-2", queued: true, err: "task of array job {job:eval}"},
	{name: "attempt ID", cmd: "remove", args: "-b {B} -p sweep -j {att:train-SEED2/0}", queued: true, err: "attempt ID", known: knownAttemptEdits},

	// run and retry: which jobs the new run executes.
	{name: "failed and unfinished", cmd: "retry", args: "-b {B} -p sweep", jobs: []string{"eval-2", "train-SEED2"}},
	{name: "stage", cmd: "retry", args: "-b {B} -p sweep --stage training", jobs: []string{"train-SEED2"}},
	{name: "job ID in addition", cmd: "retry", args: "-b {B} -p sweep --job-id {job:prep}", jobs: []string{"eval-2", "prep", "train-SEED2"}, known: knownRetryJobID},
	{name: "given run", cmd: "run", args: "-b {B} -p sweep --run-id {run:sweep-first} --failed", jobs: []string{"eval-2", "train-SEED2"}},
	{name: "stage", cmd: "run", args: "-b {B} -p sweep --stage evaluation", jobs: []string{"eval-1", "eval-2", "eval-3"}},
	{name: "matrix", cmd: "run", args: "-b {B} -p sweep --matrix train", jobs: []string{"train-SEED1", "train-SEED2"}},
	{name: "matrix and failed", cmd: "run", args: "-b {B} -p sweep --matrix train --failed", jobs: []string{"train-SEED2"}},
	{name: "job ID", cmd: "run", args: "-b {B} -p sweep --job-id {job:prep}", jobs: []string{"prep"}},
	{name: "job name", cmd: "run", args: "-b {B} -p sweep --job-name prep", jobs: []string{"prep"}},
	{name: "job name shared by projects", cmd: "run", args: "-b {B} --job-name prep", err: "ambiguous"},
	{name: "attempt ID", cmd: "run", args: "-b {B} --job-id {att:train-SEED2/0}", jobs: []string{"train-SEED2"}},
	{name: "array command ID", cmd: "run", args: "-b {B} -p sweep --job-id {job:eval}", jobs: []string{"eval-1", "eval-2", "eval-3"}, known: knownArrayLookup},
	{name: "array task ID", cmd: "run", args: "-b {B} -p sweep --job-id {job:eval}-2", jobs: []string{"eval-2"}, known: knownArrayLookup},
	{name: "unknown stage", cmd: "run", args: "-b {B} -p sweep --stage nope", err: `no jobs in stage "nope"`},
	{name: "stage and job ID", cmd: "run", args: "-b {B} -p sweep --stage training --job-id {job:prep}", err: "cannot be combined"},
}
