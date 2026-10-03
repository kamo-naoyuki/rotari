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

Distinguishing inputs from outputs would make a viewer more useful, but static
evidence rarely decides it: the same `--config results/run.yaml` form can name
either, and option names are application conventions. Instead of a role, keep the
evidence that hints at one in provenance: the accepting rule, the configuration
key or option name, and the direction of a shell redirection (`>` versus `<`).
A future viewer can present these as hints without rotari claiming a role.

Referenced scripts and configuration files are candidates on purpose. Seeing the
`train.py`, `run.sh`, or `config.yaml` a job was started with is itself useful,
even though these are usually inputs.

## Agreed scope

- Extract identifiable path references from command arguments.
- Record the job's own stdout/stderr destinations (`QueuedCommand.Output` and
  `Error`) as candidates; rotari already knows these paths exactly.
- Classify the values of the job's `Environment` entries (`KEY=value`) with the
  same classifier, using the variable name as key context.
- Inspect referenced configuration files in selected formats, initially YAML,
  JSON, and TOML, for identifiable path-valued strings.
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
- Array tasks and matrix members share one argv. A matrix member differs only by
  environment entries (`MatrixEnvironment`), and an array task by
  `ROTARI_ARRAY_TASK_ID`. Per-task output paths are therefore usually built in a
  shell or configuration from these variables, not written in argv.
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
- **PATH-X4:** for a recognized interpreter invocation at the command position
  or behind a recognized launcher, exclude the complete code operand from
  ordinary classification. A code operand is not one path even if its source
  text contains slashes or filename suffixes.
  Shell `-c` bodies are inspected only by shell inspection (section 3); bodies
  for other languages are opaque and are not inspected.

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
`.py` and `.sh` are included on purpose so the job's own script becomes a candidate
(for example `train.py` in `python train.py`); see Purpose.

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
| `python train.py --lr 0.1` | Accept `train.py`, PATH-R4 | The job's script is an intended candidate |
| `python train.py > out/log.txt` after `bash -c` | Skip as argv, PATH-X4 | Code body; shell inspection handles it |
| `OUTPUT_DIR=results` in `Environment` | Accept `results`, PATH-R5 | `_dir` key context from the variable name |

Known false positives that PATH-R3 accepts by design. Keep them in the fixture
table as accepted, so a rule change that starts or stops matching them is visible:

| Value | Actually | Note |
| --- | --- | --- |
| `meta-llama/Llama-3-8B` | Hugging Face model ID | Common in ML experiments |
| `origin/main` | Git ref | |
| `2026/10/03` | Date | Unlike `1/2`, not covered by the numeric-ratio exclusion |

If these turn out to be noisy in practice, add a narrow exclusion with fixtures.
Do not broaden PATH-X1 into a general "looks like an identifier" test.

The classifier deliberately accepts imperfect heuristics, not a claim that every
accepted string is a path. Unknown expressions that resemble paths can still be
misclassified. Keep the rules explainable, skip ambiguous unsupported forms, and
add distinguishing negative fixtures when adjusting a rule. Do not probe several
paths and use existence to make an ambiguous interpretation appear certain.

#### Recognizing code-bearing interpreter invocations (PATH-X4)

Do not search arbitrary argument values for interpreter-looking words. Start at
the command position: argv[0] for a direct command, or the command position
identified by one of the recognized launchers below. This avoids treating
`echo bash -c output.csv` or an option value such as `--engine python` as a
nested interpreter invocation. Match executable basenames case-insensitively,
so `/bin/bash` and `bash` are equivalent. Use this closed initial list of
interpreters:

| Executable basename | Code option | Options that take a separate value |
| --- | --- | --- |
| `sh`, `dash`, `bash`, `zsh` | A `c` flag anywhere in a short-option bundle (for example `-c`, `-lc`, `-ce`, `-xec`) | `-o`, `+o`, `-O`, `+O` |
| `python`, `python2`, `python3`, `python3.N`, `pypy`, `pypy3` | Exactly `-c` | `-W`, `-X` |
| `perl` | Exactly `-e` | none |
| `node` | `-e` or `--eval` | none |

For each interpreter, parse options after its command word according to that
interpreter's option grammar. Options may appear before the code option; options
that consume a separate value consume exactly that next element. Stop at the
first script/positional operand or `--`; a later `-c`/`-e` belongs to the script,
not the interpreter. Which element is the code operand differs by interpreter:

- **Shells:** `-c` is a flag meaning "read commands from the first operand".
  If a `c` flag appeared among the options, the code operand is the first
  element after option parsing ends (after `--` if present), not necessarily
  the element right after `-c`. So `bash -c -e CODE`, `bash -ce CODE`, and
  `bash -c -- CODE` all have `CODE` as the operand.
- **Python, Perl, Node:** the code option takes its value, so the code operand is
  the element right after it.

The code operand is excluded from ordinary classification. Arguments after it
remain eligible (for `bash -c CODE NAME ARG...` they are positional parameters).
Attached code operands such as `python -cCODE` are unsupported initially.

#### Recognized launchers

Walk inward only through this small explicit launcher table. Each parser must
identify the next command's exact argv boundary; do not scan the remaining words
for an interpreter name. Match launcher basenames case-insensitively. Nested
recognized launchers may be followed, with a maximum depth of four; an unknown
launcher or unsupported option form ends PATH-X4 inspection without examining
its remaining arguments as commands.

| Launcher | Initially accepted prefix before the wrapped command | Examples |
| --- | --- | --- |
| `env` | Zero or more literal `NAME=value` assignments; env options such as `-i`, `-u`, and `-S` are not supported initially | `env A=1 python -c CODE` |
| `timeout` | `--foreground`, `--preserve-status`, `--verbose`; `-s VALUE`/`--signal VALUE`/`--signal=VALUE`; `-k VALUE`/`--kill-after VALUE`/`--kill-after=VALUE`; then the required duration and wrapped command | `timeout 1h bash -c CODE`; `timeout -s TERM -k 5s 1h python -c CODE` |
| `srun` | No-option form; `--name=value` options; and the listed boolean flags `--pty`, `--unbuffered`, `--label`, `--overlap`, `--exclusive`. Separate-value options are limited to `-n`/`--ntasks`, `-N`/`--nodes`, `-G`/`--gpus`, `-c`/`--cpus-per-task`, `-p`/`--partition`, `-t`/`--time`, `-o`/`--output`, `-e`/`--error`, `-J`/`--job-name`, and `-A`/`--account` | `srun python train.py`; `srun --ntasks=2 python -c CODE`; `srun -n 2 python -c CODE` |

Examples that define the boundary:

| Argument list | Code operand | Candidates from ordinary classification |
| --- | --- | --- |
| `bash -c "python train.py > out/log.txt"` | the quoted string | none |
| `bash -l -c "..."`, `bash -o pipefail -c "..."` | the quoted string | none |
| `bash -c -e "..."`, `bash -ce "..."` | the quoted string, not `-e` | none |
| `timeout 1h bash -c "..."` | the quoted string | none |
| `env A=1 python3.12 -c "open('out/a.txt')"` | the quoted string | none |
| `srun python train.py --out results/model.pt` | none | `train.py`, `results/model.pt` |
| `srun --ntasks 2 python -c CODE` | `CODE` | none |
| `python train.py -c config.yaml` | none: `-c` follows the script | `train.py`, `config.yaml` |
| `cat bash` | none: no code option | as usual |
| `echo bash -c output.csv` | none | `output.csv` remains subject to ordinary classification |
| `python train.py --engine python -c config.yaml` | none: the inner `python` is an option value, not a command | `train.py`, `config.yaml` remain eligible |

An unsupported option or launcher form is not searched through. Its remaining
arguments stay ordinary argv values, so a code body behind it is classified as
one value and can become a false candidate. The shell body is also not inspected,
so the paths inside it are missed. This is accepted in exchange for never
suppressing a value based only on an interpreter-looking word. Keep these cases
in the fixture table as known false positives, so adding a launcher shows up as
a fixture change:

| Argument list | Result in the initial version |
| --- | --- |
| `nice bash -c "python train.py > out/log.txt"` | Whole code string accepted (PATH-R3/R4) |
| `nohup bash -c "..."` | Same |
| `uv run python -c "open('out/a.txt')"` | Same |
| `conda run -n ENV bash -c "..."` | Same |
| `srun --mpi pmix bash -c "..."` (unlisted separate-value option) | Same |

Add launcher forms only with accepted and rejected command examples. Never infer
that a string is code just because it contains shell syntax.

If the interpreter is a listed shell, pass the code operand to shell inspection
(section 3) instead of classifying it; the parser then finds literal command
arguments and PATH-R1 redirections without executing anything. For Python, Perl,
and Node, the operand is opaque and is not parsed.

Implement the recognizer once, on a command plus its literal argument words, and
call it from both argv extraction and shell inspection. Inside a shell script,
`python -c "open('out/a.txt')"` and `timeout 1h bash -c '...'` follow the same
direct-command/recognized-launcher rules as argv. Whether a shell `-c` body
found inside shell source is inspected recursively, and to what depth, is
decided with the shell parser.

#### Job log destinations (PATH-D1)

`QueuedCommand.Output` and `Error` are destinations rotari itself writes, so they
are candidates without classification, under rule PATH-D1. Resolve them on the
same base the executors use when they open them; confirm that base in each
executor (local, SSH, schedulers) in phase 0 rather than assuming the job's
working directory. Their provenance records the stream (stdout or stderr), which
is also an output-role hint.

### 2. Referenced configuration files

A command argument or environment value accepted by the classifier is inspected as
a configuration file when its extension is `.yaml`, `.yml`, `.json`, or `.toml`
(case-insensitive). The parsers already in `go.mod` (`gopkg.in/yaml.v3`,
`encoding/json`, `github.com/BurntSushi/toml`) cover these; no new dependency.

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

Some commands provide their script through a heredoc rather than a script path.
Heredocs exist only inside shell source, so this applies to a script or `-c` body
already under inspection; an argv value is never treated as a heredoc. When a
recognized shell command in that source consumes a heredoc as its stdin script
(for example, `bash <<'SH' ... SH`), inspect that body as shell source, with the
same static-only rules and provenance pointing to the heredoc body. The parser
must associate the heredoc with the command that consumes it; do not scan every
heredoc body as shell. In particular, heredocs used as data (`cat <<'EOF'`),
here-strings, and bodies passed to non-shell interpreters such as `python <<'PY'`
are not shell source and are not parsed in this phase. Do not evaluate the body
or perform shell expansion while inspecting it.

Inspect the body only when the delimiter is quoted (`<<'SH'`, `<<"SH"`, `<<\SH`).
With an unquoted delimiter, the outer shell expands `$NAME`, `$(...)`, and
backslashes in the body before the inner shell reads it, so the body text is not
the source that runs; skip it in this phase.

Choose a parser only when this phase starts; add a dependency only if necessary.
Shell inspection must not block delivery of argument/configuration discovery.

#### Variables rotari sets for the task (PATH-E1)

Without an exception, array and matrix jobs lose their most important outputs:
`> "out/$ROTARI_ARRAY_TASK_ID.log"` or `--out "results/$LR"` is an unresolved
expansion and is skipped, and every task gets the same candidates. In shell
contexts only, expand a parameter whose value rotari itself fixes for the
concrete task:

- `ROTARI_ARRAY_TASK_ID` and `ROTARI_JOB_DIR`;
- matrix variables of the member (`MatrixValue`);
- names set by the job's `Environment` entries.

Only plain `$NAME` and `${NAME}` forms are expanded. Any other parameter, an
operator form such as `${NAME:-default}`, a command substitution, or a glob in
the same word still skips the whole reference. Do not read the supervisor's or
the execution host's inherited environment: those values are not known for the
task. Record that PATH-E1 expansion was applied in provenance.

This is not evaluation of a dynamic expression: the values are part of the
prepared job specification, and the shell's expansion of these forms is fixed.
Configuration files are not covered. Whether `${NAME}` or `${oc.env:NAME}` in
YAML expands at all is up to the application, so PATH-X2 still applies there.
Ordinary argv also stays literal, because the executor passes it quoted and no
shell expands it.

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
| `cat <<'EOF'` with body containing `results.csv` | Do not inspect body as shell; it is data |
| `bash <<'SH'` with body `python train.py > result.csv` | Inspect body as shell source; retain literal path candidates |
| `bash <<SH` (unquoted delimiter) | Skip body: the outer shell expands it first |
| `python <<'PY'` with body containing `open('result.csv')` | Skip body in this phase; it is Python source |

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
- Role hints for each source, without a role decision: key or option name,
  redirection direction for PATH-R1, stream for PATH-D1, and whether PATH-E1
  expanded a variable.
- Association with the concrete job/task and attempt for execution-bound records.

Missing existence/type data means **not observed**, not **missing**. Discovery
diagnostics are separate from candidates and from the job's execution status.
Do not collect file contents as candidate metadata.

Place classification/extraction in a focused shared package, tentatively
`internal/artifact`, with explicit inputs. Keep state I/O, lifecycle wiring, and
future CLI/Web presentation outside that core. Do not duplicate rules in `add`,
run preparation, or future viewers.

Finalize the persisted schema before integration. Persist only attempt-owned
records. Queue-time candidates are a pure function of the current job definition,
so compute them when a queue view needs them instead of storing them. This keeps
discovery out of `QueuedCommand`, from which job fingerprints are computed
(`Fingerprint` in [fingerprint.go](../../internal/model/fingerprint.go)); check
which fields it hashes before adding any field there. Missing metadata in old state means discovery
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
| 0. Rules and schema | Accepted/rejected path fixtures, lifecycle placement, persistence and inspection safety decisions, log-destination base per executor | No unresolved assumption about add/run CWD, argv semantics, or remote visibility | Pending |
| 1. Argument extraction | Shared classifier; argv, environment, and log-destination extraction with provenance, role hints, and deduplication | Deterministic unit tests; nonexistent output references retained; interpreter code bodies skipped | Pending |
| 2. Configuration extraction | Bounded YAML/JSON/TOML parsing of directly referenced sources | Nested strings and provenance covered; ambiguous/dynamic values skipped | Pending |
| 3. Lifecycle and persistence | Attempt-bound resolution and storage shared by all job creation paths | Plain/array/matrix/retry/carried cases and old state covered; execution behavior unchanged | Pending |
| 4. Shell inspection | Conservative syntax-aware extraction of literals and PATH-E1 task variables | Array tasks and matrix members get distinct candidates for `$ROTARI_ARRAY_TASK_ID`/matrix-variable paths; other dynamic or ambiguous cases skipped; no execution during inspection | Deferred until phases 1-3 are validated |
| 5. Contracts and documentation | Document implemented guarantees and limitations | Representative conformance tests, contract IDs/status rows, architecture and affected guides agree | Pending |

Argument/configuration discovery can be completed without shell inspection or a
viewer. Add implementation history to this directory's `work-log.md` after
related implementation commits exist, following [development tracking rules](../README.md).

## Validation

- Table-driven classifier tests: absolute/relative paths, spaces, Unicode,
  standalone and equals-style values, duplicate references, extensionless names,
  URLs, numeric values, expressions, nonexistent paths, interpreter code bodies
  (`bash -c`/`-lc`, options and value-taking options before the code option,
  shell code operand after later flags or `--` (`bash -c -e CODE`, `-ce`),
  versioned `python3.N -c`, `perl -e`, `node --eval`, recognized `env`/`timeout`/
  `srun` launcher forms, unsupported launchers kept as known false positives, `-c` after the script such
  as `python train.py -c config.yaml`, interpreter-looking words in ordinary
  arguments such as `echo bash -c output.csv`, positional args after `-c`, and
  the same recognizer applied to words parsed from shell source), environment
  entries, and the known false positives.
- Log-destination tests: relative and absolute `Output`/`Error` for each executor,
  resolved on the same base the executor opens them on.
- Configuration tests: nested mappings/sequences, malformed input, empty sources,
  YAML aliases/tags, interpolation, separate directory/name keys, and size/depth
  limits. Confirm no custom-tag evaluation or unbounded alias expansion.
- Shell tests: literal redirections and arguments, quoted spaces, numeric target
  names, FD duplication/closure, here-documents/strings, heredoc-fed shell script
  versus heredoc data, non-shell script, and an unquoted delimiter, special sinks, variables,
  substitutions, globbing, `cd`, and non-shell source rejection. PATH-E1: each
  array task and matrix member resolves its own path; `${NAME:-x}`, unknown
  names, and inherited environment variables are skipped.
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
- Source-inspection budgets, symlink rules, and bounded diagnostic storage.
- Behavior for execution-host-only configuration/script files beyond the initial
  supervisor-visible/shared-filesystem support.

Viewing, directory traversal, file publication policy, and historical content
preservation remain explicitly outside this plan's implementation scope.
