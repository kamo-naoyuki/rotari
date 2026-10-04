# Automatic artifact candidate discovery

Created: 2026-10-03

Status: phases 0 to 3 and 5 implemented (`internal/artifact`,
`internal/artifactsource`, the run's `artifactRecorder`,
`jobstatus.Artifacts`, and contract RUN-9); phase 4 (shell inspection)
deferred. No CLI or Web view reads the records yet.

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
- **PATH-X4:** for a recognized interpreter invocation at argv[0] or found by
  scanning behind a supported launcher, exclude the complete code operand from
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

Look for an interpreter only at argv[0], or by scanning the arguments of a
supported launcher (see Recognized launchers below). Do not search the arguments
of any other command for interpreter-looking words. This avoids treating
`echo bash -c output.csv` or an option value such as `--engine python` as a
nested interpreter invocation. Match executable basenames case-insensitively,
so `/bin/bash` and `bash` are equivalent. Use this closed initial list of
interpreters:

| Executable basename | Code option | Required-value options before code | Optional-value options |
| --- | --- | --- | --- |
| `sh`, `dash` | `-c` in the POSIX option grammar | `-o VALUE`, `+o VALUE` | none in the portable initial grammar |
| `bash` | A `c` flag in a valid short-option bundle (`-c`, `-lc`, `-ce`, `-xec`) | `-o/+o VALUE`, `-O/+O VALUE`, `--rcfile FILE`, `--init-file FILE` | none |
| `zsh` | A `c` flag in a valid short-option bundle | `-o/+o VALUE`, `-O/+O VALUE` | none |
| `python`, `python2`, `python3`, `python3.N`, `pypy`, `pypy3` | Exactly `-c` | `-W VALUE`, `-X VALUE`, `--check-hash-based-pycs VALUE` | none |
| `perl` | `-e` or `-E` | `-I DIR`; `-M MODULE`/`-m MODULE` (separate form since Perl 5.39.8; attached forms also supported) | `-C[VALUE]`, `-Fpattern`, `-0[VALUE]`, `-i[EXT]`, `-l[VALUE]`, `-x[DIR]`; optional values are attached only |
| `node` | `-e`/`--eval` or `-p`/`--print`; `--eval=CODE` and `--print=CODE` carry code in the same token | `-r/--require MODULE`, `--import MODULE` | none; any `--name=value` is one self-contained token |

Node.js has a much larger, release-varying CLI than the other interpreters. Do
not catalog it. List only the module-loading options above, which commonly
precede `-e`, and treat any `--name=value` as one self-contained token. Other
Node options follow the unlisted-option rule below.

This is a small initial grammar, not a promise to model every option ever added
to every interpreter release. The option metadata must also recognize attached
short values (`-Wignore`, `-Xdev`, `-Idir`, `-MModule`) and equals forms for long
options (`--check-hash-based-pycs=always`, `--experimental-loader=./loader.mjs`).
For an option declared as optional-value, consume only its attached value; never
steal the next word, which may be the code operand.

**Unlisted options are boolean.** An option not in the table (for example
`python -u`, `bash --login`, `node --no-warnings`) consumes no word, so
`python -u -c CODE` is recognized without a catalog of boolean flags. If such an
option actually takes a separate value, that value is read as the first operand,
which ends option parsing before any code option; recognition stops and the
remaining words are classified as usual. A wrong guess therefore loses at most
the code-body exclusion. It goes wrong in the other direction only when the
value itself looks like a code option (`--opt -c`), which is accepted.

Use the documented option syntax as the source of truth and keep positive and
negative fixtures for each option arity. The initial reference set is POSIX
`sh`, Bash, zsh, Python 3.14 command-line documentation, the current Perl 5
`perlrun` documentation, and the Node.js CLI documentation: [POSIX sh](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/sh.html),
[Bash invocation](https://www.gnu.org/software/bash/manual/html_node/Invoking-Bash.html),
[zsh invocation](https://zsh.sourceforge.io/Doc/Release/Invocation.html),
[Python command line](https://docs.python.org/3/using/cmdline.html),
[Perl command switches](https://perldoc.perl.org/perlrun#Command-Switches), and
[Node.js CLI](https://nodejs.org/api/cli.html). Shell options differ by shell;
do not apply Bash or zsh options to `sh`/`dash` unless POSIX defines them.

Interpreter options evolve. Supporting more releases means adding documented
forms and fixtures; it does not mean consulting the executing machine's
environment or running the interpreter to ask for help.

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

Launcher handling is deliberately heuristic, not a full command-line parser.
Do **not** treat every word before an interpreter as an option: the words can
be option values or positional operands. For example, `timeout 1h bash ...`
has a positional duration, while `srun -n 4 python ...` has an option value
before the interpreter.

| Strategy | Benefit | Cost / failure mode | Decision |
| --- | --- | --- | --- |
| Treat every word between launcher and interpreter as an option | No option metadata | Positional operands such as `timeout`'s `1h` are not options; arbitrary values can be mistaken for the wrapped command | Reject |
| Fully parse every launcher option and operand | Most accurate command boundary | Requires a large, version-sensitive option catalog | Defer |
| Scan for interpreter names behind supported launchers; suppress values of a short list of free-text options | Small metadata list for plausible false matches such as `--job-name python` | An unlisted option value can still be mistaken for an interpreter; command arguments can also match | Use; best-effort |

For direct commands, identify an interpreter only at argv[0]. For a supported
launcher, scan its arguments for listed interpreter basenames without requiring
a complete launcher grammar. The scan stops at the first listed interpreter it
finds, whether or not that interpreter has a code option; it never continues
into the wrapped command's arguments. Without this,
`srun python train.py --engine bash -c x.yaml` would find `bash -c` and drop
`x.yaml`. A nested supported launcher found first is entered and scanned the
same way. Match basenames case-insensitively. Suppress a
token as an interpreter candidate only when it is the separate value of a
small, explicit set of free-text options whose values plausibly resemble
commands. Initially protect `srun --job-name`/`-J` and `srun --partition`/`-p`,
in both separate and equals forms (`--job-name python`, `--job-name=python`).
Do not build a catalog of every launcher option. Add a protected option only
when a concrete false match is demonstrated.

Do not treat positional launcher operands such as `timeout`'s duration as
options; they simply do not match an interpreter basename. `timeout` is scanned
past its fixed duration position. For `srun`, protected free-text values are
skipped and other tokens are scanned, including values of unlisted options.
Thus an unlisted option value that equals an interpreter name may be mistaken
for a command; this is accepted rather than maintaining a complete,
version-sensitive option catalog. Unknown launchers are not traversed. Nested
listed launchers may be followed up to depth four.

| Launcher | Initially accepted prefix before the wrapped command | Examples |
| --- | --- | --- |
| `env` | Scan like the other launchers; `NAME=value` assignments never match an interpreter basename. Env options are not parsed | `env A=1 python -c CODE` |
| `timeout` | Scan past launcher arguments, including the required positional duration | `timeout 1h bash -c CODE`; `timeout -s TERM -k 5s 1h python -c CODE` |
| `srun` | Scan argument tokens; skip only separate values of protected free-text options above. Other options are not exhaustively parsed | `srun -n 2 python -c CODE`; `srun --job-name python -n 2 bash -c CODE` |

When a protected free-text option is found, skip its value so an interpreter-
looking value is not treated as a command. All other launcher arguments remain
eligible for interpreter matching. This deliberately favors recall over exact
command-boundary parsing; a match can be wrong when an unprotected option value
or an argument to the wrapped command happens to look like an interpreter.

| Unsupported form | Why traversal is incomplete | Expected limitation |
| --- | --- | --- |
| `nice bash -c "python train.py > out/log.txt"` | `nice` is not listed | Inner shell is not inspected; code string may be a false path candidate |
| `nohup bash -c "..."` | `nohup` is not listed | Same |
| `uv run python -c "open('out/a.txt')"` | `uv` is not listed | Same |
| `conda run -n ENV bash -c "..."` | `conda` is not listed | Same |
| `srun --job-name python -n 2 bash -c CODE` | protected `--job-name` value is skipped | Correctly finds `bash` rather than mistaking `python` for the command |
| `srun --comment bash -c out.csv python train.py` | `--comment` is not in the protected-value list | False match: `bash` is taken as the command and `out.csv` is dropped as its code operand |
| `srun echo bash -c output.csv` | Wrapped command arguments are not parsed | Possible false match: `bash -c` may be ordinary arguments to `echo` |

Add launcher or protected-option support when its expected benefit justifies
the maintenance cost; include accepted and rejected command-line fixtures. The
initial list is intentionally useful rather than exhaustive.

Examples that define the interpreter/launcher boundary:

| Argument list | Code operand | Candidates from ordinary classification |
| --- | --- | --- |
| `bash -c "python train.py > out/log.txt"` | the quoted string | none |
| `bash -l -c "..."`, `bash -o pipefail -c "..."`, `bash --rcfile rc -c "..."` | the quoted string | none |
| `bash -c -e "..."`, `bash -ce "..."` | the quoted string, not `-e` | none |
| `timeout 1h bash -c "..."` | the quoted string | none |
| `env A=1 python3.12 -c "open('out/a.txt')"` | the quoted string | none |
| `python -W ignore -X dev --check-hash-based-pycs always -c CODE` | `CODE` | none |
| `perl -I ./lib -M Data::Dumper -e CODE` | `CODE` | `./lib` remains a PATH-R2 candidate; `Data::Dumper` is skipped |
| `node --require ./hook.js --import ./setup.mjs -p CODE` | `CODE` | `./hook.js` and `./setup.mjs` remain PATH-R2 candidates |
| `node --experimental-loader=./loader.mjs --eval CODE` | `CODE` | Equals-form option is self-contained; `./loader.mjs` remains a PATH-R2 candidate |
| `node --inspect -e CODE`, `python -u -c CODE`, `bash --login -c CODE` | `CODE` | Unlisted options are boolean; they must not consume the code option |
| `node --eval=CODE`, `node --print=CODE` | the value within the same token | The code value is excluded; do not pass it through ordinary path classification |
| `node --unlisted-option VALUE --eval "out/result.csv"` | none: `VALUE` is read as the script operand | Recognition stops; `out/result.csv` is classified as usual and may be a false candidate |
| `srun python train.py --out results/model.pt` | none | `train.py`, `results/model.pt` |
| `srun --ntasks 2 python -c CODE` | `CODE` | none |
| `python train.py -c config.yaml` | none: `-c` follows the script | `train.py`, `config.yaml` |
| `cat bash` | none: no code option | as usual |
| `echo bash -c output.csv` | none | `output.csv` remains subject to ordinary classification |
| `python train.py --engine python -c config.yaml` | none: the inner `python` is an option value, not a command | `train.py`, `config.yaml` remain eligible |

Never infer that a string is code just because it contains shell syntax.

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

## Phase 0 decisions

These settle the rule details the sections above left open. The fixtures are
in [classify_test.go](../../internal/artifact/classify_test.go) and
[discover_test.go](../../internal/artifact/discover_test.go).

- **Log-destination base.** Every executor (local, SSH, Slurm, PBS, LSF, SGE)
  changes to the job's effective working directory before `__log-forward`
  opens the `Output`/`Error` destinations with `filepath.Abs`, so PATH-D1
  resolves them on the effective working directory, like every other
  reference. When a job has no `Error` destination, stderr also goes to the
  `Output` destinations, so those sources record both streams. Special sinks
  (PATH-X3) are excluded from PATH-D1 too.
- **Path semantics.** Paths are POSIX: rotari runs jobs through POSIX shells,
  so the package uses `path`, not `path/filepath`, and reads no files.
- **Argument forms.** `key=value` is split only when the key is an identifier,
  optionally dotted and with a Hydra prefix (`+`, `++`, `~`); otherwise the
  word is one value, so `out/a=b.csv` stays whole. Words starting with a single
  `-` are option tokens and are skipped whole, including attached values such
  as `-r./hook.js`. A standalone long option passes its name as key context to
  the next word unless that word is itself an option.
- **Command words.** Recognized launcher and interpreter words
  (`/usr/bin/env`, `/bin/bash`) name programs, not artifacts, and are not
  candidates. Other command words, such as `./run.sh`, are classified.
- **Launcher depth.** At most four nested listed launchers are followed;
  beyond that, recognition stops.
- **Perl switch clusters** such as `-lne` are not split; the cluster is a
  boolean word, so the code after it is read as the script operand and
  classified as usual. Only separate `-e`/`-E` words are code options.
- **Additional PATH-X1 cases.** Values containing control characters (a
  newline in a code string) and recognized regular expressions (a leading `^`,
  or `.*`, `.+`, `\.`, `\d`, `\w`, `\s`, `(?`, `[^`) are skipped.
- **PATH-X2 in configuration/shell contexts** also covers Jinja `{{ }}`,
  Python `%(name)s`, backquotes, and a leading `~`.
- **Search-path lists.** An environment value whose variable name ends in
  `PATH` and that contains `:` (`PATH`, `PYTHONPATH`, `LD_LIBRARY_PATH`) is a
  list, not one path, and is skipped.
- **Candidate shape and order.** A candidate is a cleaned path with its
  resolution basis (`absolute`, `working_directory`, or `unresolved`) and every
  source that referenced it, in order of first reference: argv, environment,
  log destinations, then configuration. Sources keep the literal value, rule,
  index or configuration location, key context, and stream. References that
  clean to the same path are merged. One job records at most
  `MaxCandidates` (1000) candidates; the overflow is a diagnostic.

### Configuration inspection decisions (phase 2)

- `internal/artifact` parses bytes it is given (`ConfigReferences`) and asks a
  caller-supplied `SourceReader` for each referenced configuration file
  (`Discover`). `internal/artifactsource.Read` is that reader: the only file
  access of discovery.
- Only a `.yaml`/`.yml`/`.json`/`.toml` candidate referenced by a command
  argument or environment value and resolved to an absolute path is read. Log
  destinations, unresolved references, and references found inside a
  configuration file are not read.
- **Symlinks** are followed, because configuration files are often linked;
  the target must be a regular file. There is no confinement: the reader runs
  as the job's own user, and only classified path references are recorded,
  never contents. Directories, FIFOs, devices, and sockets are rejected
  without blocking (the open uses `O_NONBLOCK` and re-checks the opened file).
- **Budgets:** 1 MiB per source (`MaxSourceBytes`), nesting depth 64, and
  100,000 scalar values per source. References found before a limit are kept,
  with a diagnostic.
- Configuration strings use interpolated syntax (PATH-X2). YAML aliases and
  merge keys are not followed, custom tags and non-string scalars are skipped,
  non-string mapping keys are skipped, and each document of a multi-document
  YAML file is walked (`$1.key` locates the second document). JSON and TOML
  mapping keys are walked in sorted order; YAML keeps source order.
- Locations use `a.b[2].c`; a key containing `.`, brackets, quotes, or spaces
  is written `["key"]`. The key context of a sequence element is the key of
  its sequence.
- Diagnostics name the source and the reason (`not inspected: ...`); parse
  errors are reported only as `cannot parse YAML/JSON/TOML`, so no source
  text reaches them.

### Lifecycle and persistence decisions (phase 3)

- **Record.** Each started attempt gets `artifacts.json` in its attempt
  directory: `artifact.Record`, a `version` (`DiscoveryVersion`, now 1) plus
  the candidates and diagnostics. Older records keep the version they were
  written with and are never recomputed. Nothing is added to `QueuedCommand`,
  `JobSpec`, or `commands.json`, so fingerprints, matching, scheduling,
  selection, retries, copy, import, and export are unaffected.
- **Timing.** `artifactRecorder` (internal/projectrun/artifacts.go) runs
  discovery in the engine's `AssignAttemptID` callback, where the attempt has
  its resolved working directory and its own attempt ID, so configuration
  files are read before the attempt is submitted. It writes the record from
  the dispatcher's start callback, after the executor has created the attempt
  directory; the local executor starts the process only after that callback.
  An attempt that is cancelled before start or fails to submit gets no record.
  A retry is a new attempt with its own record.
- **Environment input.** Discovery reads the job's own environment entries:
  `add --env` values and, for a matrix member, its matrix values. Variables
  the run adds (`ROTARI_*`, `PWD`) and the caller's environment are not
  candidates.
- **Execution hosts.** Local and scheduler (Slurm, PBS, LSF, SGE) attempts
  read configuration files from the supervisor's host, which rotari already
  assumes shares the run directory with scheduler jobs; a working directory
  that is not shared is read as whatever the supervisor sees there. SSH
  attempts do not read them, because the paths name files on the SSH host; a
  diagnostic says so. Paths are still recorded for every executor.
- **Queue views.** Queue-time candidates are not stored: `artifact.FromJob`
  computes them from the current definition, with relative references
  unresolved unless the job has an absolute working directory.
- **Carried results and older attempts.** `jobstatus.Artifacts` reads the
  record of the attempt a `JobOrigin` names, following carried jobs with the
  same `locateAttempt` chain as `FilterJob`. A missing or unreadable record
  means discovery information is unavailable, not that the job had no files.
- **Failures.** A record that cannot be written is a warning in the
  supervisor log; the job's execution and result are unchanged.

## Implementation phases

| Phase | Deliverable | Exit criteria | Status |
| --- | --- | --- | --- |
| 0. Rules and schema | Accepted/rejected path fixtures, lifecycle placement, persistence and inspection safety decisions, log-destination base per executor | No unresolved assumption about add/run CWD, argv semantics, or remote visibility | Done |
| 1. Argument extraction | Shared classifier; argv, environment, and log-destination extraction with provenance, role hints, and deduplication | Deterministic unit tests; nonexistent output references retained; interpreter code bodies skipped | Done |
| 2. Configuration extraction | Bounded YAML/JSON/TOML parsing of directly referenced sources | Nested strings and provenance covered; ambiguous/dynamic values skipped | Done |
| 3. Lifecycle and persistence | Attempt-bound resolution and storage shared by all job creation paths | Plain/array/matrix/retry/carried cases and old state covered; execution behavior unchanged | Done |
| 4. Shell inspection | Conservative syntax-aware extraction of literals and PATH-E1 task variables | Array tasks and matrix members get distinct candidates for `$ROTARI_ARRAY_TASK_ID`/matrix-variable paths; other dynamic or ambiguous cases skipped; no execution during inspection | Deferred until phases 1-3 are validated |
| 5. Contracts and documentation | Document implemented guarantees and limitations | Representative conformance tests, contract IDs/status rows, architecture and affected guides agree | Done: RUN-9, `TestStartedAttemptRecordsArtifactCandidates`, docs/INSPECT.md |

Argument/configuration discovery can be completed without shell inspection or a
viewer. Add implementation history to this directory's `work-log.md` after
related implementation commits exist, following [development tracking rules](../README.md).

## Validation

- Table-driven classifier tests: absolute/relative paths, spaces, Unicode,
  standalone and equals-style values, duplicate references, extensionless names,
  URLs, numeric values, expressions, nonexistent paths, interpreter code bodies
  (`bash -c`/`-lc`, options and value-taking options before the code option,
  shell code operand after later flags or `--` (`bash -c -e CODE`, `-ce`),
  interpreter options with separate, attached, optional, and equals values
  (`bash --rcfile rc -c CODE`, `python -Xdev -c CODE`,
  `python --check-hash-based-pycs=always -c CODE`,
  `perl -I ./lib -M Data::Dumper -e CODE`,
  `node --require ./hook.js --import ./setup.mjs -p CODE`,
  `node --eval=CODE`/`--print=CODE`, unlisted options treated as boolean
  (`python -u -c CODE`, `node --inspect -e CODE`) and an unlisted option that
  actually takes a value, the launcher scan stopping at the first interpreter, `node --experimental-loader=./loader.mjs -e CODE`),
  versioned `python3.N -c`, recognized `env`/`timeout`/`srun` launcher forms,
  unsupported launchers kept as known false positives, `-c` after the script such
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
