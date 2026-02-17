// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/bassosimone/runtimex"
	"github.com/bassosimone/vflag"
)

func serveMain(ctx context.Context, args []string) error {
	var (
		addressFlag   = "127.0.0.1"
		certFlag      = "testdata/cert.pem"
		keyFlag       = "testdata/key.pem"
		portFlag      = "4567"
		staticDirFlag = "./static"
	)

	fset := vflag.NewFlagSet("ndt7 serve", vflag.ExitOnError)
	fset.StringVar(&addressFlag, 'A', "address", "Use the given IP `ADDRESS`.")
	fset.StringVar(&certFlag, 0, "cert", "Use `FILE` as the TLS certificate.")
	fset.AutoHelp('h', "help", "Print this help text and exit.")
	fset.StringVar(&keyFlag, 0, "key", "Use `FILE` as the TLS private key.")
	fset.StringVar(&portFlag, 'p', "port", "Use the given TCP `PORT`.")
	fset.StringVar(&staticDirFlag, 0, "static-dir", "Serve static files from `DIR`.")
	runtimex.PanicOnError0(fset.Parse(args))

	mux := http.NewServeMux()
	mux.HandleFunc("/ndt/v7/download", func(rw http.ResponseWriter, req *http.Request) {
		conn, err := upgrade(rw, req)
		if err != nil {
			return
		}
		slog.Info("download", slog.String("remote", req.RemoteAddr))
		sender(req.Context(), conn, "download")
	})
	mux.HandleFunc("/ndt/v7/upload", func(rw http.ResponseWriter, req *http.Request) {
		conn, err := upgrade(rw, req)
		if err != nil {
			return
		}
		slog.Info("upload", slog.String("remote", req.RemoteAddr))
		receiver(req.Context(), conn, "upload")
	})
	mux.Handle("/", http.FileServer(http.Dir(staticDirFlag)))

	endpoint := net.JoinHostPort(addressFlag, portFlag)
	srv := &http.Server{Addr: endpoint, Handler: mux}
	go func() {
		defer srv.Close()
		<-ctx.Done()
	}()

	slog.Info("serving at", slog.String("addr", endpoint))
	err := srv.ListenAndServeTLS(certFlag, keyFlag)
	slog.Info("interrupted", slog.Any("err", err))

	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	runtimex.LogFatalOnError0(err)
	return nil
}
