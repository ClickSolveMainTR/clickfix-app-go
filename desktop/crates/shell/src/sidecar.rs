//! Go sidecar sürecinin başlatılması ve izlenmesi.
//!
//! Supervisor süreci başlatır, `ping` ile doğrular ve çıkınca geri çekilmeli
//! olarak yeniden başlatır. Süreç `stable_after` kadar sağlıklı çalıştıysa
//! bekleme süresi başa döner. Sidecar stderr'i kabuk günlüğüne akar.

use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::{Duration, Instant};

use serde_json::{Value, json};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::{RwLock, broadcast, watch};

use crate::backoff::Backoff;
use crate::rpc::{CallError, Client, Event};

/// Sidecar'ın nasıl başlatılacağı.
#[derive(Debug, Clone)]
pub struct SidecarSpec {
    pub program: PathBuf,
    pub args: Vec<String>,
    /// Başlangıç `ping`'i ve her çağrı için zaman aşımı.
    pub call_timeout: Duration,
    pub backoff: Backoff,
    /// Bu kadar çalışan süreç sağlıklı sayılır ve geri çekilme sıfırlanır.
    pub stable_after: Duration,
}

impl SidecarSpec {
    /// Uygulama ikilisinin yanındaki sidecar ile `--mode=<mode>` çalıştırır.
    pub fn bundled(mode: &str) -> std::io::Result<Self> {
        Ok(Self::new(
            bundled_path(crate::SIDECAR_NAME)?,
            [format!("--mode={mode}")],
        ))
    }

    pub fn new(program: impl Into<PathBuf>, args: impl IntoIterator<Item = String>) -> Self {
        Self {
            program: program.into(),
            args: args.into_iter().collect(),
            call_timeout: Duration::from_secs(5),
            backoff: Backoff::default(),
            stable_after: Duration::from_secs(60),
        }
    }
}

/// Tauri `externalBin` ikilileri pakette (ve `tauri dev`'de hedef dizinde)
/// uygulama ikilisinin yanına, hedef üçlüsü eki olmadan kopyalanır.
pub fn bundled_path(name: &str) -> std::io::Result<PathBuf> {
    let exe = std::env::current_exe()?;
    let dir = exe
        .parent()
        .ok_or_else(|| std::io::Error::other("uygulama dizini bulunamadı"))?;
    Ok(sibling(dir, name))
}

fn sibling(dir: &Path, name: &str) -> PathBuf {
    dir.join(format!("{name}{}", std::env::consts::EXE_SUFFIX))
}

struct Inner {
    client: RwLock<Option<Client>>,
    events: broadcast::Sender<Event>,
    shutdown: watch::Sender<bool>,
    starts: AtomicU32,
    call_timeout: Duration,
}

/// Çalışan supervisor'a tutamaç. Ucuz kopyalanır.
#[derive(Clone)]
pub struct Sidecar {
    inner: Arc<Inner>,
}

impl Sidecar {
    /// Supervisor'ı arka planda başlatır.
    pub fn spawn(spec: SidecarSpec) -> Self {
        let (events, _) = broadcast::channel(64);
        let (shutdown, _) = watch::channel(false);
        let inner = Arc::new(Inner {
            client: RwLock::new(None),
            events,
            shutdown,
            starts: AtomicU32::new(0),
            call_timeout: spec.call_timeout,
        });
        tauri::async_runtime::spawn(supervise(inner.clone(), spec));
        Self { inner }
    }

    /// Sidecar'da bir yöntemi çağırır.
    pub async fn call(&self, method: &str, params: Value) -> Result<Value, CallError> {
        let client = self.inner.client.read().await.clone();
        let client = client.ok_or(CallError::NotRunning)?;
        tokio::time::timeout(self.inner.call_timeout, client.call(method, params))
            .await
            .unwrap_or(Err(CallError::Timeout))
    }

    /// Sidecar olaylarına abone olur (`ready` vb.).
    pub fn subscribe(&self) -> broadcast::Receiver<Event> {
        self.inner.events.subscribe()
    }

    /// Şimdiye kadar kaç kez başlatıldığı.
    pub fn starts(&self) -> u32 {
        self.inner.starts.load(Ordering::SeqCst)
    }

    /// Sidecar'ı durdurur ve yeniden başlatmayı keser.
    pub fn shutdown(&self) {
        let _ = self.inner.shutdown.send(true);
    }
}

async fn supervise(inner: Arc<Inner>, spec: SidecarSpec) {
    let mut backoff = spec.backoff.clone();
    let mut stop = inner.shutdown.subscribe();
    loop {
        if *stop.borrow() {
            break;
        }
        inner.starts.fetch_add(1, Ordering::SeqCst);
        let started = Instant::now();
        match start(&spec, &inner).await {
            Ok((mut child, client)) => {
                match tokio::time::timeout(spec.call_timeout, client.call("ping", json!({}))).await
                {
                    Ok(Ok(_)) => {
                        log::info!("sidecar hazır (pid {:?})", child.id());
                        *inner.client.write().await = Some(client);
                        tokio::select! {
                            status = child.wait() => log::warn!("sidecar çıktı: {status:?}"),
                            _ = stop.changed() => {
                                let _ = child.kill().await;
                                *inner.client.write().await = None;
                                break;
                            }
                        }
                    }
                    Ok(Err(e)) => {
                        log::warn!("sidecar ping hatası: {e}");
                        let _ = child.kill().await;
                    }
                    Err(_) => {
                        log::warn!("sidecar ping zaman aşımı");
                        let _ = child.kill().await;
                    }
                }
                *inner.client.write().await = None;
            }
            Err(e) => log::error!("sidecar başlatılamadı ({}): {e}", spec.program.display()),
        }
        if started.elapsed() >= spec.stable_after {
            backoff.reset();
        }
        let delay = backoff.next_delay();
        log::info!("sidecar {delay:?} sonra yeniden başlatılacak");
        tokio::select! {
            _ = tokio::time::sleep(delay) => {}
            _ = stop.changed() => break,
        }
    }
    log::info!("sidecar supervisor durdu");
}

async fn start(spec: &SidecarSpec, inner: &Inner) -> std::io::Result<(Child, Client)> {
    let mut child = Command::new(&spec.program)
        .args(&spec.args)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()?;
    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| std::io::Error::other("stdin yok"))?;
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| std::io::Error::other("stdout yok"))?;
    if let Some(stderr) = child.stderr.take() {
        tauri::async_runtime::spawn(async move {
            let mut lines = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                log::info!(target: "sidecar", "{line}");
            }
        });
    }
    let (client, _) = Client::new(stdin, stdout, inner.events.clone());
    Ok((child, client))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sibling_has_platform_suffix() {
        let p = sibling(Path::new("/opt/app"), "clickfix-sidecar");
        assert_eq!(
            p.file_name().and_then(|s| s.to_str()),
            Some(format!("clickfix-sidecar{}", std::env::consts::EXE_SUFFIX).as_str())
        );
    }
}
