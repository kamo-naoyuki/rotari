# Technology rationale

- Rotari uses Go because it is primarily a command-line and background-server
  tool coordinating OS processes, files, locks, signals, pipes, and external
  schedulers.
- A statically linked Go binary keeps installation and deployment simple on
  login nodes, worker nodes, and shared HPC environments. The core does not
  require a language runtime, daemon framework, or database service at runtime.
- Go's standard library provides the required filesystem, process, signal,
  networking, JSON, and concurrency primitives directly. This keeps the
  file-backed state model explicit and lets local and server execution paths
  share the same implementation.
- Goroutines and channels fit the execution model: multiple jobs may run
  concurrently, while locks and a single supervisor coordinate access to each
  project.
- Python is intentionally limited to the optional client interface. It wraps
  the installed CLI rather than reimplementing queue, persistence, or execution
  semantics, leaving one authoritative core implementation.
- This choice does not require Go for job commands or scheduler integrations.
  Jobs may use any executable, and executor-specific behavior stays behind the
  `JobExecutor` boundary.
