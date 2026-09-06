use std::env;
use std::error::Error;
use std::net::SocketAddr;

use ledgermeadow_financial_engine::transport::FinancialEngineService;
use ledgermeadow_financial_engine::transport::pb::financial_engine_server::FinancialEngineServer;
use tonic::transport::Server;

const MAX_GRPC_MESSAGE_BYTES: usize = 4 * 1024 * 1024;

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    let address = parse_bind_address(
        &env::var("FINANCIAL_ENGINE_ADDR").map_err(|_| "FINANCIAL_ENGINE_ADDR is required")?,
    )?;
    Server::builder()
        .add_service(
            FinancialEngineServer::new(FinancialEngineService)
                .max_decoding_message_size(MAX_GRPC_MESSAGE_BYTES)
                .max_encoding_message_size(MAX_GRPC_MESSAGE_BYTES),
        )
        .serve_with_shutdown(address, shutdown_signal())
        .await
        .map_err(|_| "financial engine server failed")?;
    Ok(())
}

fn parse_bind_address(value: &str) -> Result<SocketAddr, &'static str> {
    let invalid_address = "FINANCIAL_ENGINE_ADDR must be a literal loopback IP with a nonzero port";
    let address: SocketAddr = value.parse().map_err(|_| invalid_address)?;
    if !address.ip().is_loopback() || address.port() == 0 {
        return Err(invalid_address);
    }
    Ok(address)
}

async fn shutdown_signal() {
    if tokio::signal::ctrl_c().await.is_err() {
        std::future::pending::<()>().await;
    }
}

#[cfg(test)]
mod tests {
    use super::parse_bind_address;

    #[test]
    fn rejects_non_loopback_addresses_without_echoing_input() {
        for address in [
            "",
            "0.0.0.0:50051",
            "[::]:50051",
            "192.0.2.1:50051",
            "10.0.0.1:50051",
            "[2001:db8::1]:50051",
            "localhost:50051",
            "engine.invalid:50051",
            "dns:///127.0.0.1:50051",
            "[::ffff:127.0.0.1]:50051",
            "[::1%lo]:50051",
            "127.0.0.1:0",
            "127.0.0.1:65536",
            "127.0.0.1:not-a-port",
            "https://example.invalid/private-path?token=test",
        ] {
            assert_eq!(
                parse_bind_address(address),
                Err("FINANCIAL_ENGINE_ADDR must be a literal loopback IP with a nonzero port")
            );
        }
    }

    #[test]
    fn accepts_literal_loopback_addresses() {
        for address in ["127.0.0.1:50051", "127.0.0.2:50051", "[::1]:50051"] {
            assert!(parse_bind_address(address).is_ok());
        }
    }
}
