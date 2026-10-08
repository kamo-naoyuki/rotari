# Implicit location defaults for aggregate commands

This note records which commands ignore implicit location defaults while
collecting results across projects. It is an implementation reference, not a
user-facing contract; user-visible behavior is summarized in the relevant
contracts.

“Implicit” means values supplied by `ROTARI_BASEDIR`, `ROTARI_PROJECT_NAME`,
or configuration defaults. An explicit CLI option still scopes the command.
The resolved default basedir used when no basedir is selected is distinct from
an implicit `ROTARI_BASEDIR` value.

| Invocation | Ignore `ROTARI_BASEDIR` / basedir config | Ignore `ROTARI_PROJECT_NAME` / project config | Scope |
| --- | --- | --- | --- |
| `basedirs`, `projects`, `runs` | Yes | Yes | List across registered state directories; explicit `--basedir` narrows the listing where supported. |
| `jobs` | Yes | Yes | Aggregate job listing; explicit basedir and positional project filters still apply. |
| `lineage` with no selector or project option | No | Yes | Search the normally resolved basedir without choosing its default project. |
| `config --list` | No | Yes | List config files in the normally resolved basedir without selecting its default project. |
| `wait` with no selector, `--run-id`, or `--project-name` | No | Yes | Wait for every active detached run in the normally resolved basedir; skip runs with an attached synchronous client. |

A selector or explicit project option changes the invocation out of its
aggregate form where applicable, so normal project selection rules apply. In
particular, bare `wait` ignores `ROTARI_PROJECT_NAME` and scans all active
projects; `wait -p PROJECT` waits for only that project. Explicit
`--basedir` continues to select the state directory for either invocation.

## Implementation and tests

`aggregateCommandInvocation` decides whether a command invocation is an
aggregate operation. `ignoredImplicitLocationDefaults` maps that decision to
the location values that the config loader must not inject. Keep the behavior
centralized there rather than adding per-command environment checks.

- Implementation: [CLI config resolution](../../../cmd/rotari/config.go#L211-L255)
- Aggregate coverage: [aggregate target defaults](../../../conformance/01-resolution/aggregate_target_defaults_test.go)
- Wait environment behavior: [wait selector conformance](../../../conformance/01-resolution/export_target_test.go)
