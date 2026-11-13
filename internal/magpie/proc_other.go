//go:build !windows

package magpie

import "os/exec"

func configureProcess(*exec.Cmd) {}
