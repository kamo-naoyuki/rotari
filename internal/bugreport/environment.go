package bugreport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/config"
	"github.com/rotari-dev/rotari/internal/notification"
)

// schedulerCommands are the scheduler client tools rotari runs; their
// presence tells which scheduler this host can reach.
var schedulerCommands = []string{"sbatch", "squeue", "sacct", "qsub", "qstat", "qacct", "bsub", "bjobs", "bhist"}

// schedulerVersionCommands print a scheduler's version.
var schedulerVersionCommands = [][]string{{"sbatch", "--version"}, {"qstat", "--version"}, {"lsid"}}

func (collector *collector) environment() {
	options := collector.options
	builder := &collector.summary
	builder.WriteString("## rotari and host\n\n")
	fmt.Fprintf(builder, "- Version: %s\n", options.Version)
	fmt.Fprintf(builder, "- Go: %s, %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if hostname, err := os.Hostname(); err == nil {
		fmt.Fprintf(builder, "- Host: %s\n", hostname)
	}
	if options.BaseDir != "" {
		fmt.Fprintf(builder, "- Basedir: %s\n", options.BaseDir)
		fmt.Fprintf(builder, "- Basedir filesystem: %s\n", filesystemType(options.BaseDir))
		fmt.Fprintf(builder, "- Basedir clock: %s\n", describeClockSkew(options.BaseDir, options.Now))
	}
	if options.Project != "" {
		fmt.Fprintf(builder, "- Project: %s\n", options.Project)
	}
	builder.WriteString("- Scheduler commands:")
	found := false
	for _, name := range schedulerCommands {
		if path, err := exec.LookPath(name); err == nil {
			fmt.Fprintf(builder, " %s (%s)", name, path)
			found = true
		}
	}
	if !found {
		builder.WriteString(" none found in PATH")
	}
	builder.WriteString("\n")
	for _, command := range schedulerVersionCommands {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		output, err := runProbe(command)
		line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(output), "\n", 2)[0])
		if err != nil {
			line = strings.TrimSpace(line + " (" + err.Error() + ")")
		}
		fmt.Fprintf(builder, "- `%s`: %s\n", strings.Join(command, " "), line)
	}
	builder.WriteString("\n")

	variables := redactEnvironment(options.Environ)
	builder.WriteString("## ROTARI_* environment variables\n\n")
	if len(variables) == 0 {
		builder.WriteString("None set.\n\n")
	} else {
		writeFence(builder, strings.Join(variables, "\n"))
		collector.addFile("environment.txt", []byte(strings.Join(variables, "\n")+"\n"))
	}
}

// describeClockSkew compares the modification time the basedir's
// filesystem gives a new file with this host's clock. A shared filesystem
// whose server clock differs makes recorded times look wrong.
func describeClockSkew(dir string, now func() time.Time) string {
	file, err := os.CreateTemp(dir, ".rotari-bug-report-")
	if err != nil {
		return "not measured (" + err.Error() + ")"
	}
	name := file.Name()
	defer os.Remove(name)
	before := now()
	_, writeErr := file.WriteString("clock probe\n")
	closeErr := file.Close()
	after := now()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "not measured (" + err.Error() + ")"
	}
	info, err := os.Stat(name)
	if err != nil {
		return "not measured (" + err.Error() + ")"
	}
	local := before.Add(after.Sub(before) / 2)
	skew := info.ModTime().Sub(local).Round(time.Second)
	switch {
	case skew > time.Second:
		return fmt.Sprintf("filesystem time is %s ahead of this host", skew)
	case skew < -time.Second:
		return fmt.Sprintf("filesystem time is %s behind this host", -skew)
	default:
		return "matches this host within 1s"
	}
}

func runProbe(command []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, command[0], command[1:]...).CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("timed out after %s", commandTimeout)
	}
	return string(output), err
}

func (collector *collector) configuration() {
	builder := &collector.summary
	builder.WriteString("## Configuration files\n\n")
	common, projects := config.ListPaths(collector.options.BaseDir, collector.options.Project, notification.FileName)
	paths := append([]string(nil), common...)
	projectNames := make([]string, 0, len(projects))
	for name := range projects {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)
	for _, name := range projectNames {
		paths = append(paths, projects[name]...)
	}
	if len(paths) == 0 {
		builder.WriteString("None found.\n\n")
		return
	}
	for index, path := range paths {
		name := fmt.Sprintf("config/%02d-%s.toml", index+1, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		data, err := readLimited(path, stateFileBytes)
		if err == nil {
			data, err = redactConfig(path, data)
		}
		if err != nil {
			fmt.Fprintf(builder, "- %s: content omitted (%v)\n", path, err)
			continue
		}
		collector.addFile(name, data)
		fmt.Fprintf(builder, "- %s: `%s`\n", path, name)
	}
	builder.WriteString("\n")
}

func (collector *collector) facts() {
	if len(collector.options.Facts) == 0 {
		return
	}
	builder := &collector.summary
	builder.WriteString("## rotari's own view\n\n")
	for _, fact := range collector.options.Facts {
		data := fact.Data
		if strings.HasSuffix(fact.Name, ".json") {
			if redacted, err := redactJSON(data); err == nil {
				data = redacted
			}
		}
		collector.addFile(fact.Name, data)
		fmt.Fprintf(builder, "### `%s`\n\n", fact.Name)
		writeFence(builder, string(data))
	}
}

func (collector *collector) serverLog() {
	if collector.options.BaseDir == "" {
		return
	}
	builder := &collector.summary
	data, err := readTail(filepath.Join(collector.options.BaseDir, "server.log"), serverLogBytes)
	if errors.Is(err, os.ErrNotExist) {
		builder.WriteString("## server.log\n\nNo server.log in the basedir.\n\n")
		return
	}
	if err != nil {
		collector.note("server.log not read: %v", err)
		return
	}
	collector.addFile("server.log", data)
	fmt.Fprintf(builder, "## server.log (last %d lines)\n\n", summaryLogLines)
	writeFence(builder, lastLines(string(data), summaryLogLines))
}

func readLimited(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}

// readTail reads at most the last limit bytes of path.
func readTail(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		if _, err := file.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(file, limit))
}
