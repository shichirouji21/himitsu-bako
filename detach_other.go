//go:build !unix

package main

import "os/exec"

// detachCmd is a no-op on non-Unix platforms; Go does not auto-reap orphans
// on Windows, so the spawned helper survives the parent exit.
func detachCmd(cmd *exec.Cmd) {}
