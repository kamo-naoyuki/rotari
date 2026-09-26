// Package project owns a project's run state and its idle queue edits.
//
// It classifies a project as idle, running, or interrupted from running.lock
// and meta.json, checks that those files agree with the run directory, and
// explains an interrupted run to the user. It also owns the sequence every
// idle edit follows: take the state lock, require an idle project, load the
// queue, apply the edit, and write metadata before the queue.
//
// What an edit does to the queue belongs to the caller or internal/queueedit;
// running a queue belongs to internal/projectrun.
package project
