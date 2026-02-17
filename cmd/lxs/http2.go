// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/bassosimone/runtimex"
	"github.com/bassosimone/vflag"
)

func serveHTTP2Main(ctx context.Context, args []string) error {
	var (
		addressFlag = "127.0.0.1"
		portFlag    = "4444"
	)

	fset := vflag.NewFlagSet("lxs serve http2", vflag.ExitOnError)
	fset.StringVar(&addressFlag, 'A', "address", "Use the given IP `ADDRESS`.")
	fset.AutoHelp('h', "help", "Print this help text and exit.")
	fset.StringVar(&portFlag, 'p', "port", "Use the given TCP `PORT`.")
	runtimex.PanicOnError0(fset.Parse(args))

	mustRun("go build -v ./cmd/gencert")
	mustRun("cargo build --release --manifest-path cmd/http2-server/Cargo.toml")
	mustRun("cp cmd/http2-server/target/release/http2-server .")

	mustRun("./gencert --ip-addr %s", addressFlag)
	mustRun("./http2-server -A %s -p %s", addressFlag, portFlag)

	return nil
}
