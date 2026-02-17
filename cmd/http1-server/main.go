// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bassosimone/2026-02-js-perf/internal/infinite"
	"github.com/bassosimone/runtimex"
	"github.com/bassosimone/vclip"
	"github.com/bassosimone/vflag"
)

func main() {
	vclip.Main(context.Background(), vclip.CommandFunc(serveMain), os.Args[1:])
}

func serveMain(ctx context.Context, args []string) error {
	var (
		addressFlag   = "127.0.0.1"
		certFlag      = "testdata/cert.pem"
		keyFlag       = "testdata/key.pem"
		portFlag      = "4443"
		staticDirFlag = "./static/http1"
	)

	fset := vflag.NewFlagSet("http1-server", vflag.ExitOnError)
	fset.StringVar(&addressFlag, 'A', "address", "Use the given IP `ADDRESS`.")
	fset.StringVar(&certFlag, 0, "cert", "Use `FILE` as the TLS certificate.")
	fset.AutoHelp('h', "help", "Print this help text and exit.")
	fset.StringVar(&keyFlag, 0, "key", "Use `FILE` as the TLS private key.")
	fset.StringVar(&portFlag, 'p', "port", "Use the given TCP `PORT`.")
	fset.StringVar(&staticDirFlag, 0, "static-dir", "Serve static files from `DIR`.")
	runtimex.PanicOnError0(fset.Parse(args))

	mux := http.NewServeMux()
	mux.Handle("GET /api/{size}", http.HandlerFunc(handleGet))
	mux.Handle("PUT /api/{size}", http.HandlerFunc(handlePut))
	mux.Handle("/", http.FileServer(http.Dir(staticDirFlag)))

	endpoint := net.JoinHostPort(addressFlag, portFlag)
	srv := &http.Server{
		Addr:    endpoint,
		Handler: mux,
		TLSConfig: &tls.Config{
			NextProtos: []string{"http/1.1"},
		},
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				slog.Info("conn new", slog.String("remote", conn.RemoteAddr().String()))
			case http.StateClosed:
				slog.Info("conn closed", slog.String("remote", conn.RemoteAddr().String()))
			}
		},
	}
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

func tlsALPN(req *http.Request) string {
	if req.TLS != nil {
		return req.TLS.NegotiatedProtocol
	}
	return ""
}

func handleGet(rw http.ResponseWriter, req *http.Request) {
	count, err := strconv.ParseInt(req.PathValue("size"), 10, 64)
	if err != nil || count < 0 {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	slog.Info("GET",
		slog.Int64("count", count),
		slog.String("proto", req.Proto),
		slog.String("alpn", tlsALPN(req)),
		slog.String("remote", req.RemoteAddr),
	)
	t0 := time.Now()
	bodyReader := io.LimitReader(infinite.Reader{}, count)
	rw.Header().Set("Content-Length", strconv.FormatInt(count, 10))
	rw.WriteHeader(http.StatusOK)
	buf := make([]byte, 1<<20) // 1 MiB
	written, _ := io.CopyBuffer(rw, bodyReader, buf)
	elapsed := time.Since(t0)
	slog.Info("GET done",
		slog.Int64("bytes", written),
		slog.Duration("elapsed", elapsed),
		slog.String("remote", req.RemoteAddr),
	)
}

func handlePut(rw http.ResponseWriter, req *http.Request) {
	expectCount, err := strconv.ParseInt(req.PathValue("size"), 10, 64)
	if err != nil || expectCount < 0 {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	slog.Info("PUT",
		slog.Int64("expectCount", expectCount),
		slog.String("proto", req.Proto),
		slog.String("alpn", tlsALPN(req)),
		slog.String("remote", req.RemoteAddr),
	)
	t0 := time.Now()
	bodyReader := io.LimitReader(req.Body, expectCount)
	buf := make([]byte, 1<<20) // 1 MiB
	read, _ := io.CopyBuffer(io.Discard, bodyReader, buf)
	elapsed := time.Since(t0)
	slog.Info("PUT done",
		slog.Int64("bytes", read),
		slog.Duration("elapsed", elapsed),
		slog.String("remote", req.RemoteAddr),
	)
	rw.WriteHeader(http.StatusNoContent)
}
