package diagnose

import "testing"

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
