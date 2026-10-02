// Command mcp-cmd prints the same job report exposed by the rotari MCP tool.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("mcp-cmd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	basedir := flags.String("basedir", "", "rotari state directory")
	project := flags.String("project", "", "rotari project name")
	runID := flags.String("run-id", "", "exact run ID containing the job")
	jobID := flags.String("job-id", "", "exact job ID to inspect")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "mcp-cmd does not accept positional arguments")
		return 2
	}

	result, err := rotarimcp.GetJobInfo(rotarimcp.GetJobInfoInput{
		BaseDir: *basedir,
		Project: *project,
		RunID:   *runID,
		JobID:   *jobID,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := fmt.Fprint(stdout, result.Report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
