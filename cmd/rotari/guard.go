package main

import (
	"flag"
	"fmt"
	"slices"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
)

// guardFlags are the --dry-run and --if-revision options of the commands
// that change a project; see project.Guard.
type guardFlags struct {
	dryRun     *bool
	ifRevision *string
	outcome    *project.Outcome
}

// cliGuardFlags registers --dry-run and --if-revision on fs.
func cliGuardFlags(fs *flag.FlagSet) guardFlags {
	return guardFlags{dryRun: cliBool(fs, "dry-run", false), ifRevision: cliString(fs, "if-revision", ""), outcome: &project.Outcome{}}
}

// guard returns the project.Guard the flags ask for, recording its outcome.
func (flags guardFlags) guard() project.Guard {
	return project.Guard{DryRun: *flags.dryRun, IfRevision: *flags.ifRevision, Report: func(outcome project.Outcome) { *flags.outcome = outcome }}
}

// editor returns the queue editor under the flags' guard.
func (flags guardFlags) editor() queueops.Editor {
	editor := queueEditor()
	editor.Guard = flags.guard()
	return editor
}

// printResult prints an operation's message: a dry run is labeled and
// followed by the revision to pass to --if-revision, and an edit applied
// with --if-revision by the revision it produced. Without either option the
// output is the message alone, which quiet suppresses.
func (flags guardFlags) printResult(message string, quiet bool) {
	switch {
	case *flags.dryRun:
		fmt.Println(colorKeyValueMessage("dry run: "+message, green))
		fmt.Printf("revision=%s\n", flags.outcome.Revision)
	case *flags.ifRevision != "":
		if !quiet {
			fmt.Println(colorKeyValueMessage(message, green))
		}
		fmt.Printf("revision=%s\n", flags.outcome.NewRevision)
	case !quiet:
		fmt.Println(colorKeyValueMessage(message, green))
	}
}

// guardedCommands change a project and take --dry-run and --if-revision.
var guardedCommands = []string{"add", "change", "copy", "delete", "import", "remove", "reset"}

// guardFlagSpecs are the specs of --dry-run and --if-revision. They apply to
// one invocation, so neither is read from the environment or config.
func guardFlagSpecs() []cliFlagSpec {
	return []cliFlagSpec{
		{Name: "dry-run", Description: "print what the command would change, and the project revision, without writing", CommandLineOnly: true},
		{Name: "if-revision", Description: "apply only if the project is still at this revision, as printed by --dry-run", ValueName: "REVISION", CommandLineOnly: true},
	}
}

func init() {
	for index := range cliCommandSpecs {
		if slices.Contains(guardedCommands, cliCommandSpecs[index].Name) {
			cliCommandSpecs[index].Flags = append(cliCommandSpecs[index].Flags, guardFlagSpecs()...)
		}
	}
}
