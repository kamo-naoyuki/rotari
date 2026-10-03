# Automatic artifact candidate discovery

Created: 2026-10-03

Status: planning only; no implementation has started.

## Purpose

Discover paths associated with a job without requiring users to declare every
file with an additional `add` option. Experiments often write result tables,
images, checkpoints, and other files outside stdout and stderr. Recording their
references is the foundation for later CLI and Web viewing.

The term **artifact candidate** means a statically discoverable file or directory
reference, not a proven output of the job. Input and output paths are deliberately
not distinguished. Discovery is best-effort and does not promise completeness.

## Agreed scope

- Extract identifiable path references from command arguments.
- Inspect referenced configuration files in selected formats, initially YAML
  and JSON, for identifiable path-valued strings.
- Add conservative shell-script inspection as a later phase of this plan.
- Record files and directories alike. A nonexistent path can remain a candidate
  with an unknown filesystem type.
- Skip values that cannot be statically identified or resolved safely; do not
  retain arbitrary dynamic expressions as artifact candidates.
- Keep discovery separate from existence checks, reading contents, and rendering.
- Do not require extra user operations for ordinary discovery.

Directory contents are **not** enumerated by this feature. Whether a future
viewer lists immediate children or recursively explores a directory, and with
what limits, will be decided in the viewing design.

## Non-goals

- CLI display commands, Web endpoints, image previews, downloads, or UI design.
- Deciding whether a file is an input, output, or actually created by the job.
- Executing commands or scripts to discover paths; evaluating shell substitutions.
- Scanning working directories, tracing filesystem calls, or comparing directory
  snapshots before and after a job.
- Artifact copying, remote transfer, retention, or immutable historical contents.
- Inferring dependencies, changing job selection, or changing execution results.
- Application-specific reconstruction of paths from separate configuration keys.
- A mandatory `--artifact` option or a new artifact declaration workflow.

## Existing integration points

- [QueuedCommand](../../internal/model/model.go) stores command arguments as
  `[]string`, working-directory settings, environment, and external log
  destinations. There is currently no artifact-candidate field.
- [Run preparation](../../internal/projectrun/execute.go) resolves an unspecified
  working directory to the run caller's directory and a relative directory
  against that directory. The directory at `add` time is not necessarily the
  execution directory.
- [Executor wrappers](../../internal/executor/wrapper.go) quote each argument
  and change to the resolved working directory. Shell syntax is only command
  syntax when passed to an actual shell, not merely because an argv value
  contains shell punctuation.
- [Job status resolution](../../internal/jobstatus/) owns result-origin fallback.
  Candidate lookup for carried results must follow the same origin rather than
  constructing a separate fallback chain.
- [State path validation](../../internal/state/paths.go) protects state identifiers.
  Artifact filesystem paths are a different type of value: they must not be
  confused with project, run, job, or attempt identifiers.

Implementation must re-read these sources and their tests before changing them.
See [architecture](../../docs/ARCHITECTURE.md) and
[contracts](../../contracts/README.md) for the current boundaries and invariants.

## Detection rules

### 1. Command arguments

Use the stored argv directly; do not join it and reinterpret the result as shell
code. Inspect standalone argument values and value portions of supported forms
such as `--option=value` and configuration overrides such as `key=value`.

Define one shared, conservative path classifier. Explicit absolute or relative
path syntax and recognized filename forms can supply evidence. An extension or
a slash alone is not universal proof. URLs, numeric values, regular expressions,
and obvious non-path values need negative cases in the classifier tests.

Do not assume an option is an output declaration because it is named `output`.
Key and option names can support classification, but input and output candidates
use the same mechanism. Bare extensionless names may be missed; accepting this
limitation is preferable to treating every argument as a path.

#### Shared rule-based classifier

Use deterministic rules, not a score or an application-specific output detector.
Record the accepting rule ID with provenance. These are initial design rules;
phase 0 must turn their examples and edge cases into executable fixtures.

First extract a value and its context. Support standalone argv values,
`--name=value`, and `key=value` by splitting at the first `=` only. For a
standalone long option followed by a value, use its name as context only for
the narrow path-key forms below; do not guess short-option arity or interpret
arbitrary embedded command strings. Do not include option tokens themselves.

Apply exclusions before ordinary positive rules:

- **PATH-X1:** skip empty strings, URLs/URI schemes, numeric values (including
  scientific notation), numeric ratios such as `1/2`, and recognized non-path
  expressions. Do not reject a value solely because it contains spaces or Unicode.
- **PATH-X2:** skip unresolved interpolation, substitutions, or glob patterns in
  configuration/shell contexts. Raw argv has literal semantics: do not perform
  expansion or automatically reject literal `$`/`*` characters as shell syntax.
  Unsupported application-specific interpolation is not evaluated.
- **PATH-X3:** exclude known special sinks such as `/dev/null`. Do not require
  an existence check to identify ordinary references or infer regular-file type.

Then accept the first matching positive rule:

| Rule | Evidence | Examples | Limitations |
| --- | --- | --- | --- |
| PATH-R1 | A literal file-opening shell redirection target | `> results`, `>> results`, `< input`, `2> errors` | Requires parsed shell syntax and a known resolution base; see below |
| PATH-R2 | Explicit absolute/relative path notation | `/work/results`, `./results`, `../results` | Not proof of existence or output role |
| PATH-R3 | A directory-containing relative reference or trailing separator | `results/metrics`, `results/` | Slash alone is not proof; exclusions take precedence |
| PATH-R4 | A basename with a recognized filename extension | `metrics.csv`, `plot.png`, `config.yaml` | A dot alone, unknown suffix, or version number is insufficient |
| PATH-R5 | A literal value under a narrowly recognized path key/long option | `output_dir: results`, `--file-path results`, `checkpoint_file=latest` | Generic `output`, `input`, and `format` names alone are insufficient |

Initial PATH-R4 extensions (case-insensitive): `.yaml`, `.yml`, `.json`, `.toml`,
`.csv`, `.tsv`, `.jsonl`, `.txt`, `.png`, `.jpg`, `.jpeg`, `.svg`, `.pdf`, `.npy`,
`.npz`, `.h5`, `.hdf5`, `.pt`, `.pth`, `.sh`, and `.py`. This is a deliberately
limited classifier list, not a list of supported viewers or configuration parsers.
Changes to it require accepted/rejected fixtures, not inference from any suffix.

For PATH-R5, normalize hyphens to underscores and compare case-insensitively.
Initially recognize exact `path`, `file`, and `dir`, and keys ending in `_path`,
`_file`, or `_dir`; use the leaf configuration key, not concatenated ancestors.
Apply this context to a string value or each string in its sequence. These names
are heuristics, not application contracts; they can still produce false positives.

Examples that must anchor the initial test table:

| Value or context | Decision | Reason |
| --- | --- | --- |
| `results` without path context | Skip | Bare extensionless name |
| `results` under `output_dir` | Accept, PATH-R5 | Narrow directory-key context |
| `png` under `output` | Skip | Output could mean a format |
| `metrics.csv` that does not exist yet | Accept, PATH-R4 | Existence is not required |
| `v1.2.3` | Skip | Not a recognized filename extension |
| `https://example.org/plot.png` | Skip, PATH-X1 | URL, despite slash and suffix |
| `s3://bucket/results` | Skip, PATH-X1 | Remote URI, not a filesystem path |
| `0.001`, `1e-3`, `1/2` | Skip, PATH-X1 | Numeric value or ratio |
| `${output_dir}/plot.png` in YAML | Skip, PATH-X2 | Unresolved interpolation |
| Literal `>` passed as ordinary argv | Skip | Not shell redirection syntax |

The classifier deliberately accepts imperfect heuristics, not a claim that every
accepted string is a path. Unknown expressions that resemble paths can still be
misclassified. Keep the rules explainable, skip ambiguous unsupported forms, and
add distinguishing negative fixtures when adjusting a rule. Do not probe several
paths and use existence to make an ambiguous interpretation appear certain.

### 2. Referenced configuration files

Parse supported configuration formats structurally and inspect string values in
nested mappings and sequences. Preserve the source file and key/index location
for each detected path. Use the same classifier as command-argument discovery.

Do not concatenate `output_dir` and `filename`, evaluate interpolation, expand
custom YAML tags, or infer framework-specific conventions. Treat dynamic values
as unsupported. Initial inspection covers directly referenced configuration
files only; recursive configuration includes are deferred.

Configuration-relative and working-directory-relative interpretation cannot be
deduced from YAML or JSON syntax. The initial generic rule uses the effective
job working directory and records that basis. Do not silently try several bases
and select whichever currently exists. Application-specific bases are future
extensions, not hidden heuristics.

### 3. Shell scripts

Inspect recognized shell invocations and directly referenced shell scripts using
syntax-aware parsing, not a broad regular expression over source text. Limit the
first slice to literal paths, literal command arguments, and literal redirection
targets whose working-directory basis is known.

Variable expansion, command substitution, glob expansion, loops, sourced files,
and control-flow-dependent `cd` are not initially resolved. When the base is
ambiguous, skip the affected reference. A script's own location is not its
execution directory. Do not inspect Python or other program source as shell.

Choose a parser only when this phase starts; add a dependency only if necessary.
Shell inspection must not block delivery of argument/configuration discovery.

#### Redirection-specific rule (PATH-R1)

Literal targets of file-opening operators are path candidates even without a
slash or recognized extension. Cover `<`, `>`, `>>`, `<>`, and `>|`, with optional
file-descriptor prefixes; shell-specific `&>`/`&>>` are included only when the
selected parser supports the invoked shell dialect. Remove syntactic quoting
through parsing while preserving the target's actual literal value.

Do not apply URL/numeric filename heuristics to a literal file-opening target:
`> 123` names a path regardless of its appearance. Still exclude known special
sinks and skip targets requiring unsupported expansion or an ambiguous base.

| Parsed shell construct | Decision |
| --- | --- |
| `> results`, `>> results`, `< input`, `2> errors` | Accept literal target, PATH-R1 |
| `> "result table"` | Accept the literal target including its space |
| `2>&1`, `<&0`, `>&-` | Skip: FD duplication or closure |
| `<<EOF`, `<<-EOF`, `<<< "text"` | Skip: here-document or here-string |
| `> "$OUTPUT"`, `> "$(choose_path)"` | Skip: unresolved expansion |
| `> >(consumer)` | Skip: process substitution |
| `> /dev/null` | Skip: known special sink |

This rule applies to parsed shell scripts and recognized shell `-c` bodies only.
An ordinary argv value containing `>` is not a redirection. Redirection establishes
a path reference, not that its filesystem object is a regular file: it could be a
FIFO or device. Discovery must not open the target to test it.

## Timing and execution context

1. **Queue definition:** argument-only discovery may produce unresolved relative
   references at `add` or edit time. Do not freeze absolute paths using the
   adding process's directory. Recompute when discovery-relevant inputs change.
2. **Attempt preparation:** bind relative references to the effective execution
   directory and inspect readable referenced configuration/script files before
   the command starts. Use the prepared job specification, including array/matrix
   expansion, rather than an unrelated caller environment.
3. **Future viewing:** check current existence, filesystem type, and readability
   when a candidate is accessed. A completion-time existence check is not a
   prerequisite for this plan. Missing files must not invalidate candidates.

Ordinary argv values are literal; rotari must not introduce environment or tilde
expansion that the executed command does not perform. Unsupported interpolation
inside configuration files or shell source is skipped.

For SSH and scheduler jobs, distinguish the execution filesystem from the
filesystem visible to the supervisor or future Web server. A resolved path can
be recorded without proving local visibility. Unavailable referenced sources
are skipped with discovery diagnostics, not reported as nonexistent remote
files. Initial source inspection can use supervisor-accessible/shared files;
execution-host inspection and transfer are deferred unless needed for that
first implementation. This limitation must be documented per executor.

## Candidate metadata and ownership

Proposed information for each candidate:

- Original literal value and its resolved filesystem path when available.
- Resolution basis: effective job working directory in the initial design.
- Provenance: command argument index, or source file plus configuration location
  or script source location. Preserve multiple sources when paths are deduplicated.
- Accepting classifier rule ID for each source, so detection remains explainable.
- Association with the concrete job/task and attempt for execution-bound records.

Missing existence/type data means **not observed**, not **missing**. Discovery
diagnostics are separate from candidates and from the job's execution status.
Do not collect file contents as candidate metadata.

Place classification/extraction in a focused shared package, tentatively
`internal/artifact`, with explicit inputs. Keep state I/O, lifecycle wiring, and
future CLI/Web presentation outside that core. Do not duplicate rules in `add`,
run preparation, or future viewers.

Finalize the persisted schema before integration. Prefer attempt-owned metadata
for execution-bound references; queue-time references, if persisted, are derived
from the current job definition. Missing metadata in old state means discovery
information is unavailable, not that the job had no associated files. Check
state compatibility and copy/import/export behavior without assuming a schema
version bump is automatically required.

Candidate metadata must not influence command identity/fingerprint matching,
scheduling, result selection, or retries. Executed attempts get their own records.
Carried results refer to the producing attempt's records. Array tasks and matrix
members must not accidentally share a mutable candidate list or resolution base.

## Safety and failure policy

- Read sources only; never execute a script, `eval`, or a configuration loader
  that runs application code.
- Define limits for source bytes, parsed nesting, candidate count, and inspection
  work. Do not recursively inspect directories or configuration includes.
- Inspect regular source files only; do not block on FIFOs, devices, or sockets.
  Define symlink handling and confinement for source inspection before integration.
- Parse errors, inaccessible sources, and inspection limits yield best-effort
  diagnostics without changing the job's exit status or preventing execution.
- Keep diagnostics bounded and do not expose source contents or secret values in
  routine logs. Restrict persisted discovery data like other job metadata.
- Detection is not permission to serve a file. Viewing must separately define
  authorization, allowed roots, symlink escape checks, and remote access.
- Recording a path is not preserving its contents. Later runs can overwrite it;
  historical artifact snapshots require a separate collection design.

## Implementation phases

| Phase | Deliverable | Exit criteria | Status |
| --- | --- | --- | --- |
| 0. Rules and schema | Accepted/rejected path fixtures, lifecycle placement, persistence and inspection safety decisions | No unresolved assumption about add/run CWD, argv semantics, or remote visibility | Pending |
| 1. Argument extraction | Shared classifier and argv extractor with provenance and deduplication | Deterministic unit tests; nonexistent output references retained | Pending |
| 2. Configuration extraction | Bounded YAML/JSON parsing of directly referenced sources | Nested strings and provenance covered; ambiguous/dynamic values skipped | Pending |
| 3. Lifecycle and persistence | Attempt-bound resolution and storage shared by all job creation paths | Plain/array/matrix/retry/carried cases and old state covered; execution behavior unchanged | Pending |
| 4. Shell inspection | Conservative literal-only syntax-aware extraction | Dynamic or ambiguous cases skipped; no execution during inspection | Deferred until phases 1-3 are validated |
| 5. Contracts and documentation | Document implemented guarantees and limitations | Representative conformance tests, contract IDs/status rows, architecture and affected guides agree | Pending |

Argument/configuration discovery can be completed without shell inspection or a
viewer. Add implementation history to this directory's `work-log.md` after
related implementation commits exist, following [development tracking rules](../README.md).

## Validation

- Table-driven classifier tests: absolute/relative paths, spaces, Unicode,
  standalone and equals-style values, duplicate references, extensionless names,
  URLs, numeric values, expressions, and nonexistent paths.
- Configuration tests: nested mappings/sequences, malformed input, empty sources,
  YAML aliases/tags, interpolation, separate directory/name keys, and size/depth
  limits. Confirm no custom-tag evaluation or unbounded alias expansion.
- Shell tests: literal redirections and arguments, quoted spaces, numeric target
  names, FD duplication/closure, here-documents/strings, special sinks, variables,
  substitutions, globbing, `cd`, and non-shell source rejection.
- Lifecycle tests: different add/run directories, explicit relative/absolute
  working directories, changed commands/configuration, arrays, matrices, retries,
  older attempts, carried origins, and unavailable remote sources.
- Safety tests: unreadable sources, directories, symlinks, FIFOs/devices, bounded
  inspection, and diagnostics that do not alter job success/failure.
- Persistence tests: old state without discovery metadata, round trips,
  copy/import/export, and derived metadata excluded from identity matching.
- Add binary/API conformance coverage when metadata becomes externally visible;
  give each new guarantee a contract ID and keep the contract-status table current.
- Run focused tests first, then relevant package tests, pre-commit on changed
  files, `scripts/check.sh --short`, and `scripts/check.sh` before completing
  implementation. Preserve full failure logs and report checks actually run.

## Remaining design decisions

- Finalize the initial classifier rules above as fixtures, including expression
  exclusions, shell dialects, and unsupported argument forms.
- Concrete metadata schema, deterministic ordering, and discovery-version policy.
- Whether queue-time derived candidates need persistence or only attempt records.
- Source-inspection budgets, symlink rules, and bounded diagnostic storage.
- Behavior for execution-host-only configuration/script files beyond the initial
  supervisor-visible/shared-filesystem support.

Viewing, directory traversal, file publication policy, and historical content
preservation remain explicitly outside this plan's implementation scope.
