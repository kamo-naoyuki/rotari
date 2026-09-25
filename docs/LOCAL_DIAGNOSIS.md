# Local rule-based diagnosis

When a failed job is finalized, rotari examines its scheduler error and output
locally and stores the result in that run's `summary.json`. It sends no log
content over the network, does not need an API key or model, and never reruns
the job.

The matcher ignores letter case, ANSI terminal color escapes, and repeated
whitespace. Error names, errno constants, and HTTP status codes must appear as
whole words, and status codes only in an HTTP or status context such as
`HTTP/1.1 503`, `HTTPError: 503`, or `status code 503`; numbers in URLs or file
names do not match. Rules are deliberately narrow: no match means rotari has no
rule-based conclusion, not that the job has no diagnosable cause.

Each recognized diagnosis cites the latest line that matches it as evidence.
Diagnoses are listed from the latest evidence to the earliest, so the first one
is usually closest to the failure; earlier entries may come from warnings the
job recovered from. The scheduler error is recorded after the job output and is
therefore listed first. The generic `Python exception` diagnosis is added only
when no more specific rule already matches the final exception line.

This is a historical, informational snapshot; it does not affect job status,
retries, dependencies, or scheduler control. Every finalized failed job records
one of three outcomes in `diagnosis_status`: `matched` with its recognized
`diagnoses`, `no_match`, or `unavailable` with the reason in `diagnosis_note`
when its output cannot be read.

The snapshot also records `diagnosis_rules`, an identifier of the rule set that
produced it. Saved results are not recomputed when rotari's rules change.
`show`, reports, and the Web UI note when a saved result came from earlier
rules; run `rotari diagnose --rules` to see the result under the current rules.

View saved diagnoses in the CLI with `rotari show --run-id RUN_ID --job-id
JOB_ID`. The Web UI always shows a `Diagnosis` button beside each job's log
button; it is enabled for finalized failed jobs with saved analysis and opens
the evidence and suggested next steps.

To check a saved job manually, run `rotari diagnose --run-id RUN_ID --job-id
JOB_ID --rules`. The `diagnose` command/API is experimental and may change in
future releases.

| Diagnosis | Recognized log signatures | Suggested next step |
| --- | --- | --- |
| CUDA/GPU memory exhausted | `cuda ... out of memory`, `torch.cuda.OutOfMemoryError`, `CUBLAS_STATUS_ALLOC_FAILED`, `CUDNN_STATUS_ALLOC_FAILED` | Reduce batch size or model memory use, select a GPU with more free memory, and check for competing GPU processes. |
| CUDA/GPU unavailable | `no CUDA-capable device`, `cuda driver version is insufficient`, `NVIDIA-SMI has failed` | Confirm a usable GPU is assigned and the NVIDIA driver and CUDA runtime are compatible. |
| Slurm memory limit exceeded | `slurmstepd: error: Detected ... oom-kill`, `exceeded job memory limit`, `State=OUT_OF_MEMORY` | Request more Slurm memory if permitted or reduce memory use; inspect `sacct` for state and peak memory. |
| Slurm time limit exceeded | Slurm messages containing `time limit`, `CANCELLED ... DUE TO TIME LIMIT`, `State=TIME_LIMIT` | Increase the Slurm time limit if permitted, or reduce runtime and checkpoint before allocation expiry. |
| PBS resource or walltime limit exceeded | `walltime ... exceeded`, `exceeded ... walltime`, `job exceeded ... resource limit` | Increase the requested PBS walltime or resource limit if permitted, or reduce runtime and resource use. |
| LSF memory limit exceeded | `TERM_MEMLIMIT` | Increase the LSF memory limit if permitted, or reduce memory use. |
| LSF run time limit exceeded | `TERM_RUNLIMIT`, `TERM_CPULIMIT` | Increase the LSF run or CPU time limit if permitted, or reduce runtime and checkpoint. |
| Scheduler submission failed | `sbatch: error:`, scheduler-error forms of `qsub:` / `bsub:` | Check the scheduler command, account/project, partition/queue, requested resources, and submission permissions. |
| Invalid scheduler resource request | `Invalid qos specification`, `Invalid partition`, `invalid account`, `invalid generic resource`, PBS/LSF invalid-resource messages | Check partition/queue, account, QOS, GPU/resource names, and requested limits. |
| Scheduler cancelled job | Slurm `CANCELLED` state or `slurmstepd ... cancelled`, PBS job-deletion messages, LSF `TERM_OWNER`, `TERM_ADMIN`, `TERM_PREEMPT` | Inspect scheduler accounting for the cancellation reason and actor; resolve policy, preemption, dependency, or administrator-cancellation conditions before retrying. |
| Host memory exhausted | `out of memory: kill process`, `oom-kill`, `memory cgroup out of memory`, `Killed process ... out of memory` | Request more host memory or reduce use; inspect scheduler memory limits and kernel OOM messages. |
| Process killed | A log line consisting of `Killed` or `Killed PID` | Inspect scheduler accounting and host logs; SIGKILL can be OOM, a scheduler limit, or explicit cancellation. |
| Kernel panic or kernel fault | `kernel panic`, `BUG: unable to handle kernel`, `Oops: NNNN`, `general protection fault`, watchdog hard/soft lockup, RCU stall | Treat the compute node as unhealthy: inspect kernel logs and scheduler node health, report it to the cluster administrator, and retry on another node if appropriate. |
| Application panic | A log line beginning `panic:` | Inspect the application stack trace and failing invariant; fix the application error before retrying. |
| Segmentation fault | `segmentation fault`, `SIGSEGV`, `signal 11` | Inspect native extensions, shared-library and driver compatibility, and a core dump or debugger backtrace if available. |
| NVIDIA GPU driver/device error | `NVRM: Xid`, `GPU has fallen off the bus` | Inspect GPU/node health and NVIDIA kernel logs; retry on another GPU/node if appropriate. |
| CUDA device-side assert | `device-side assert triggered` | Check tensor shapes, labels, and index ranges passed to CUDA kernels; rerun with synchronous CUDA error reporting if needed. |
| NCCL failure | `NCCL error`, `NCCL WARN` lines that report an error, failure, abort, or timeout, `NCCL ... unhandled system error` | Check GPU/node connectivity, NCCL configuration, network-interface selection, and distributed-rank consistency. |
| MPI runtime failure | `MPI_ABORT`, PMI errors, `mpirun`/`mpiexec` fatal/error/abort messages | Check MPI/runtime compatibility, ranks, host allocation, and launcher configuration. |
| Disk space or quota exhausted | `no space left on device`, `disk quota exceeded` | Free space or files, or use a filesystem with sufficient capacity and quota. |
| Storage device I/O error | `input/output error`, `EIO`, `blk_update_request`, `Buffer I/O error`, `I/O error, dev` | Inspect kernel and storage-service logs, then check the affected disk or network filesystem with the storage administrator before retrying writes. |
| Read-only filesystem | `read-only file system`, `EROFS` | Check mount state and write location; use a writable filesystem. |
| File or directory not found | `no such file or directory`, `FileNotFoundError`, `ENOENT` | Check the path, working directory, mounted filesystems, and whether an earlier job produced the required file. |
| Permission denied | `permission denied`, `EACCES` | Check ownership, file and directory permissions, mount options, and the account used by the job. |
| Command or executable not found | `command not found`, `executable file not found` | Check the command spelling and `PATH`, or use an absolute path and ensure the executable is installed on the execution host. |
| Shared library or ABI mismatch | `undefined symbol`, `symbol lookup error`, missing `GLIBCXX_*`/`CXXABI_*`, `cannot open shared object file`, `wrong ELF class`, `undefined reference to` | Check `ldd`, `LD_LIBRARY_PATH`, and loaded `.so` files; align compiler, libstdc++, CUDA, and Python-extension ABI versions with the runtime host. |
| Python import or module missing | `ModuleNotFoundError`, `ImportError`, `cannot import name` | Check the Python environment, `PYTHONPATH`, package installation, and version compatibility. |
| Python dependency or version conflict | `requires ... but ... is installed`, dependency-resolver conflicts, incompatible-version messages | Check package versions and the environment lockfile or requirements, then install a compatible set. |
| Python syntax or indentation error | `SyntaxError`, `IndentationError`, `TabError` | Inspect the reported source line for invalid syntax, indentation, tabs, or an incompatible Python language feature. |
| Python missing name or attribute | `NameError`, `UnboundLocalError`, `AttributeError` | Check spelling, imports, object types, and variable initialization along the failing code path. |
| Python type or value error | `TypeError`, `ValueError` | Check function arguments, input shapes and formats, and conversions at the reported traceback location. |
| Python key or index error | `KeyError`, `IndexError` | Validate dictionary keys, list or array bounds, and assumptions about input or intermediate data. |
| Python assertion failed | `AssertionError` | Inspect the failed assertion and the input or invariant it checks; preserve relevant values in the job log if needed. |
| Python memory error | `MemoryError` | Reduce in-memory data or worker concurrency, stream or batch input, and confirm the scheduler memory limit is sufficient. |
| Python recursion limit exceeded | `RecursionError`, `maximum recursion depth exceeded` | Check for unintended recursion or cycles; rewrite iteratively or adjust recursion depth only when safe. |
| Python exception | A traceback followed by a final `...Error:` or `...Exception:` line that no more specific rule matches | Save the final exception line as evidence; inspect the traceback for the failing call. |
| File descriptor limit exceeded | `too many open files`, `EMFILE` | Check file descriptor limits and close leaked descriptors; inspect `ulimit -n`. |
| Process or thread limit exceeded | `fork: retry`, `fork ... resource temporarily unavailable`, thread/process `EAGAIN` | Check process/thread limits, then reduce worker count or request a suitable scheduler limit. |
| DNS lookup failed | `temporary failure in name resolution`, `could not resolve host`, `no such host` | Check the hostname, DNS resolver configuration, and compute-node DNS access. |
| Network connection refused | `connection refused`, `ECONNREFUSED` | Check the destination host and port, service availability, and firewall rules. |
| Network connection timed out | `connection timed out`, `i/o timeout`, `ETIMEDOUT` | Check routing, firewall rules, VPN or proxy requirements, and destination load. |
| Network connection reset or closed | `connection reset by peer`, `ECONNRESET`, `broken pipe`, `unexpected EOF` | Inspect remote-service restarts or crashes and proxy/load-balancer timeouts. |
| TLS certificate or handshake failure | `certificate verify failed`, `x509: certificate`, `tls handshake timeout` | Check the system clock, certificate chain, CA configuration, and TLS-intercepting proxies. |
| HTTP authentication or authorization failed | HTTP or status-code context for `401`/`403`, `401 Unauthorized`, `403 Forbidden` | Check credentials, access tokens, scopes, and the target service's authorization policy. |
| HTTP rate limited | HTTP or status-code context for `429`, `429 Too Many Requests`, `rate limit exceeded` | Wait or apply exponential backoff, reduce request concurrency, and check quota or rate-limit policy. |
| HTTP service unavailable | HTTP or status-code context for `503`, `503 Service Unavailable`, `upstream unavailable` | Retry after a delay; if it persists, check service health, maintenance notices, and proxy/load-balancer upstreams. |
| HTTP server or bad gateway error | HTTP or status-code context for `500`/`502`, `500 Internal Server Error`, `502 Bad Gateway` | Retry transient requests and inspect target-service and proxy/load-balancer logs for the upstream failure. |
| HTTP gateway timeout | HTTP or status-code context for `504`, `504 Gateway Timeout` | Check target-service latency and proxy timeout settings; retry with backoff when safe. |
| SSH authentication or host verification failed | `permission denied (publickey)`, `host key verification failed` | Check SSH keys and permissions, `known_hosts`, and server access controls. |
