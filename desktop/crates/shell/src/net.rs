//! Uzak köke erişilebilirlik denetimi (yerel hata sayfası için).

use std::net::{TcpStream, ToSocketAddrs};
use std::time::Duration;

use url::Url;

/// `url`'nin ana makinesine TCP bağlantısı kurulabiliyor mu? Engelleyicidir;
/// async koddan `spawn_blocking` ile çağrılır.
pub fn reachable(url: &Url, timeout: Duration) -> Result<(), String> {
    let host = url.host_str().ok_or("URL'de ana makine yok")?;
    let port = url.port_or_known_default().ok_or("URL'de port yok")?;
    let addrs = (host.trim_matches(['[', ']']), port)
        .to_socket_addrs()
        .map_err(|e| format!("{host} çözülemedi: {e}"))?;
    let mut last = format!("{host} için adres yok");
    for addr in addrs {
        match TcpStream::connect_timeout(&addr, timeout) {
            Ok(_) => return Ok(()),
            Err(e) => last = format!("{addr} bağlantısı kurulamadı: {e}"),
        }
    }
    Err(last)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::TcpListener;

    #[test]
    fn reachable_local_listener() {
        let l = TcpListener::bind("127.0.0.1:0").expect("bind");
        let port = l.local_addr().expect("addr").port();
        let url = Url::parse(&format!("http://127.0.0.1:{port}/")).expect("url");
        assert!(reachable(&url, Duration::from_secs(2)).is_ok());
    }

    #[test]
    fn unreachable_closed_port() {
        // Bağlanıp kapatılan port artık dinlemez.
        let port = {
            let l = TcpListener::bind("127.0.0.1:0").expect("bind");
            l.local_addr().expect("addr").port()
        };
        let url = Url::parse(&format!("http://127.0.0.1:{port}/")).expect("url");
        assert!(reachable(&url, Duration::from_secs(2)).is_err());
    }
}
