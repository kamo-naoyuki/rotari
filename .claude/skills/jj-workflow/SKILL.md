---
name: jj-workflow
description: "Use when managing rotari workspaces or preparing a commit, advancing the local main bookmark, or pushing changes with Jujutsu (jj)."
---

# Jujutsu workflow for rotari

1. Before editing, inspect `jj status` and `jj workspace list`. Do not edit
   another thread's workspace or include its changes without coordination.
2. For concurrent work, use a separate `jj workspace` with an explicit base
   revision. Workspaces isolate files, but bookmarks, operations, and changes
   are shared.
3. Before committing, inspect the diff and ensure it contains only this
   thread's work. Jujutsu snapshots working-copy changes; there is no Git
   staging area. Do not amend commits unless explicitly requested.
4. Commit verified work from this thread with a descriptive `jj commit`
   message. Do not add a `Co-authored-by` trailer unless the user explicitly
   requests one.
5. Unless a separate branch or unintegrated work was requested, advance the
   local `main` bookmark to the completed revision with
   `jj bookmark set main -r <completed-revision>`. Never move it to the empty
   working-copy change. If `main` has diverged or the change includes other
   unverified work, coordinate before integrating.
6. Push only when requested. In the completion report, distinguish committed,
   integrated into local `main`, and pushed.
