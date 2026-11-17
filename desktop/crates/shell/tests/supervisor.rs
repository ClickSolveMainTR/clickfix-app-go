//! Supervisor'ın gerçek süreçlerle davranışı.
//!
//! `CLICKFIX_SIDECAR_BIN` verilirse (scripts/check.sh verir) gerçek Go sidecar
//! ile uçtan uca denenir; verilmezse o test atlanır.

use std::time::Duration;

use clickfix_shell::{Backoff, CallError, Sidecar, SidecarSpec};
use serde_json::json;

fn fast(spec: SidecarSpec) -> SidecarSpec {
    SidecarSpec {
        call_timeout: Duration::from_secs(5),
        backoff: Backoff::new(Duration::from_millis(20), Duration::from_millis(80)),
        stable_after: Duration::from_secs(60),
        ..spec
    }
}

async fn wait_for<F: Fn() -> bool>(f: F, what: &str) {
    for _ in 0..200 {
        if f() {
            return;
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    panic!("beklenen olmadı: {what}");
}

#[tokio::test]
async fn missing_program_is_retried_with_backoff() {
    let sc = Sidecar::spawn(fast(SidecarSpec::new("/yok/clickfix-sidecar", [])));
    wait_for(|| sc.starts() >= 3, "en az 3 deneme").await;
    assert_eq!(sc.call("ping", json!({})).await, Err(CallError::NotRunning));
    sc.shutdown();
}

#[cfg(unix)]
#[tokio::test]
async fn crashing_process_is_restarted() {
    let sc = Sidecar::spawn(fast(SidecarSpec::new(
        "sh",
        ["-c".to_string(), "exit 3".to_string()],
    )));
    wait_for(|| sc.starts() >= 3, "çöken süreç yeniden başlatılır").await;
    sc.shutdown();
    let after = sc.starts();
    tokio::time::sleep(Duration::from_millis(300)).await;
    assert!(
        sc.starts() <= after + 1,
        "durdurulduktan sonra yeniden başlatılmamalı"
    );
}

#[tokio::test]
async fn real_sidecar_ping_version_and_restart() {
    let Ok(bin) = std::env::var("CLICKFIX_SIDECAR_BIN") else {
        eprintln!("CLICKFIX_SIDECAR_BIN yok, atlandı");
        return;
    };
    let sc = Sidecar::spawn(fast(SidecarSpec::new(bin, ["--mode=support".to_string()])));
    let mut ready = None;
    for _ in 0..200 {
        if let Ok(v) = sc.call("ping", json!({})).await {
            ready = Some(v);
            break;
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
    assert_eq!(ready, Some(json!({"pong": true})));

    let v = sc.call("version", json!({})).await.expect("version");
    assert_eq!(v["mode"], "support");
    assert!(v["version"].as_str().is_some_and(|s| !s.is_empty()));

    let err = sc
        .call("yok", json!({}))
        .await
        .expect_err("bilinmeyen yöntem");
    assert!(
        matches!(err, CallError::Rpc { code: -32601, .. }),
        "{err:?}"
    );
    sc.shutdown();
}
