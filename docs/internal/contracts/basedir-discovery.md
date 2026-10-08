# Basedir discovery registry: implementation note

This is an internal indexing decision, not a user-facing contract. Do not add
SQLite for registry metadata. Keep the filesystem state authoritative and
maintain a separate one-record-per-basedir index under the master directory:

```text
<masterdir>/basedirs/<hash>.json -> { base_dir }
```

`add` and `copy` register through `queueops.Editor.RegisterBaseDir`, import
through `workflowstate.Import`; `TestCommandsThatCreateAProjectRegisterItsBasedir`
checks the command paths. The basedir registry lets `projects` and `basedirs`
discover locations without scanning every historical run record. Registration,
legacy-state discovery, and deletion behavior are specified in the
[resolution and configuration contracts](../../../contracts/01-resolution-and-config.md).

The run registry remains the run-ID lookup index:

```text
<masterdir>/runs/<run-id>.json -> { base_dir, project_name, run_id }
```

The run registry remains responsible for run-ID resolution; run-registry orphan
GC is separate because stale run entries can interfere with that resolution.
