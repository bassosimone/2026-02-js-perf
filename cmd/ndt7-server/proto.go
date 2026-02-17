// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/bassosimone/2026-02-js-perf/internal/humanize"
	"github.com/gorilla/websocket"

	"net/http"
)

const (
	// minMessageSize is the initial WebSocket message size.
	minMessageSize = 1 << 10

	// maxScaledMessageSize is the maximum message size during scaling.
	maxScaledMessageSize = 1 << 20

	// maxMessageSize is the maximum accepted message size.
	maxMessageSize = 1 << 24

	// maxRuntime is the maximum duration for a test.
	maxRuntime = 10 * time.Second

	// measureInterval is the interval between measurement reports.
	measureInterval = 250 * time.Millisecond

	// fractionForScaling controls the message-size scaling rate.
	fractionForScaling = 16

	// wsProto is the WebSocket subprotocol for ndt7.
	wsProto = "net.measurementlab.ndt.v7"
)

// emitAppInfo logs a local measurement using slog.
func emitAppInfo(start time.Time, total int64, testname string) {
	elapsed := time.Since(start).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(total) * 8 / elapsed
	}
	slog.Info(testname,
		slog.String("test", testname),
		slog.String("bytes", humanize.IEC(float64(total), "B")),
		slog.String("elapsed", time.Since(start).Truncate(time.Millisecond).String()),
		slog.String("speed", humanize.SI(speed, "bit/s")),
	)
}

// newMessage creates a prepared WebSocket binary message of the given size.
func newMessage(n int) (*websocket.PreparedMessage, error) {
	return websocket.NewPreparedMessage(websocket.BinaryMessage, make([]byte, n))
}

// sender writes binary WebSocket messages with adaptive sizing. Used by
// the server for download and by the client for upload.
func sender(ctx context.Context, conn *websocket.Conn, testname string) error {
	var total int64
	start := time.Now()
	if err := conn.SetWriteDeadline(start.Add(maxRuntime)); err != nil {
		return err
	}
	size := minMessageSize
	message, err := newMessage(size)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(measureInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := conn.WritePreparedMessage(message); err != nil {
			return err
		}
		total += int64(size)
		select {
		case <-ticker.C:
			emitAppInfo(start, total, testname)
		default:
		}
		if int64(size) >= maxScaledMessageSize || int64(size) >= (total/fractionForScaling) {
			continue
		}
		size <<= 1
		if message, err = newMessage(size); err != nil {
			return err
		}
	}
	return nil
}

// receiver reads WebSocket messages and discards binary data.
// Text messages (server-side measurements) are printed to stdout.
// Used by the client for download and by the server for upload.
func receiver(ctx context.Context, conn *websocket.Conn, testname string) error {
	var total int64
	start := time.Now()
	if err := conn.SetReadDeadline(start.Add(maxRuntime)); err != nil {
		return err
	}
	conn.SetReadLimit(maxMessageSize)
	ticker := time.NewTicker(measureInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		kind, reader, err := conn.NextReader()
		if err != nil {
			return err
		}
		if kind == websocket.TextMessage {
			data, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			total += int64(len(data))
			fmt.Printf("%s\n", string(data))
			continue
		}
		n, err := io.Copy(io.Discard, reader)
		if err != nil {
			return err
		}
		total += n
		select {
		case <-ticker.C:
			emitAppInfo(start, total, testname)
		default:
		}
	}
	return nil
}

// upgrade performs the WebSocket upgrade handshake on the server side.
func upgrade(rw http.ResponseWriter, req *http.Request) (*websocket.Conn, error) {
	if req.Header.Get("Sec-WebSocket-Protocol") != wsProto {
		rw.WriteHeader(http.StatusBadRequest)
		return nil, errors.New("missing Sec-WebSocket-Protocol header")
	}
	h := http.Header{}
	h.Add("Sec-WebSocket-Protocol", wsProto)
	u := websocket.Upgrader{
		ReadBufferSize:  maxMessageSize,
		WriteBufferSize: maxMessageSize,
	}
	return u.Upgrade(rw, req, h)
}
