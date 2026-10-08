# Scheduler integration coverage

This is an internal test-scope note, not a user guide, compatibility guarantee,
or production-image recommendation.

- Slurm and PBS are integration-tested in CI against a Slurm container and an
  OpenPBS container. Passing those tests does not certify compatibility with
  every real cluster configuration.
- LSF is covered by unit tests using fake scheduler commands. It has not yet
  been tested against a real LSF installation.
- SGE is covered by unit tests using fake commands. It can also be tested on
  demand by selecting `sge` in the
  [Scheduler integration workflow](../../../.github/workflows/scheduler-integration.yml).
  The workflow uses a digest-pinned CentOS 7 Grid Engine image last published
  in 2021. This is a compatibility smoke test, not certification for every
  Grid Engine fork or a recommendation to use that image in production.

Relevant tests include the [LSF unit tests](../../../internal/executor/lsf_test.go),
the [SGE unit tests](../../../internal/executor/sge_test.go), the
[scheduler container tests](../../../cmd/rotari/scheduler_container_test.go),
and the [container run tests](../../../cmd/rotari/scheduler_container_run_test.go).
