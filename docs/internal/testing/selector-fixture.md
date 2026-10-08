# Selector conformance fixture

`newSelectorFixture` in
[conformance/06-selectors/selector_fixture_test.go](../../../conformance/06-selectors/selector_fixture_test.go)
builds, with the binary, a base directory with project `sweep` (a plain job, a
matrix with a retried failure, an array with a failed task, and an unnamed job,
each in its own stage; runs `first` and `second`), project `other` (a job that
shares the name `prep`; a run also named `first`), and a second base directory
reachable only through the run registry. Symbolic keys map to the generated
run, job, and attempt IDs. The environment has no location variables, so
commands find a base directory only through `-b` or the run registry.
`TestSelectorFixtureLayout` checks the layout.

The job control rows add run `live` of project `sweep`, run in the background,
with an array job `hold` of two tasks and a job `idle`, each sleeping
(`startJobControlRun` in
[conformance/06-selectors/job_control_test.go](../../../conformance/06-selectors/job_control_test.go)).
The positional rows that need an active run use a run `live` with one sleeping
job, and those that need an interrupted one kill its supervisor and job
(`setUp` in
[conformance/06-selectors/positional_test.go](../../../conformance/06-selectors/positional_test.go)).
