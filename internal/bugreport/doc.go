// Package bugreport collects the evidence a rotari bug report needs: the
// build, the host and filesystem, configuration, the supervisor log, and the
// state files of a run, with secrets removed. It reads state and runs only
// read-only scheduler commands.
package bugreport
