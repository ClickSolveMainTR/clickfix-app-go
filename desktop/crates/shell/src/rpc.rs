//! Sidecar ile stdin/stdout satır sınırlı JSON-RPC istemcisi.
//!
//! Protokol `desktop/sidecar/internal/ipc` ile aynıdır:
//! istek `{"id","method","params"}`, yanıt `{"id","result"}` ya da
//! `{"id","error":{"code","message"}}`, olay `{"event","params"}`.

use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::sync::{broadcast, oneshot};

/// Sidecar'dan gelen uzun ömürlü bildirim.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Event {
    pub event: String,
    #[serde(default)]
    pub params: Value,
}

#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
pub enum CallError {
    #[error("sidecar hatası {code}: {message}")]
    Rpc { code: i64, message: String },
    #[error("sidecar çalışmıyor")]
    NotRunning,
    #[error("sidecar bağlantısı kapandı")]
    Closed,
    #[error("sidecar yanıt vermedi")]
    Timeout,
    #[error("sidecar'a yazılamadı: {0}")]
    Io(String),
}

#[derive(Serialize)]
struct Request<'a> {
    id: u64,
    method: &'a str,
    params: &'a Value,
}

#[derive(Deserialize)]
struct RpcErrorBody {
    code: i64,
    message: String,
}

#[derive(Deserialize)]
struct Incoming {
    #[serde(default)]
    id: Option<Value>,
    #[serde(default)]
    result: Option<Value>,
    #[serde(default)]
    error: Option<RpcErrorBody>,
    #[serde(default)]
    event: Option<String>,
    #[serde(default)]
    params: Option<Value>,
}

type Pending = Mutex<HashMap<u64, oneshot::Sender<Result<Value, CallError>>>>;
type Writer = tokio::sync::Mutex<Box<dyn AsyncWrite + Send + Unpin>>;

struct Inner {
    next_id: AtomicU64,
    pending: Pending,
    writer: Writer,
    closed: AtomicBool,
}

impl Inner {
    fn fail_all(&self) {
        self.closed.store(true, Ordering::SeqCst);
        let drained: Vec<_> = self
            .pending
            .lock()
            .map(|mut p| p.drain().collect())
            .unwrap_or_default();
        for (_, tx) in drained {
            let _ = tx.send(Err(CallError::Closed));
        }
    }
}

/// Tek bir sidecar sürecine bağlı istemci. Ucuz kopyalanır.
#[derive(Clone)]
pub struct Client {
    inner: Arc<Inner>,
}

impl Client {
    /// `writer` sidecar'ın stdin'i, `reader` stdout'udur. Okuma döngüsü arka
    /// planda çalışır; dönen tutamaç stdout kapanınca biter.
    pub fn new<W, R>(
        writer: W,
        reader: R,
        events: broadcast::Sender<Event>,
    ) -> (Self, tauri::async_runtime::JoinHandle<()>)
    where
        W: AsyncWrite + Send + Unpin + 'static,
        R: AsyncRead + Send + Unpin + 'static,
    {
        let inner = Arc::new(Inner {
            next_id: AtomicU64::new(1),
            pending: Mutex::new(HashMap::new()),
            writer: tokio::sync::Mutex::new(Box::new(writer)),
            closed: AtomicBool::new(false),
        });
        let reader_inner = inner.clone();
        let task = tauri::async_runtime::spawn(async move {
            let mut lines = BufReader::new(reader).lines();
            loop {
                match lines.next_line().await {
                    Ok(Some(line)) => handle_line(&reader_inner, &events, &line),
                    Ok(None) => break,
                    Err(e) => {
                        log::warn!("sidecar çıktısı okunamadı: {e}");
                        break;
                    }
                }
            }
            reader_inner.fail_all();
        });
        (Self { inner }, task)
    }

    /// Bir yöntemi çağırır ve yanıtı bekler. Zaman aşımı çağıranın işidir
    /// (`tokio::time::timeout`).
    pub async fn call(&self, method: &str, params: Value) -> Result<Value, CallError> {
        if self.inner.closed.load(Ordering::SeqCst) {
            return Err(CallError::Closed);
        }
        let id = self.inner.next_id.fetch_add(1, Ordering::Relaxed);
        let (tx, rx) = oneshot::channel();
        if let Ok(mut p) = self.inner.pending.lock() {
            p.insert(id, tx);
        }
        let mut line = serde_json::to_vec(&Request {
            id,
            method,
            params: &params,
        })
        .map_err(|e| CallError::Io(e.to_string()))?;
        line.push(b'\n');

        let write = async {
            let mut w = self.inner.writer.lock().await;
            w.write_all(&line).await?;
            w.flush().await
        };
        if let Err(e) = write.await {
            if let Ok(mut p) = self.inner.pending.lock() {
                p.remove(&id);
            }
            return Err(CallError::Io(e.to_string()));
        }
        // Okuma döngüsü bittiyse ve kayıt onun boşaltmasından sonra
        // eklendiyse, bekleyen kalmasın.
        if self.inner.closed.load(Ordering::SeqCst) {
            self.inner.fail_all();
        }
        rx.await.unwrap_or(Err(CallError::Closed))
    }

    /// Sidecar'ın stdout'u kapandı mı?
    pub fn is_closed(&self) -> bool {
        self.inner.closed.load(Ordering::SeqCst)
    }
}

fn handle_line(inner: &Inner, events: &broadcast::Sender<Event>, line: &str) {
    if line.trim().is_empty() {
        return;
    }
    let msg: Incoming = match serde_json::from_str(line) {
        Ok(m) => m,
        Err(e) => {
            log::warn!("sidecar'dan çözülemeyen satır: {e}");
            return;
        }
    };
    if let Some(event) = msg.event {
        let _ = events.send(Event {
            event,
            params: msg.params.unwrap_or(Value::Null),
        });
        return;
    }
    let Some(id) = msg.id.as_ref().and_then(Value::as_u64) else {
        if let Some(err) = msg.error {
            log::warn!(
                "sidecar kimliksiz hata döndürdü: {} {}",
                err.code,
                err.message
            );
        }
        return;
    };
    let tx = inner.pending.lock().ok().and_then(|mut p| p.remove(&id));
    let Some(tx) = tx else {
        log::warn!("sidecar bilinmeyen id'ye yanıt verdi: {id}");
        return;
    };
    let result = match msg.error {
        Some(e) => Err(CallError::Rpc {
            code: e.code,
            message: e.message,
        }),
        None => Ok(msg.result.unwrap_or(Value::Null)),
    };
    let _ = tx.send(result);
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use std::time::Duration;
    use tokio::io::{duplex, split};

    /// Boru çiftinin karşı ucunda sahte bir sidecar çalıştırır: gelen her
    /// isteği `respond` ile yanıtlar.
    fn fake<F>(respond: F) -> (Client, broadcast::Receiver<Event>)
    where
        F: Fn(Value) -> Option<String> + Send + 'static,
    {
        let (ours, theirs) = duplex(64 * 1024);
        let (our_r, our_w) = split(ours);
        let (their_r, mut their_w) = split(theirs);
        let (events, rx) = broadcast::channel(16);
        let (client, _) = Client::new(our_w, our_r, events);
        tauri::async_runtime::spawn(async move {
            let mut lines = BufReader::new(their_r).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                let req: Value = serde_json::from_str(&line).expect("istek JSON");
                if let Some(out) = respond(req)
                    && their_w.write_all(out.as_bytes()).await.is_err()
                {
                    break;
                }
            }
        });
        (client, rx)
    }

    #[tokio::test]
    async fn call_returns_result() {
        let (c, _) = fake(|req| {
            Some(format!(
                "{}\n",
                json!({"id": req["id"], "result": {"pong": true}})
            ))
        });
        let got = c.call("ping", json!({})).await.expect("yanıt");
        assert_eq!(got, json!({"pong": true}));
    }

    #[tokio::test]
    async fn call_returns_rpc_error() {
        let (c, _) = fake(|req| {
            Some(format!(
                "{}\n",
                json!({"id": req["id"], "error": {"code": -32601, "message": "bilinmeyen yöntem"}})
            ))
        });
        let err = c.call("yok", Value::Null).await.expect_err("hata");
        assert_eq!(
            err,
            CallError::Rpc {
                code: -32601,
                message: "bilinmeyen yöntem".into()
            }
        );
    }

    #[tokio::test]
    async fn events_and_garbage_lines() {
        let (c, mut rx) = fake(|req| {
            Some(format!(
                "bozuk satır\n\n{}\n{}\n",
                json!({"event": "ready", "params": {"mode": "support"}}),
                json!({"id": req["id"], "result": 1})
            ))
        });
        assert_eq!(c.call("ping", Value::Null).await, Ok(json!(1)));
        let ev = rx.recv().await.expect("olay");
        assert_eq!(ev.event, "ready");
        assert_eq!(ev.params, json!({"mode": "support"}));
    }

    #[tokio::test]
    async fn concurrent_calls_match_by_id() {
        // Yanıtları tersten yollamak için her isteği biriktir ve ikinci
        // istekte ikisini birden ters sırada yanıtla.
        let seen = Arc::new(Mutex::new(Vec::<Value>::new()));
        let s = seen.clone();
        let (c, _) = fake(move |req| {
            let mut v = s.lock().expect("kilit");
            v.push(req);
            if v.len() < 2 {
                return None;
            }
            let out: String = v
                .drain(..)
                .rev()
                .map(|r| format!("{}\n", json!({"id": r["id"], "result": r["params"]["n"]})))
                .collect();
            Some(out)
        });
        let (a, b) = tokio::join!(
            c.call("echo", json!({"n": "a"})),
            c.call("echo", json!({"n": "b"}))
        );
        assert_eq!(a, Ok(json!("a")));
        assert_eq!(b, Ok(json!("b")));
    }

    #[tokio::test]
    async fn closed_stream_fails_pending_and_future_calls() {
        let (ours, theirs) = duplex(1024);
        let (our_r, our_w) = split(ours);
        let (events, _) = broadcast::channel(4);
        let (c, task) = Client::new(our_w, our_r, events);
        let pending = tokio::spawn({
            let c = c.clone();
            async move { c.call("ping", Value::Null).await }
        });
        tokio::time::sleep(Duration::from_millis(50)).await;
        drop(theirs);
        task.await.expect("okuma döngüsü");
        let res = pending.await.expect("görev");
        assert!(
            matches!(res, Err(CallError::Closed | CallError::Io(_))),
            "{res:?}"
        );
        assert!(c.is_closed());
        assert_eq!(c.call("ping", Value::Null).await, Err(CallError::Closed));
    }
}
