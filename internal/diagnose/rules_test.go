package diagnose

import (
	"strings"
	"testing"
)

func TestDiagnoseDefaultNormalizesAndMatchesKnownErrors(t *testing.T) {
	tests := []struct {
		name string
		job  Job
		want string
	}{
		{name: "CUDA OOM", job: Job{Log: "\x1b[31mCUDA Error: Out   Of Memory\x1b[0m"}, want: "CUDA/GPU memory exhausted"},
		{name: "GPU unavailable", job: Job{Log: "NVIDIA-SMI has failed because it couldn't communicate"}, want: "CUDA/GPU unavailable"},
		{name: "Slurm memory limit", job: Job{Log: "slurmstepd: error: Detected 1 oom-kill event(s) in StepId=1"}, want: "Slurm memory limit exceeded"},
		{name: "Slurm time limit", job: Job{Log: "slurmstepd: error: CANCELLED AT 2026-09-21 DUE TO TIME LIMIT"}, want: "Slurm time limit exceeded"},
		{name: "PBS walltime", job: Job{Log: "PBS: job exceeded walltime limit"}, want: "PBS resource or walltime limit exceeded"},
		{name: "LSF memory limit", job: Job{Log: "Exited with exit code 137. TERM_MEMLIMIT"}, want: "LSF memory limit exceeded"},
		{name: "LSF run limit", job: Job{Log: "TERM_RUNLIMIT: job killed after run limit"}, want: "LSF run time limit exceeded"},
		{name: "scheduler submission", job: Job{Log: "sbatch: error: Unable to contact slurm controller"}, want: "Scheduler submission failed"},
		{name: "invalid scheduler resource", job: Job{Log: "sbatch: error: Invalid qos specification"}, want: "Invalid scheduler resource request"},
		{name: "scheduler cancelled", job: Job{Log: "slurmstepd: error: CANCELLED AT 2026-09-21 DUE TO PREEMPTION"}, want: "Scheduler cancelled job"},
		{name: "host OOM", job: Job{Log: "Memory cgroup out of memory: Killed process 42"}, want: "Host memory exhausted"},
		{name: "killed", job: Job{Log: "Killed"}, want: "Process killed"},
		{name: "kernel fault", job: Job{Log: "BUG: unable to handle kernel NULL pointer dereference"}, want: "Kernel panic or kernel fault"},
		{name: "application panic", job: Job{Log: "panic: unexpected nil pointer"}, want: "Application panic"},
		{name: "Rust panic", job: Job{Log: "thread 'main' panicked at src/main.rs:4:5: explicit panic"}, want: "Application panic"},
		{name: "JVM uncaught exception", job: Job{Log: "Exception in thread \"main\" java.lang.IllegalStateException: invalid state"}, want: "Unhandled application exception"},
		{name: "Node.js unhandled rejection", job: Job{Log: "UnhandledPromiseRejection: request failed"}, want: "Unhandled application exception"},
		{name: "C++ uncaught exception", job: Job{Log: "terminate called after throwing an instance of 'std::runtime_error'"}, want: "Unhandled application exception"},
		{name: "JVM out of memory", job: Job{Log: "java.lang.OutOfMemoryError: Java heap space"}, want: "Runtime memory exhausted"},
		{name: "Node.js heap exhausted", job: Job{Log: "FATAL ERROR: Reached heap limit Allocation failed - JavaScript heap out of memory"}, want: "Runtime memory exhausted"},
		{name: "Rust allocation failed", job: Job{Log: "memory allocation of 1048576 bytes failed"}, want: "Runtime memory exhausted"},
		{name: "JVM stack overflow", job: Job{Log: "java.lang.StackOverflowError"}, want: "Stack overflow"},
		{name: "Node.js stack overflow", job: Job{Log: "RangeError: Maximum call stack size exceeded"}, want: "Stack overflow"},
		{name: "Go stack overflow", job: Job{Log: "runtime: goroutine stack exceeds 1000000000-byte limit\nfatal error: stack overflow"}, want: "Stack overflow"},
		{name: "Node.js missing module", job: Job{Log: "Error: Cannot find module 'left-pad'"}, want: "Application dependency or module missing"},
		{name: "JVM missing class", job: Job{Log: "java.lang.ClassNotFoundException: com.example.Main"}, want: "Application dependency or module missing"},
		{name: "JVM missing class definition", job: Job{Log: "java.lang.NoClassDefFoundError: com/example/Library"}, want: "Application dependency or module missing"},
		{name: "Go missing module", job: Job{Log: "go: no required module provides package example.com/missing"}, want: "Application dependency or module missing"},
		{name: "Rust missing crate", job: Job{Log: "error: failed to resolve: use of undeclared crate or module `serde`"}, want: "Application dependency or module missing"},
		{name: "native undefined reference", job: Job{Log: "main.cpp:(.text+0x10): undefined reference to `missing_symbol'"}, want: "Native linking failed"},
		{name: "native linker missing library", job: Job{Log: "/usr/bin/ld: cannot find -lmissing"}, want: "Native linking failed"},
		{name: "segmentation fault", job: Job{Log: "Fatal Python error: Segmentation fault"}, want: "Segmentation fault"},
		{name: "GPU Xid", job: Job{Log: "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus."}, want: "NVIDIA GPU driver/device error"},
		{name: "CUDA assertion", job: Job{Log: "RuntimeError: CUDA error: device-side assert triggered"}, want: "CUDA device-side assert"},
		{name: "NCCL", job: Job{Log: "NCCL WARN unhandled system error"}, want: "NCCL failure"},
		{name: "MPI", job: Job{Log: "MPI_ABORT was invoked on rank 0"}, want: "MPI runtime failure"},
		{name: "disk quota", job: Job{Log: "write: Disk quota exceeded"}, want: "Disk space or quota exhausted"},
		{name: "storage I/O", job: Job{Log: "cp: error writing 'checkpoint': Input/output error"}, want: "Storage device I/O error"},
		{name: "read-only filesystem", job: Job{Log: "open output: read-only file system"}, want: "Read-only filesystem"},
		{name: "missing file", job: Job{Log: "open input.json: no such file or directory"}, want: "File or directory not found"},
		{name: "permission denied", job: Job{Log: "./train: Permission denied"}, want: "Permission denied"},
		{name: "missing command", job: Job{Log: "python: command not found"}, want: "Command or executable not found"},
		{name: "shared library ABI", job: Job{Log: "ImportError: libstdc++.so.6: version `GLIBCXX_3.4.30' not found"}, want: "Shared library or ABI mismatch"},
		{name: "Python import", job: Job{Log: "ModuleNotFoundError: No module named 'torch'"}, want: "Python import or module missing"},
		{name: "Python dependency", job: Job{Log: "package-a requires package-b but version 1 is installed"}, want: "Python dependency or version conflict"},
		{name: "Python syntax", job: Job{Log: "SyntaxError: invalid syntax"}, want: "Python syntax or indentation error"},
		{name: "Python attribute", job: Job{Log: "AttributeError: 'NoneType' object has no attribute 'shape'"}, want: "Python missing name or attribute"},
		{name: "Python type", job: Job{Log: "TypeError: expected str, got None"}, want: "Python type or value error"},
		{name: "Python key", job: Job{Log: "KeyError: 'learning_rate'"}, want: "Python key or index error"},
		{name: "Python assertion", job: Job{Log: "AssertionError: invalid dataset"}, want: "Python assertion failed"},
		{name: "Python memory", job: Job{Log: "MemoryError"}, want: "Python memory error"},
		{name: "Python recursion", job: Job{Log: "RecursionError: maximum recursion depth exceeded"}, want: "Python recursion limit exceeded"},
		{name: "file descriptors", job: Job{Log: "open: Too many open files"}, want: "File descriptor limit exceeded"},
		{name: "process limit", job: Job{Log: "fork: retry: Resource temporarily unavailable"}, want: "Process or thread limit exceeded"},
		{name: "DNS", job: Job{Log: "curl: (6) Could not resolve host: example.test"}, want: "DNS lookup failed"},
		{name: "connection refused", job: Job{Log: "dial tcp: connection refused"}, want: "Network connection refused"},
		{name: "network timeout", job: Job{Error: "dial tcp: i/o timeout"}, want: "Network connection timed out"},
		{name: "connection reset", job: Job{Log: "read: connection reset by peer"}, want: "Network connection reset or closed"},
		{name: "TLS certificate", job: Job{Log: "x509: certificate signed by unknown authority"}, want: "TLS certificate or handshake failure"},
		{name: "HTTP authorization", job: Job{Log: "HTTP 403 Forbidden"}, want: "HTTP authentication or authorization failed"},
		{name: "HTTP rate limit", job: Job{Log: "request failed: status code 429"}, want: "HTTP rate limited"},
		{name: "HTTP unavailable", job: Job{Log: "503 Service Unavailable"}, want: "HTTP service unavailable"},
		{name: "HTTP gateway error", job: Job{Log: "HTTP 502 Bad Gateway"}, want: "HTTP server or bad gateway error"},
		{name: "HTTP gateway timeout", job: Job{Log: "504 Gateway Timeout"}, want: "HTTP gateway timeout"},
		{name: "SSH key", job: Job{Log: "Permission denied (publickey)."}, want: "SSH authentication or host verification failed"},
		{name: "HTTP status line", job: Job{Log: "< HTTP/1.1 500 Internal Server Error"}, want: "HTTP server or bad gateway error"},
		{name: "Python HTTPError", job: Job{Log: "urllib.error.HTTPError: HTTP Error 429: Too Many Requests"}, want: "HTTP rate limited"},
		{name: "requests HTTPError", job: Job{Log: "403 Client Error: Forbidden for url: https://example.test/api"}, want: "HTTP authentication or authorization failed"},
		{name: "NCCL warning with failure", job: Job{Log: "node1:42 NCCL WARN NET/IB : Got completion with error 12"}, want: "NCCL failure"},
		{name: "kernel oops", job: Job{Log: "[1234.5] Oops: 0002 [#1] SMP PTI"}, want: "Kernel panic or kernel fault"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnoses := DiagnoseDefault(test.job)
			if len(diagnoses) != 1 || diagnoses[0].Name != test.want {
				t.Fatalf("DiagnoseDefault() = %#v, want %q", diagnoses, test.want)
			}
		})
	}
}

func TestDiagnoseDefaultIgnoresLookalikeText(t *testing.T) {
	tests := []struct {
		name string
		log  string
	}{
		{name: "status code in URL", log: "Downloading https://example.test/model-00500-of-01000.safetensors"},
		{name: "status code in HTTP URL path", log: "GET http://example.test/items/403 completed"},
		{name: "step counter", log: "step 429 loss 0.12 lr 5e-4"},
		{name: "informational NCCL warning", log: "node1:42 NCCL WARN NET/IB : No device found."},
		{name: "signal number prefix", log: "received signal 110 from peer"},
		{name: "application oops message", log: "Oops: something went wrong, retrying"},
		{name: "ordinary mention of missing module", log: "The documentation explains how to report a missing module."},
		{name: "ordinary allocation message", log: "memory allocation of 1048576 bytes succeeded"},
		{name: "ordinary linker text", log: "The linker command failed yesterday, but the retry succeeded."},
		{name: "exception name inside identifier", log: "counter.valueerrors=0 keyerrorsuppressed=1"},
		{name: "errno name inside word", log: "prefix_enoentry and leacces and teagain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if diagnoses := DiagnoseDefault(Job{Log: test.log}); len(diagnoses) != 0 {
				t.Fatalf("DiagnoseDefault() = %#v, want no diagnosis", diagnoses)
			}
		})
	}
}

func TestDiagnoseDefaultDoesNotGuess(t *testing.T) {
	if diagnoses := DiagnoseDefault(Job{Log: "the job failed"}); len(diagnoses) != 0 {
		t.Fatalf("DiagnoseDefault() = %#v, want no diagnosis", diagnoses)
	}
}

func TestDiagnoseDefaultReportsSpecificPythonExceptionOnce(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want string
	}{
		{name: "value error", log: "Traceback (most recent call last):\n  File \"train.py\", line 10\nValueError: invalid batch size", want: "Python type or value error"},
		{name: "CUDA OOM is not a Python memory error", log: "Traceback (most recent call last):\ntorch.cuda.OutOfMemoryError: CUDA out of memory. Tried to allocate 2.00 GiB", want: "CUDA/GPU memory exhausted"},
		{name: "missing file", log: "Traceback (most recent call last):\nFileNotFoundError: [Errno 2] No such file or directory: 'x'", want: "File or directory not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnoses := DiagnoseDefault(Job{Log: test.log})
			if len(diagnoses) != 1 || diagnoses[0].Name != test.want {
				t.Fatalf("DiagnoseDefault() = %#v, want only %q", diagnoses, test.want)
			}
		})
	}
}

func TestDiagnoseDefaultFallsBackToFinalPythonException(t *testing.T) {
	log := "Traceback (most recent call last):\n  File \"a.py\", line 1\nRuntimeError: first\nTraceback (most recent call last):\n  File \"a.py\", line 2\nmypkg.CustomError: final failure"
	diagnoses := DiagnoseDefault(Job{Log: log})
	if len(diagnoses) != 1 || diagnoses[0].Name != "Python exception" || diagnoses[0].Evidence != "mypkg.CustomError: final failure" {
		t.Fatalf("DiagnoseDefault() = %#v, want final Python exception", diagnoses)
	}
}

func TestDiagnoseDefaultOrdersLatestEvidenceFirst(t *testing.T) {
	log := "open /data/in.json: no such file or directory\nread: connection reset by peer\nretrying download\nwrite ckpt: No space left on device"
	diagnoses := DiagnoseDefault(Job{Log: log, Error: "sbatch: error: Batch job submission failed"})
	want := []string{"Scheduler submission failed", "Disk space or quota exhausted", "Network connection reset or closed", "File or directory not found"}
	if len(diagnoses) != len(want) {
		t.Fatalf("DiagnoseDefault() = %#v, want %q", diagnoses, want)
	}
	for index, name := range want {
		if diagnoses[index].Name != name {
			t.Fatalf("DiagnoseDefault()[%d] = %q, want %q (all: %#v)", index, diagnoses[index].Name, name, diagnoses)
		}
	}
}

func TestDiagnoseDefaultCitesLatestMatchingLine(t *testing.T) {
	diagnoses := DiagnoseDefault(Job{Log: "dial tcp 10.0.0.1:80: connection refused\nretrying\ndial tcp 10.0.0.2:80: connection refused"})
	if len(diagnoses) != 1 || diagnoses[0].Evidence != "dial tcp 10.0.0.2:80: connection refused" {
		t.Fatalf("DiagnoseDefault() = %#v, want latest matching line as evidence", diagnoses)
	}
}

func TestDefaultRuleIDsAreUnique(t *testing.T) {
	seen := make(map[string]string)
	for _, rule := range DefaultRules() {
		id := RuleID(rule)
		if id == "" {
			t.Fatalf("rule %q has an empty ID", rule.Name)
		}
		if previous, ok := seen[id]; ok {
			t.Fatalf("rules %q and %q share ID %q", previous, rule.Name, id)
		}
		seen[id] = rule.Name
	}
}

func TestResolveAndMatchRuleSelectors(t *testing.T) {
	if err := ResolveRuleSelectors([]string{"cuda-gpu-memory-exhausted", "python IMPORT"}); err != nil {
		t.Fatal(err)
	}
	if err := ResolveRuleSelectors([]string{"not-a-diagnosis"}); err == nil {
		t.Fatal("unknown diagnosis was accepted")
	}
	exitCode := 1
	if !MatchesRuleSelectors([]string{"cuda-gpu-memory-exhausted"}, Job{ExitCode: &exitCode, Log: "CUDA out of memory"}) {
		t.Fatal("diagnosis slug did not match current rules")
	}
	if !MatchesRuleSelectors([]string{strings.ToLower("memory exhausted")}, Job{ExitCode: &exitCode, Log: "CUDA out of memory"}) {
		t.Fatal("diagnosis name substring did not match")
	}
}
