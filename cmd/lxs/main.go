// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"

	"github.com/bassosimone/vclip"
	"github.com/bassosimone/vflag"
)

func main() {
	serveDisp := vclip.NewDispatcherCommand("lxs serve", vflag.ExitOnError)
	serveDisp.AddCommand("http1", vclip.CommandFunc(serveHTTP1Main), "Run HTTP/1.1+TLS service.")
	serveDisp.AddCommand("ndt7", vclip.CommandFunc(serveNDT7Main), "Run ndt7 service.")

	disp := vclip.NewDispatcherCommand("lxs", vflag.ExitOnError)
	disp.AddCommand("serve", serveDisp, "Run servers.")

	vclip.Main(context.Background(), disp, os.Args[1:])
}
