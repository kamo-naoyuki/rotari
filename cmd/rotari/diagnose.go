package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
)

const (
	defaultLLMProvider   = "openai"
	defaultLLMEndpoint   = "https://api.openai.com/v1/responses"
	chatLLMEndpoint      = "https://api.openai.com/v1/chat/completions"
	anthropicLLMEndpoint = "https://api.anthropic.com/v1/messages"
	cohereLLMEndpoint    = "https://api.cohere.com/v2/chat"
	geminiLLMEndpoint    = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
	diagnosisLogLimit    = 12000
)

const noRuleDiagnosisName = "No known rule-based diagnosis matched"
const unavailableRuleDiagnosisName = "Rule-based diagnosis unavailable"

type diagnosisJob struct {
	RunID    string
	JobID    string
	Command  []string
	ExitCode *int
	Error    string
	Log      string
}

type diagnosisRule struct {
	Name       string
	Patterns   []*regexp.Regexp
	Excludes   []*regexp.Regexp
	Suggestion string
}

var diagnosisRules = []diagnosisRule{
	{Name: "CUDA/GPU memory exhausted", Patterns: []*regexp.Regexp{regexp.MustCompile(`cuda.*out of memory`), regexp.MustCompile(`torch\.cuda\.outofmemoryerror`), regexp.MustCompile(`cublas_status_alloc_failed`), regexp.MustCompile(`cudnn_status_alloc_failed`)}, Suggestion: "Reduce batch size or model memory use, select a GPU with more free memory, and check for other processes using the GPU."},
	{Name: "CUDA/GPU unavailable", Patterns: []*regexp.Regexp{regexp.MustCompile(`no cuda-capable device`), regexp.MustCompile(`cuda driver version is insufficient`), regexp.MustCompile(`nvidia-smi has failed`)}, Suggestion: "Confirm that the assigned host has a usable GPU and that the NVIDIA driver and CUDA runtime are compatible."},
	{Name: "Slurm memory limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`slurmstepd: error: detected .*oom-kill`), regexp.MustCompile(`exceeded job memory limit`), regexp.MustCompile(`state=out_of_memory`)}, Suggestion: "Request more Slurm memory with the appropriate scheduler option or reduce memory use; inspect sacct for the job state and peak memory."},
	{Name: "Slurm time limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`slurm.*time limit`), regexp.MustCompile(`cancelled .* due to time limit`), regexp.MustCompile(`state=time_limit`)}, Suggestion: "Increase the Slurm time limit if policy permits, or reduce runtime and checkpoint the job before its allocation expires."},
	{Name: "PBS resource or walltime limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`walltime.*exceeded`), regexp.MustCompile(`exceeded.*walltime`), regexp.MustCompile(`job exceeded.*resource limit`)}, Suggestion: "Increase the requested PBS walltime or resource limit if permitted, or reduce runtime and resource use."},
	{Name: "LSF memory limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`term_memlimit`)}, Suggestion: "Increase the LSF memory limit if permitted, or reduce the job's memory use."},
	{Name: "LSF run time limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`term_runlimit`), regexp.MustCompile(`term_cpulimit`)}, Suggestion: "Increase the LSF run or CPU time limit if permitted, or reduce runtime and checkpoint the job."},
	{Name: "Scheduler submission failed", Patterns: []*regexp.Regexp{regexp.MustCompile(`sbatch: error:`), regexp.MustCompile(`qsub:.*error`), regexp.MustCompile(`bsub:.*error`)}, Excludes: []*regexp.Regexp{regexp.MustCompile(`invalid qos specification`), regexp.MustCompile(`invalid partition`), regexp.MustCompile(`invalid account`), regexp.MustCompile(`invalid generic resource`)}, Suggestion: "Check the scheduler command, account or project, partition or queue, requested resources, and submission permissions."},
	{Name: "Invalid scheduler resource request", Patterns: []*regexp.Regexp{regexp.MustCompile(`invalid qos specification`), regexp.MustCompile(`invalid partition`), regexp.MustCompile(`invalid account`), regexp.MustCompile(`invalid generic resource`), regexp.MustCompile(`(?:pbs|lsf).*invalid.*resource`)}, Suggestion: "Check the partition or queue, account, QOS, GPU or resource names, and requested limits."},
	{Name: "Scheduler cancelled job", Patterns: []*regexp.Regexp{regexp.MustCompile(`slurmstepd.*cancelled`), regexp.MustCompile(`state=cancelled`), regexp.MustCompile(`jobstate=cancelled`), regexp.MustCompile(`job.*cancelled by scheduler`), regexp.MustCompile(`pbs.*job.*deleted`), regexp.MustCompile(`term_owner`), regexp.MustCompile(`term_admin`), regexp.MustCompile(`term_preempt`)}, Excludes: []*regexp.Regexp{regexp.MustCompile(`time limit`)}, Suggestion: "Inspect scheduler accounting for the cancellation reason and actor; retry only after resolving policy, preemption, dependency, or administrator cancellation conditions."},
	{Name: "Host memory exhausted", Patterns: []*regexp.Regexp{regexp.MustCompile(`out of memory: kill process`), regexp.MustCompile(`oom-kill`), regexp.MustCompile(`memory cgroup out of memory`), regexp.MustCompile(`killed process .* out of memory`)}, Excludes: []*regexp.Regexp{regexp.MustCompile(`slurmstepd: error: detected .*oom-kill`)}, Suggestion: "Request more host memory or reduce the process memory use; inspect scheduler memory limits and kernel OOM messages."},
	{Name: "Process killed", Patterns: []*regexp.Regexp{regexp.MustCompile(`^killed$`), regexp.MustCompile(`^killed [0-9]+$`)}, Suggestion: "Inspect scheduler accounting and host logs: SIGKILL can result from an OOM killer, a scheduler resource limit, or explicit cancellation."},
	{Name: "Kernel panic or kernel fault", Patterns: []*regexp.Regexp{regexp.MustCompile(`kernel panic`), regexp.MustCompile(`bug: unable to handle kernel`), regexp.MustCompile(`oops:`), regexp.MustCompile(`general protection fault`), regexp.MustCompile(`watchdog:.*(?:hard|soft) lockup`), regexp.MustCompile(`rcu.*stall`)}, Suggestion: "Treat the compute node as unhealthy: inspect kernel logs and scheduler node health, report it to the cluster administrator, and retry on another node if appropriate."},
	{Name: "Application panic", Patterns: []*regexp.Regexp{regexp.MustCompile(`^panic:`)}, Suggestion: "Inspect the application stack trace and failing invariant; fix the application error before retrying."},
	{Name: "Segmentation fault", Patterns: []*regexp.Regexp{regexp.MustCompile(`segmentation fault`), regexp.MustCompile(`sigsegv`), regexp.MustCompile(`signal 11`)}, Suggestion: "Inspect native extensions, shared-library and driver compatibility, and a core dump or debugger backtrace if available."},
	{Name: "NVIDIA GPU driver/device error", Patterns: []*regexp.Regexp{regexp.MustCompile(`nvrm: xid`), regexp.MustCompile(`gpu has fallen off the bus`)}, Suggestion: "Inspect GPU and node health plus NVIDIA kernel logs; retry on another GPU or node if appropriate."},
	{Name: "CUDA device-side assert", Patterns: []*regexp.Regexp{regexp.MustCompile(`device-side assert triggered`)}, Suggestion: "Check tensor shapes, labels, and index ranges passed to CUDA kernels; rerun with synchronous CUDA error reporting if needed."},
	{Name: "NCCL failure", Patterns: []*regexp.Regexp{regexp.MustCompile(`nccl (?:error|warn)`), regexp.MustCompile(`nccl.*unhandled system error`)}, Suggestion: "Check GPU and node connectivity, NCCL configuration, network-interface selection, and consistency across distributed ranks."},
	{Name: "MPI runtime failure", Patterns: []*regexp.Regexp{regexp.MustCompile(`mpi_abort`), regexp.MustCompile(`pmi.*(?:error|failed)`), regexp.MustCompile(`(?:mpirun|mpiexec).*(?:fatal|error|abort)`)}, Suggestion: "Check MPI and runtime compatibility, rank counts, host allocation, and launcher configuration."},
	{Name: "Disk space or quota exhausted", Patterns: []*regexp.Regexp{regexp.MustCompile(`no space left on device`), regexp.MustCompile(`disk quota exceeded`)}, Suggestion: "Free space or files in the relevant filesystem, or use a location with sufficient quota and capacity."},
	{Name: "Storage device I/O error", Patterns: []*regexp.Regexp{regexp.MustCompile(`input/output error`), regexp.MustCompile(`\beio\b`), regexp.MustCompile(`blk_update_request`), regexp.MustCompile(`buffer i/o error`), regexp.MustCompile(`i/o error, dev`)}, Suggestion: "Inspect kernel and storage-service logs, then check the affected disk or network filesystem with the storage administrator before retrying writes."},
	{Name: "Read-only filesystem", Patterns: []*regexp.Regexp{regexp.MustCompile(`read-only file system`), regexp.MustCompile(`erofs`)}, Suggestion: "Check the mount state and write location, then use a writable filesystem."},
	{Name: "File or directory not found", Patterns: []*regexp.Regexp{regexp.MustCompile(`no such file or directory`), regexp.MustCompile(`filenotfounderror`), regexp.MustCompile(`enoent`)}, Suggestion: "Check the path, working directory, mounted filesystems, and whether an earlier job produced the required file."},
	{Name: "Permission denied", Patterns: []*regexp.Regexp{regexp.MustCompile(`permission denied`), regexp.MustCompile(`eacces`)}, Excludes: []*regexp.Regexp{regexp.MustCompile(`permission denied \(publickey\)`)}, Suggestion: "Check ownership, file and directory permissions, mount options, and the account used by the job."},
	{Name: "Command or executable not found", Patterns: []*regexp.Regexp{regexp.MustCompile(`command not found`), regexp.MustCompile(`executable file not found`)}, Suggestion: "Check the command spelling and PATH, or use an absolute path and ensure the executable is installed on the execution host."},
	{Name: "Shared library or ABI mismatch", Patterns: []*regexp.Regexp{regexp.MustCompile(`undefined symbol`), regexp.MustCompile(`symbol lookup error`), regexp.MustCompile(`glibcxx_[0-9.]+.*not found`), regexp.MustCompile(`cxxabi_[0-9.]+.*not found`), regexp.MustCompile(`cannot open shared object file`), regexp.MustCompile(`wrong elf class`), regexp.MustCompile(`undefined reference to`)}, Suggestion: "Check ldd output, LD_LIBRARY_PATH, and the loaded .so files; align the compiler, libstdc++, CUDA, and Python-extension ABI versions with the runtime host."},
	{Name: "Python import or module missing", Patterns: []*regexp.Regexp{regexp.MustCompile(`modulenotfounderror`), regexp.MustCompile(`importerror`), regexp.MustCompile(`cannot import name`)}, Excludes: []*regexp.Regexp{regexp.MustCompile(`undefined symbol`), regexp.MustCompile(`symbol lookup error`), regexp.MustCompile(`glibcxx_[0-9.]+.*not found`), regexp.MustCompile(`cxxabi_[0-9.]+.*not found`), regexp.MustCompile(`cannot open shared object file`), regexp.MustCompile(`wrong elf class`)}, Suggestion: "Check the Python environment, PYTHONPATH, package installation, and version compatibility."},
	{Name: "Python dependency or version conflict", Patterns: []*regexp.Regexp{regexp.MustCompile(`requires .* but .* is installed`), regexp.MustCompile(`dependency resolver`), regexp.MustCompile(`incompatible version`)}, Suggestion: "Check package versions and the environment lockfile or requirements, then install a compatible set."},
	{Name: "Python syntax or indentation error", Patterns: []*regexp.Regexp{regexp.MustCompile(`syntaxerror`), regexp.MustCompile(`indentationerror`), regexp.MustCompile(`taberror`)}, Suggestion: "Inspect the reported source line for invalid syntax, indentation, tabs, or an incompatible Python language feature."},
	{Name: "Python missing name or attribute", Patterns: []*regexp.Regexp{regexp.MustCompile(`nameerror`), regexp.MustCompile(`unboundlocalerror`), regexp.MustCompile(`attributeerror`)}, Suggestion: "Check spelling, imports, object types, and variable initialization along the failing code path."},
	{Name: "Python type or value error", Patterns: []*regexp.Regexp{regexp.MustCompile(`typeerror`), regexp.MustCompile(`valueerror`)}, Suggestion: "Check function arguments, input shapes and formats, and conversions at the reported traceback location."},
	{Name: "Python key or index error", Patterns: []*regexp.Regexp{regexp.MustCompile(`keyerror`), regexp.MustCompile(`indexerror`)}, Suggestion: "Validate dictionary keys, list or array bounds, and assumptions about input or intermediate data."},
	{Name: "Python assertion failed", Patterns: []*regexp.Regexp{regexp.MustCompile(`assertionerror`)}, Suggestion: "Inspect the failed assertion and the input or invariant it checks; preserve the relevant values in the job log if needed."},
	{Name: "Python memory error", Patterns: []*regexp.Regexp{regexp.MustCompile(`memoryerror`)}, Suggestion: "Reduce in-memory data or worker concurrency, stream or batch input, and confirm the scheduler memory limit is sufficient."},
	{Name: "Python recursion limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`recursionerror`), regexp.MustCompile(`maximum recursion depth exceeded`)}, Suggestion: "Check for unintended recursion or cycles; rewrite the algorithm iteratively or adjust recursion depth only when safe."},
	{Name: "File descriptor limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`too many open files`), regexp.MustCompile(`emfile`)}, Suggestion: "Check file descriptor limits and close leaked descriptors; inspect ulimit -n."},
	{Name: "Process or thread limit exceeded", Patterns: []*regexp.Regexp{regexp.MustCompile(`fork: retry`), regexp.MustCompile(`fork.*resource temporarily unavailable`), regexp.MustCompile(`pthread_create.*resource temporarily unavailable`), regexp.MustCompile(`eagain.*(?:fork|thread|process)`)}, Suggestion: "Check process and thread limits, then reduce worker count or request a suitable scheduler limit."},
	{Name: "DNS lookup failed", Patterns: []*regexp.Regexp{regexp.MustCompile(`temporary failure in name resolution`), regexp.MustCompile(`could not resolve host`), regexp.MustCompile(`no such host`)}, Suggestion: "Check the hostname, DNS resolver configuration, and whether the compute node can reach the configured DNS service."},
	{Name: "Network connection refused", Patterns: []*regexp.Regexp{regexp.MustCompile(`connection refused`), regexp.MustCompile(`econnrefused`)}, Suggestion: "Check the destination host and port, service availability, and firewall or security-group rules."},
	{Name: "Network connection timed out", Patterns: []*regexp.Regexp{regexp.MustCompile(`connection timed out`), regexp.MustCompile(`i/o timeout`), regexp.MustCompile(`etimedout`)}, Suggestion: "Check network routing, firewall rules, VPN or proxy requirements, and whether the destination is overloaded."},
	{Name: "Network connection reset or closed", Patterns: []*regexp.Regexp{regexp.MustCompile(`connection reset by peer`), regexp.MustCompile(`econnreset`), regexp.MustCompile(`broken pipe`), regexp.MustCompile(`unexpected eof`)}, Suggestion: "Inspect the remote service for restarts or crashes and check proxy or load-balancer connection timeouts."},
	{Name: "TLS certificate or handshake failure", Patterns: []*regexp.Regexp{regexp.MustCompile(`certificate verify failed`), regexp.MustCompile(`x509: certificate`), regexp.MustCompile(`tls handshake timeout`)}, Suggestion: "Check the system clock, certificate chain and CA configuration, and any TLS-intercepting proxy."},
	{Name: "HTTP authentication or authorization failed", Patterns: []*regexp.Regexp{regexp.MustCompile(`http.*(401|403)`), regexp.MustCompile(`status code (401|403)`), regexp.MustCompile(`401 unauthorized`), regexp.MustCompile(`403 forbidden`)}, Suggestion: "Check credentials, access tokens, scopes, and the target service's authorization policy."},
	{Name: "HTTP rate limited", Patterns: []*regexp.Regexp{regexp.MustCompile(`http.*429`), regexp.MustCompile(`status code 429`), regexp.MustCompile(`429 too many requests`), regexp.MustCompile(`rate limit exceeded`)}, Suggestion: "Wait or apply exponential backoff, reduce request concurrency, and check the service quota or rate-limit policy."},
	{Name: "HTTP service unavailable", Patterns: []*regexp.Regexp{regexp.MustCompile(`http.*503`), regexp.MustCompile(`status code 503`), regexp.MustCompile(`503 service unavailable`), regexp.MustCompile(`upstream unavailable`)}, Suggestion: "Retry after a delay; if it persists, check service health, maintenance notices, and proxy or load-balancer upstreams."},
	{Name: "HTTP server or bad gateway error", Patterns: []*regexp.Regexp{regexp.MustCompile(`http.*(500|502)`), regexp.MustCompile(`status code (500|502)`), regexp.MustCompile(`500 internal server error`), regexp.MustCompile(`502 bad gateway`)}, Suggestion: "Retry transient requests and inspect the target service and any proxy or load-balancer logs for the upstream failure."},
	{Name: "HTTP gateway timeout", Patterns: []*regexp.Regexp{regexp.MustCompile(`http.*504`), regexp.MustCompile(`status code 504`), regexp.MustCompile(`504 gateway timeout`)}, Suggestion: "Check target-service latency and proxy timeout settings; retry with backoff when the operation is safe to repeat."},
	{Name: "SSH authentication or host verification failed", Patterns: []*regexp.Regexp{regexp.MustCompile(`permission denied \(publickey\)`), regexp.MustCompile(`host key verification failed`)}, Suggestion: "Check SSH keys and permissions, the known_hosts entry, and SSH server access controls."},
}

func cmdDiagnose(args []string) int {
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	jobID := cliString(fs, "job-id", "")
	attemptID := ""
	provider := cliString(fs, "provider", defaultLLMProvider)
	endpoint := cliString(fs, "endpoint", defaultLLMEndpoint)
	model := cliString(fs, "model", "")
	language := cliString(fs, "language", "")
	rules := cliBool(fs, "rules", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 || *jobID == "" || (!*rules && *model == "") {
		printError("usage: " + cliUsage("diagnose") + " (requires --job-id and --model unless --rules is set)")
		return 1
	}
	if strings.HasPrefix(*jobID, "att_") {
		attemptID = *jobID
		baseDir, resolvedProjectName, resolvedRunID, resolvedJobID, err := resolveAttemptTarget(*jobID, *basedir, *projectName, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		*basedir, *projectName, *runIDOption, *jobID = baseDir, resolvedProjectName, resolvedRunID, resolvedJobID
	}
	if *rules {
		baseDir, queueName, err := resolveExistingRunTarget(*basedir, *projectName, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		paths, err := resolvePaths(baseDir, queueName)
		if err != nil {
			printErrorf("failed to resolve paths: %v", err)
			return 1
		}
		runID, err := selectRunID(paths, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		job, err := loadDiagnosisJob(paths, runID, *jobID, attemptID)
		if err != nil {
			printError(err)
			return 1
		}
		fmt.Print(formatRuleDiagnoses(diagnoseWithRules(job)))
		return 0
	}
	if *provider != "openai" && *provider != "openai-chat" && *provider != "anthropic" && *provider != "gemini" && *provider != "cohere" {
		printErrorf("unsupported LLM provider %q; use openai, openai-chat, anthropic, gemini, or cohere", *provider)
		return 1
	}
	if *provider == "anthropic" && *endpoint == defaultLLMEndpoint {
		*endpoint = anthropicLLMEndpoint
	}
	if *provider == "gemini" && *endpoint == defaultLLMEndpoint {
		*endpoint = fmt.Sprintf(geminiLLMEndpoint, *model)
	}
	if *provider == "openai-chat" && *endpoint == defaultLLMEndpoint {
		*endpoint = chatLLMEndpoint
	}
	if *provider == "cohere" && *endpoint == defaultLLMEndpoint {
		*endpoint = cohereLLMEndpoint
	}
	if *language != "" && !isLanguageTag(*language) {
		printErrorf("invalid language tag %q; use a BCP 47 tag such as ja or en-US", *language)
		return 1
	}
	apiKey := os.Getenv(envLLMAPIKey)
	if apiKey == "" {
		printError("ROTARI_LLM_API_KEY is required; it is not stored in rotari state")
		return 1
	}
	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *projectName, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	runID, err := selectRunID(paths, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	job, err := loadDiagnosisJob(paths, runID, *jobID, attemptID)
	if err != nil {
		printError(err)
		return 1
	}
	answer, err := requestProviderDiagnosis(context.Background(), *provider, *endpoint, apiKey, *model, diagnosisPrompt(job, *language))
	if err != nil {
		printErrorf("LLM diagnosis failed: %v", err)
		return 1
	}
	fmt.Println(answer)
	return 0
}

func loadDiagnosisJob(paths pathSet, runID, jobID string, attemptIDs ...string) (diagnosisJob, error) {
	if !isValidPathElement(runID) || !isValidPathElement(jobID) {
		return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	for range 16 {
		runDir, err := validatedRunDir(paths, runID)
		if err != nil {
			return diagnosisJob{}, err
		}
		jobDir, err := latestAttemptJobDir(runDir, jobID)
		attemptID := ""
		if len(attemptIDs) > 0 {
			attemptID = attemptIDs[0]
		}
		if attemptID != "" {
			jobDir, err = specificAttemptJobDir(runDir, jobID, attemptID)
		}
		if err != nil {
			return diagnosisJob{}, err
		}
		info, err := os.Stat(jobDir)
		if err != nil || !info.IsDir() {
			if attemptID != "" {
				return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			origin := loadRunOrigin(runDir, jobID)
			if origin == nil {
				return diagnosisJob{}, fmt.Errorf(jobNotFoundMessage, jobID, runID)
			}
			runID, jobID = origin.RunID, origin.JobID
			continue
		}
		data, err := os.ReadFile(filepath.Join(jobDir, commandJSONName))
		if err != nil {
			return diagnosisJob{}, fmt.Errorf("read job command: %w", err)
		}
		var spec JobSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return diagnosisJob{}, fmt.Errorf("read job command: %w", err)
		}
		log, err := os.ReadFile(filepath.Join(jobDir, "output"))
		if err != nil && !os.IsNotExist(err) {
			return diagnosisJob{}, fmt.Errorf("read job output: %w", err)
		}
		job := diagnosisJob{RunID: runID, JobID: jobID, Command: spec.Command, Log: tailString(string(log), diagnosisLogLimit)}
		if summary, err := loadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
			for _, result := range summary.Results {
				if result.ID == jobID {
					exitCode := result.ExitCode
					job.ExitCode, job.Error = &exitCode, result.Error
					break
				}
			}
		}
		return job, nil
	}
	return diagnosisJob{}, fmt.Errorf("job %q has too many carried-forward origins", jobID)
}

func tailString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return "[earlier log output omitted]\n" + value[len(value)-limit:]
}

func diagnoseWithRules(job diagnosisJob) []ruleDiagnosis {
	rules := make([]diagnose.Rule, 0, len(diagnosisRules))
	for _, rule := range diagnosisRules {
		rules = append(rules, diagnose.Rule{Name: rule.Name, Patterns: rule.Patterns, Excludes: rule.Excludes, Suggestion: rule.Suggestion})
	}
	return diagnose.Diagnose(job.Error, job.Log, rules)
}

func matchesAny(value string, patterns []*regexp.Regexp) bool {
	return diagnose.MatchesAny(value, patterns)
}

func diagnoseJobResult(runDir string, result JobResult) JobResult {
	if result.ExitCode == 0 || len(result.Diagnoses) > 0 {
		return result
	}
	if !isValidPathElement(result.ID) {
		return unavailableRuleDiagnosis(result, "The job ID is invalid, so its output could not be inspected.")
	}
	jobDir, err := latestAttemptJobDir(runDir, result.ID)
	if err != nil {
		return unavailableRuleDiagnosis(result, "The job directory could not be resolved: "+err.Error())
	}
	data, err := os.ReadFile(filepath.Join(jobDir, "output"))
	if err != nil && !os.IsNotExist(err) {
		return unavailableRuleDiagnosis(result, "The job output could not be read: "+err.Error())
	}
	result.Diagnoses = diagnoseWithRules(diagnosisJob{
		JobID: result.ID,
		Error: result.Error,
		Log:   tailString(string(data), diagnosisLogLimit),
	})
	if len(result.Diagnoses) == 0 {
		result.Diagnoses = []ruleDiagnosis{{
			Name:       noRuleDiagnosisName,
			Evidence:   "No recognized signature in the recorded scheduler error or log.",
			Suggestion: "Inspect the full job output and scheduler accounting for the failure details.",
		}}
	}
	return result
}

func unavailableRuleDiagnosis(result JobResult, evidence string) JobResult {
	result.Diagnoses = []ruleDiagnosis{{
		Name:       unavailableRuleDiagnosisName,
		Evidence:   evidence,
		Suggestion: "Inspect the job directory and output file permissions, then run rotari diagnose --rules after resolving the read error.",
	}}
	return result
}

func formatRuleDiagnoses(diagnoses []ruleDiagnosis) string {
	if len(diagnoses) == 0 {
		return "No known rule-based diagnosis matched the recorded error or log. Inspect the full job output with rotari show --run-id RUN_ID --job-id JOB_ID.\n"
	}
	var output strings.Builder
	for _, diagnosis := range diagnoses {
		fmt.Fprintf(&output, "%s\nEvidence: %s\nNext: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
	}
	return output.String()
}

func isLanguageTag(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) == 0 || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 8 {
			return false
		}
		for _, character := range part {
			if !('a' <= character && character <= 'z' || 'A' <= character && character <= 'Z' || '0' <= character && character <= '9') {
				return false
			}
		}
	}
	return true
}

func diagnosisPrompt(job diagnosisJob, language string) string {
	status := "not recorded"
	if job.ExitCode != nil {
		status = fmt.Sprintf("exit code %d", *job.ExitCode)
	}
	languageInstruction := ""
	if language != "" {
		languageInstruction = fmt.Sprintf(" Respond in the language identified by the BCP 47 tag %q.", language)
	}
	return fmt.Sprintf("Diagnose this failed rotari job. Explain the likely root cause, cite evidence from the log, and give minimal concrete next steps. Do not claim to have executed anything.%s\n\nRun: %s\nJob: %s\nStatus: %s\nScheduler error: %s\nCommand: %s\n\nLog tail:\n%s", languageInstruction, job.RunID, job.JobID, status, job.Error, strings.Join(job.Command, " "), job.Log)
}

func requestDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	return requestProviderDiagnosis(ctx, "openai", endpoint, apiKey, model, prompt)
}

func requestProviderDiagnosis(ctx context.Context, provider, endpoint, apiKey, model, prompt string) (string, error) {
	return diagnose.RequestProviderDiagnosis(ctx, provider, endpoint, apiKey, model, prompt)
}

func requestOpenAIDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{Model: model, Input: prompt})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", authBearerPrefix+apiKey)
	request.Header.Set(headerContentType, mimeApplicationJSON)
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	if decoded.OutputText != "" {
		return decoded.OutputText, nil
	}
	for _, output := range decoded.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" && content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("API response did not include output text")
}

func requestChatCompletionsDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model: model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", authBearerPrefix+apiKey)
	request.Header.Set(headerContentType, mimeApplicationJSON)
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, choice := range decoded.Choices {
		if choice.Message.Content != "" {
			return choice.Message.Content, nil
		}
	}
	return "", fmt.Errorf("API response did not include message content")
}

func requestAnthropicDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:     model,
		MaxTokens: 4096,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("x-api-key", apiKey)
	request.Header.Set("anthropic-version", "2023-06-01")
	request.Header.Set("Content-Type", "application/json")
	return doAnthropicRequest(request)
}

func doAnthropicRequest(request *http.Request) (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, content := range decoded.Content {
		if content.Type == "text" && content.Text != "" {
			return content.Text, nil
		}
	}
	return "", fmt.Errorf("API response did not include text content")
}

func requestGeminiDiagnosis(ctx context.Context, endpoint, apiKey, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}{
		Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{{Parts: []struct {
			Text string `json:"text"`
		}{{Text: prompt}}}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("x-goog-api-key", apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, candidate := range decoded.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				return part.Text, nil
			}
		}
	}
	return "", fmt.Errorf("API response did not include text content")
}

func requestCohereDiagnosis(ctx context.Context, endpoint, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(struct {
		Stream   bool   `json:"stream"`
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model: model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("decode API response: %w", err)
	}
	for _, content := range decoded.Message.Content {
		if content.Type == "text" && content.Text != "" {
			return content.Text, nil
		}
	}
	return "", fmt.Errorf("API response did not include text content")
}
