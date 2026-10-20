# Plan: Discoverable job filters for `rotari jobs`

Created: 2026-10-20
Status: Implemented; validation complete

## Goal

Make job filters discoverable through help and shell completion, then let
`rotari jobs` use the same filter vocabulary as `show`.

Today, `--since 0` excludes finished jobs from the listing window, but
unfinished rows can also be pending, waiting, suspended, or unknown. It is
therefore not an exact running-state selector.

The generic `--filter-result VALUE` also hides its accepted values behind an
argument. A user can discover `--filter-result` in completion, but not
`failed`, `success`, or `unfinished` without consulting help. Named,
argument-free options make each choice visible in completion.

## Proposed behavior

- Replace the user-facing generic result selector with named flags:
  `--filter-failed`, `--filter-unfinished`, and `--filter-success`. These use
  the existing result-selection semantics (`failed`, `unfinished`, `success`),
  including how cancellation, blocking, and accepted results are classified.
- Remove the old `--filter-result VALUE` form. Keep `--failed`,
  `--unfinished`, and `--success` as short aliases for the named result flags.
- Add named state flags for `jobs`' display states, including
  `--filter-running`, `--filter-waiting`, `--filter-suspended`,
  `--filter-pending`, and `--filter-unknown`. `--filter-running` is the direct,
  discoverable way to list running jobs.
- `jobs` supports both the named result flags and named state flags. For
  example, `--filter-unfinished --filter-running` lists unfinished jobs whose
  displayed state is running, while `--filter-failed` lists recent failures.
- Result flags are alternatives to one another; state flags are alternatives
  to one another. Supplying both a result and a state flag narrows the output
  by both dimensions.
- Ignore presentation annotations when matching states:
  `running (recorded)` matches `--filter-running`. Result annotations such as
  carried or accepted remain governed by result-selection behavior.
- Apply filters after normal job collection. Keep `--since` as an independent
  finished-history window; active unfinished rows remain subject to the
  existing collection behavior.
- Apply the same selected rows to text and `--json`. When no rows match,
  report the active filters instead of implying that the general listing had
  results.
- Register every named selector as an option in CLI metadata so help, schema,
  and shell completion expose the choices without requiring a value.
- Add short aliases (`--running`, `--waiting`, `--suspended`, `--pending`,
  `--unknown`) for the corresponding state filters.

Example:

```sh
rotari jobs --filter-running
rotari jobs --filter-failed
rotari jobs --filter-unfinished --filter-running
```

## Scope

In scope:

- Shared result-filter parsing and CLI metadata for commands that currently
  accept `--filter-result`, plus named state filters for `jobs`.
- The generated command schema/help and shell completion.
- Focused package tests and a conformance test through the built binary.
- The generated CLI reference, the running guide, and the CLI contract/status
  table.
- Relevant schema golden updates if the command schema output changes.

Out of scope:

- Adding host, command, timestamp, project, or other predicates to `jobs`.
- Changing `--since`, default history retention, job-status resolution, or
  Web/API job listings.

## Implementation outline

1. Replace generic result options and aliases with named result flags across
   commands that currently support result selection.
2. Register named nonterminal state flags for `jobs`.
3. Filter the already-collected `joblist.Row` values without reimplementing
    result or status resolution.
4. Update empty-result output and help text; preserve the existing output and
   JSON shape when no filter is given.
5. Add tests for named result and state flags, exact running matching,
    annotations, combining result and state filters, and filtered JSON/text.
6. Add a conformance case and a CLI contract note for the behavior.
7. Update the generated CLI reference and user guide.

## Validation

From the repository root:

1. Run focused `cmd/rotari` tests for `jobs` filtering.
2. Run `go test ./cmd/rotari`.
3. Run the relevant conformance tests, including the new `jobs` filter case.
4. Run `go test ./conformance` and the repository's required check script for
   CLI behavior changes.
5. Inspect the final diff and confirm the generated CLI schema/reference are
   synchronized.

## Commit

After approval, implementation, and validation, commit this work as a
standalone item with the repository's required co-author trailer.
