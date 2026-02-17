// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/bassosimone/runtimex"
	"github.com/bassosimone/vflag"
)

func serveNDT7Main(ctx context.Context, args []string) error {
	var (
		addressFlag = "127.0.0.1"
		portFlag    = "4567"
	)

	fset := vflag.NewFlagSet("lxs serve ndt7", vflag.ExitOnError)
	fset.StringVar(&addressFlag, 'A', "address", "Use the given IP `ADDRESS`.")
	fset.AutoHelp('h', "help", "Print this help text and exit.")
	fset.StringVar(&portFlag, 'p', "port", "Use the given TCP `PORT`.")
	runtimex.PanicOnError0(fset.Parse(args))

	mustRun("go build -v ./cmd/gencert")
	mustRun("go build -v ./cmd/ndt7-server")

	mustRun("./gencert --ip-addr %s", addressFlag)
	mustRun("./ndt7-server serve -A %s -p %s", addressFlag, portFlag)

	return nil
}
