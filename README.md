# 2026-02-js-perf

This repository investigates JavaScript client performance for network
measurement protocols, comparing different browser APIs (Fetch, XHR,
ReadableStream) across HTTP/1.1+TLS, HTTP/2+TLS, and WebSocket (ndt7).

This is a follow-up to [2026-02-http2-perf](https://github.com/bassosimone/2026-02-http2-perf),
which benchmarked server-side HTTP/2 throughput using Go and Rust clients.
Here the focus shifts to the browser: how fast can JavaScript move data
using the APIs available to it, and which strategies work best?

## Motivation

The real-world ndt8 client will be JavaScript running in a browser. Browser
network stacks (Chromium's "network service", Firefox's "necko") sit in a
separate process from the renderer where JavaScript runs. Data must cross
an IPC boundary to reach JS. Understanding which browser APIs minimize
overhead — and how HTTP/2 stream multiplexing interacts with large
transfers — is essential for protocol design.

## Setup

Build the launcher and generate TLS certificates:

```bash
go build -v ./cmd/lxs
go build -v ./cmd/gencert
./gencert --ip-addr 127.0.0.1
```

The certificate is self-signed and written to `testdata/`. You will need
to trust it in your browser (or accept the security exception) to run
the tests.

## Servers

Three servers are available, each on a different port:

| Server | Port | Protocol | Implementation |
|---|---|---|---|
| `http1-server` | 4443 | HTTP/1.1+TLS | Go `net/http`, ALPN forced to `http/1.1` |
| `http2-server` | 4444 | HTTP/2+TLS | Rust `axum`/`hyper`/`rustls`, h2 windows 1 GiB, max frame ~16 MiB |
| `ndt7-server` | 4567 | WebSocket+TLS | Go `net/http` + `gorilla/websocket`, ndt7 protocol |

Start any server using `lxs`:

```bash
./lxs serve http1
./lxs serve http2    # requires Rust toolchain
./lxs serve ndt7
```

Each server logs connection lifecycle, negotiated ALPN protocol, and
per-request bytes/elapsed time, so you can cross-check browser-reported
measurements against server-side observations.

## JavaScript strategies

### HTTP/1.1 and HTTP/2

Both the HTTP/1.1 and HTTP/2 test pages offer the same four download
and four upload strategies:

**Download:**
- **Fetch + ReadableStream** — streaming, minimal memory, progress updates
- **Fetch + Blob** — waits for full response, no progress
- **Fetch + ArrayBuffer** — waits for full response, no progress
- **XHR + Blob** — progress events via `onprogress`

**Upload:**
- **Fetch + ReadableStream** — streaming via `duplex: 'half'`, progress updates
- **Fetch + Blob** — builds blob from repeated 1 MiB chunk references (lazy memory)
- **Fetch + ArrayBuffer** — single allocation (may fail for large sizes)
- **XHR + Blob** — progress events via `upload.onprogress`

The HTTP/2 page additionally supports **parallel streams**: when the
transfer size exceeds 256 MiB, it is automatically split into multiple
concurrent HTTP/2 requests (streams) over the same TLS connection. This
avoids building a single large blob and tests HTTP/2 multiplexing. The
number of streams is configurable (auto, 1, 2, 4, 8, 16).

### ndt7 (WebSocket)

The ndt7 test page uses the official [M-Lab ndt7 JavaScript client](https://github.com/m-lab/ndt-server)
with Web Workers for download and upload. Message sizes adapt dynamically
(doubling up to ~8 MiB). Note: this server does not send counterflow
measurement messages during upload.

## Results

Measured on an Intel Core i5 laptop, using Firefox, connecting to
`127.0.0.1` (localhost). ndt7 is time-based (10 seconds); HTTP/1.1
and HTTP/2 use a fixed transfer size of 4 GB.

### Download

| Strategy | HTTP/1.1+TLS | HTTP/2+TLS (1 stream) | HTTP/2+TLS (auto streams) | ndt7 (WebSocket) |
|---|---|---|---|---|
| Fetch + ReadableStream | 7 Gbit/s | 6.8 Gbit/s | 8.5 Gbit/s | — |
| Fetch + Blob | 6 Gbit/s | 7.8 Gbit/s | FAIL | — |
| Fetch + ArrayBuffer | FAIL | FAIL | 6.2 Gbit/s | — |
| XHR + Blob | 8.8 Gbit/s | 9 Gbit/s | 9.4 Gbit/s | — |
| ndt7 Web Worker | — | — | — | 6 Gbit/s |

### Upload

| Strategy | HTTP/1.1+TLS | HTTP/2+TLS (1 stream) | HTTP/2+TLS (auto streams) | ndt7 (WebSocket) |
|---|---|---|---|---|
| Fetch + ReadableStream | FAIL | FAIL | FAIL | — |
| Fetch + Blob | 13.6 Gbit/s\* | 10.5 Gbit/s\* | 8 Gbit/s\* | — |
| Fetch + ArrayBuffer | FAIL | FAIL | 8 Gbit/s\* | — |
| XHR + Blob | 9 Gbit/s\* | 8.3 Gbit/s\* | 7 Gbit/s\* | — |
| ndt7 Web Worker | — | — | — | 8 Gbit/s |

\* Server-measured wire speed. Browser-reported speeds are much lower (3-3.5
Gbit/s) because client-side timing includes blob serialization overhead
(7-8 seconds to read 4096 blob parts into the network stack before data
flows on the wire). Additionally, XHR `upload.onprogress` reports 0 bytes
throughout over HTTP/2 — progress events are broken for h2 in Firefox.

### Key observations

1. **Browser is ~3x slower than a native client.** With a Go client, ndt7
   achieves ~20 Gbit/s (see [2026-02-http2-perf](https://github.com/bassosimone/2026-02-http2-perf)).
   Firefox reaches 6-9 Gbit/s across all protocols and strategies. The
   ceiling is the IPC boundary between Firefox's necko network thread and
   the JavaScript context (main thread or Web Worker).

2. **XHR+Blob is the fastest download strategy** at 8.8-9.4 Gbit/s across
   all protocols, beating Fetch+ReadableStream (6.8-8.5 Gbit/s). XHR's
   blob accumulation path in Firefox appears more optimized — the network
   stack buffers data without crossing the IPC boundary on every chunk,
   only delivering the final blob to JS.

3. **HTTP/1.1 and HTTP/2 single-stream perform identically for download.**
   Per-strategy speeds are within noise: 7 vs 6.8 (ReadableStream),
   6 vs 7.8 (Blob), 8.8 vs 9 (XHR). The protocol is not the bottleneck;
   the browser's JS data delivery path is.

4. **Parallel h2 streams help download ReadableStream** from 6.8 to
   8.5 Gbit/s (+25%) and enable ArrayBuffer (FAIL -> 6.2 Gbit/s). But
   parallel Fetch+Blob crashes (OOM with 16 x 256 MiB buffered blobs).

5. **Parallel h2 streams hurt upload.** Server-measured upload drops from
   10.5 to 8 Gbit/s (Fetch+Blob) and 8.3 to 7 Gbit/s (XHR+Blob). The
   h2 flow control window splits across 16 streams, and 16x the blob
   serialization overhead accumulates. Upload's one win: Fetch+ArrayBuffer
   goes from FAIL to 8 Gbit/s (each buffer is 256 MiB, under the 2 GB limit).

6. **Upload wire speed exceeds download** (server-measured). HTTP/1.1
   Fetch+Blob upload is 13.6 Gbit/s vs 8.8 Gbit/s best download. However,
   the browser cannot measure this: client-side timing includes 7-8
   seconds of blob serialization before data flows on the wire. This is a
   measurement artifact, not a protocol limitation.

7. **Two strategies are completely broken in Firefox.** Fetch+ArrayBuffer
   fails for single requests > 2 GB (download: allocation failure; upload:
   explicit 2 GB limit). Fetch+ReadableStream upload with `duplex: 'half'`
   is non-functional — the server receives only 23 bytes regardless of
   the intended size.

8. **XHR progress events are broken over HTTP/2 in Firefox.** Both
   `onprogress` (download) and `upload.onprogress` (upload) report 0 bytes
   throughout the transfer, only getting the final total on completion.
   HTTP/1.1 XHR progress works correctly.

9. **ndt7 upload (8 Gbit/s) beats ndt7 download (6 Gbit/s).** The upload
   Web Worker generates ArrayBuffers and calls `ws.send()` without
   back-pressure. Download requires every WebSocket message to cross the
   IPC boundary into the Worker. This receive-side overhead is consistent
   with observation 2 — strategies that minimize IPC crossings win.

10. **HTTP/2 with a Rust server is viable — and faster than ndt7.** From
    the browser's perspective, XHR+Blob download over h2 reaches 9-9.4
    Gbit/s vs ndt7's 6 Gbit/s (+50%). Server-measured h2 upload (10.5
    Gbit/s Fetch+Blob) also beats ndt7 upload (8 Gbit/s). The
    [2026-02-http2-perf](https://github.com/bassosimone/2026-02-http2-perf)
    benchmarks showed Go's HTTP/2 was a throughput regression vs WebSocket
    with native clients, but a Rust h2 server closes that gap. For browser
    clients — the actual ndt8 use case — HTTP/2 is not just acceptable,
    it outperforms WebSocket.

11. **These are single-run results.** Each strategy was tested once with a
    4 GB transfer (or 10 seconds for ndt7). The goal of this initial round
    is to identify trends and inform protocol design, not to produce
    statistically rigorous measurements. Results should be repeated
    multiple times with confidence intervals for any formal comparison.

## Server-side logging

All servers produce structured logs that let you verify:

1. **Connection count** — proves HTTP/2 uses a single connection for
   multiple streams, while HTTP/1.1 opens one connection per test.

2. **Negotiated protocol** — `alpn=http/1.1` vs `alpn=h2` confirms
   the correct protocol was negotiated.

3. **Bytes and elapsed time** — server-side transfer metrics to
   cross-check against browser-reported Mbps values.

Example HTTP/1.1 server output:
```
conn new remote=127.0.0.1:54321
GET count=268435456 proto=HTTP/1.1 alpn=http/1.1 remote=127.0.0.1:54321
GET done bytes=268435456 elapsed=1.234s remote=127.0.0.1:54321
conn closed remote=127.0.0.1:54321
```

Example HTTP/2 server output (parallel streams):
```
conn new remote=127.0.0.1:54321 alpn=h2
GET /api/268435456: headers received
GET /api/268435456: headers received
GET /api/268435456: done bytes=268435456 elapsed=0.567s
GET /api/268435456: done bytes=268435456 elapsed=0.589s
conn closed remote=127.0.0.1:54321
```

## Related work

- [2026-02-http2-perf](https://github.com/bassosimone/2026-02-http2-perf) —
  server-side HTTP/2 benchmarks (Go vs Rust, no browser involved)
