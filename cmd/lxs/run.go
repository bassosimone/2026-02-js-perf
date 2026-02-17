// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/bassosimone/runtimex"
	"github.com/kballard/go-shellquote"
)

func run(format string, args ...any) error {
	cmdline := fmt.Sprintf(format, args...)
	argv, err := shellquote.Split(cmdline)
	if err != nil {
		return err
	}
	runtimex.Assert(len(argv) > 0)
	fmt.Fprintf(os.Stderr, "+ %s\n", cmdline)

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func mustRun(format string, args ...any) {
	runtimex.LogFatalOnError0(run(format, args...))
}
