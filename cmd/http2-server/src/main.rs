// SPDX-License-Identifier: AGPL-3.0-or-later

use axum::{
    body::Body,
    extract::Path,
    http::{header, StatusCode},
    response::{IntoResponse, Response},
    routing::get,
    Router,
};
use bytes::Bytes;
use clap::Parser;
use futures::StreamExt;
use hyper::body::Incoming;
use hyper_util::rt::{TokioExecutor, TokioIo};
use hyper_util::server::conn::auto::Builder;
use rustls::ServerConfig;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Instant;
use tokio::net::TcpListener;
use tokio_rustls::TlsAcceptor;
use tower::Service;
use tower_http::services::ServeDir;

/// Size of each zero-filled chunk for streaming (1 MiB).
const CHUNK_SIZE: usize = 1 << 20;

/// Static 1 MiB zero buffer for zero-copy streaming.
static ZEROS: [u8; CHUNK_SIZE] = [0u8; CHUNK_SIZE];

#[derive(Parser)]
#[command(about = "HTTP/2+TLS server for JavaScript performance testing")]
struct Args {
    /// Listen IP address.
    #[arg(short = 'A', long = "address", default_value = "127.0.0.1")]
    address: String,

    /// TLS certificate file.
    #[arg(long = "cert", default_value = "testdata/cert.pem")]
    cert: String,

    /// TLS private key file.
    #[arg(long = "key", default_value = "testdata/key.pem")]
    key: String,

    /// Listen port.
    #[arg(short = 'p', long = "port", default_value = "4444")]
    port: u16,

    /// Serve static files from this directory.
    #[arg(long = "static-dir", default_value = "./static/http2")]
    static_dir: String,
}

#[tokio::main]
async fn main() {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("failed to install rustls crypto provider");
    let args = Args::parse();
    serve(args).await;
}

async fn serve(args: Args) {
    let app = Router::new()
        .route("/api/:size", get(handle_get).put(handle_put))
        .fallback_service(ServeDir::new(&args.static_dir));

    // Configure h2 for maximum throughput.
    let mut builder = Builder::new(TokioExecutor::new());
    builder
        .http2()
        .initial_stream_window_size(1 << 30) // 1 GiB
        .initial_connection_window_size(1 << 30) // 1 GiB
        .max_frame_size((1 << 24) - 1); // ~16 MiB (protocol max)

    let addr: SocketAddr = format!("{}:{}", args.address, args.port)
        .parse()
        .expect("invalid address");
    let listener = TcpListener::bind(addr).await.expect("failed to bind");

    let cert_pem = std::fs::read(&args.cert).expect("failed to read cert");
    let key_pem = std::fs::read(&args.key).expect("failed to read key");

    let certs = rustls_pemfile::certs(&mut cert_pem.as_slice())
        .collect::<Result<Vec<_>, _>>()
        .expect("failed to parse cert");
    let key = rustls_pemfile::private_key(&mut key_pem.as_slice())
        .expect("failed to parse key")
        .expect("no private key found");

    let mut tls_config = ServerConfig::builder()
        .with_no_client_auth()
        .with_single_cert(certs, key)
        .expect("failed to build TLS config");
    tls_config.alpn_protocols = vec![b"h2".to_vec()];

    let tls_acceptor = TlsAcceptor::from(Arc::new(tls_config));

    eprintln!("serving h2 at https://{addr}");

    loop {
        let (tcp_stream, remote_addr) = match listener.accept().await {
            Ok(v) => v,
            Err(e) => {
                eprintln!("accept error: {e}");
                continue;
            }
        };

        let tls_acceptor = tls_acceptor.clone();
        let app = app.clone();
        let builder = builder.clone();

        tokio::spawn(async move {
            let tls_stream = match tls_acceptor.accept(tcp_stream).await {
                Ok(v) => v,
                Err(e) => {
                    eprintln!("TLS handshake error from {remote_addr}: {e}");
                    return;
                }
            };
            let alpn = tls_stream
                .get_ref()
                .1
                .alpn_protocol()
                .map(|p| String::from_utf8_lossy(p).to_string())
                .unwrap_or_else(|| "none".to_string());
            eprintln!("conn new remote={remote_addr} alpn={alpn}");
            let io = TokioIo::new(tls_stream);
            let service = hyper::service::service_fn(move |req: hyper::Request<Incoming>| {
                let mut app = app.clone();
                async move {
                    let (parts, body) = req.into_parts();
                    let req = hyper::Request::from_parts(parts, Body::new(body));
                    app.call(req).await
                }
            });
            if let Err(e) = builder.serve_connection(io, service).await {
                eprintln!("conn error remote={remote_addr}: {e}");
            }
            eprintln!("conn closed remote={remote_addr}");
        });
    }
}

/// GET /api/:size -- stream the requested number of zero bytes.
async fn handle_get(Path(size): Path<u64>) -> Response {
    if size == 0 {
        return StatusCode::BAD_REQUEST.into_response();
    }
    eprintln!("GET /api/{size}: headers received");
    let start = Instant::now();
    let stream = futures::stream::unfold(0u64, move |sent| async move {
        if sent >= size {
            let elapsed = start.elapsed();
            eprintln!(
                "GET /api/{size}: done bytes={sent} elapsed={:.3}s",
                elapsed.as_secs_f64()
            );
            return None;
        }
        let remaining = (size - sent) as usize;
        let chunk_len = remaining.min(CHUNK_SIZE);
        let chunk = Bytes::from_static(&ZEROS[..chunk_len]);
        Some((Ok::<_, std::io::Error>(chunk), sent + chunk_len as u64))
    });
    Response::builder()
        .status(StatusCode::OK)
        .header(header::CONTENT_TYPE, "application/octet-stream")
        .header(header::CONTENT_LENGTH, size)
        .body(Body::from_stream(stream))
        .unwrap()
}

/// PUT /api/:size -- accept and discard the request body.
async fn handle_put(Path(size): Path<u64>, body: Body) -> Response {
    if size == 0 {
        return StatusCode::BAD_REQUEST.into_response();
    }
    eprintln!("PUT /api/{size}: headers received");
    let start = Instant::now();
    let mut stream = body.into_data_stream();
    let mut received: u64 = 0;
    let mut remaining = size + 1;
    while let Some(Ok(chunk)) = stream.next().await {
        received += chunk.len() as u64;
        remaining = remaining.saturating_sub(chunk.len() as u64);
        if remaining == 0 {
            break;
        }
    }
    let elapsed = start.elapsed();
    eprintln!(
        "PUT /api/{size}: done bytes={received} elapsed={:.3}s",
        elapsed.as_secs_f64()
    );
    StatusCode::NO_CONTENT.into_response()
}
