// CF-235 aday C: scap ile ekran yakala, LiveKit Rust SDK ile yayınla.
// Sadece keşif (spike) kodu; ürün koduna girmez.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use livekit::options::{TrackPublishOptions, VideoCodec};
use livekit::prelude::*;
use livekit::webrtc::native::yuv_helper;
use livekit::webrtc::prelude::{
    I420Buffer, RtcVideoSource, VideoFrame, VideoResolution, VideoRotation,
};
use livekit::webrtc::video_source::native::NativeVideoSource;
use scap::capturer::{Capturer, Options, Resolution};
use scap::frame::{Frame, FrameType, VideoFrame as ScapVideoFrame};
use serde::Deserialize;

const IDENTITY: &str = "customer-rust";

struct Args {
    res: Resolution,
    res_label: String,
    fps: u32,
    harness: String,
    check: bool,
}

fn usage() -> ! {
    println!(
        "Kullanım: capture-rust [--res 1080|2160|native] [--fps 30] \
         [--harness http://127.0.0.1:7890] [--check]\n\
         --check  scap destek/izin durumunu yazar, izin İSTEMEZ, çıkar."
    );
    std::process::exit(0);
}

fn parse_args() -> Args {
    let mut a = Args {
        res: Resolution::_1080p,
        res_label: "1080".into(),
        fps: 30,
        harness: "http://127.0.0.1:7890".into(),
        check: false,
    };
    let mut it = std::env::args().skip(1);
    while let Some(arg) = it.next() {
        match arg.as_str() {
            "--res" => {
                let v = it.next().unwrap_or_default();
                a.res = match v.as_str() {
                    "1080" => Resolution::_1080p,
                    "2160" => Resolution::_2160p,
                    "native" => Resolution::Captured,
                    _ => {
                        eprintln!("Geçersiz --res: {v} (1080|2160|native)");
                        std::process::exit(2);
                    }
                };
                a.res_label = v;
            }
            "--fps" => {
                a.fps = it.next().and_then(|v| v.parse().ok()).unwrap_or_else(|| {
                    eprintln!("Geçersiz --fps");
                    std::process::exit(2);
                })
            }
            "--harness" => a.harness = it.next().unwrap_or_else(|| usage()),
            "--check" => a.check = true,
            "-h" | "--help" => usage(),
            other => {
                eprintln!("Bilinmeyen argüman: {other}");
                usage();
            }
        }
    }
    a
}

fn capture_options(a: &Args) -> Options {
    Options {
        fps: a.fps,
        target: None, // None = ana ekran
        show_cursor: true,
        show_highlight: false,
        excluded_targets: None,
        output_type: FrameType::BGRAFrame,
        output_resolution: a.res,
        ..Default::default()
    }
}

// --check: sadece durum yazar. has_permission() bir "preflight"tır, istem açmaz.
fn run_check(a: &Args) {
    let supported = scap::is_supported();
    let permission = scap::has_permission();
    println!("scap::is_supported   = {supported}");
    println!("scap::has_permission = {permission}");
    if supported {
        // Bu çağrılar CoreGraphics kullanır, ScreenCaptureKit'e dokunmaz (istem yok).
        let d = scap::get_main_display();
        println!("ana ekran            = id {} \"{}\"", d.id, d.title);
        let size = scap::capturer::get_output_frame_size(&capture_options(a));
        println!(
            "çıkış boyutu (--res {}) = {}x{} @ {} fps",
            a.res_label, size[0], size[1], a.fps
        );
    }
    if !permission {
        println!("Not: izin yok. Normal çalıştırmada izin istenir ve program çıkar.");
    }
}

#[derive(Deserialize)]
struct TokenResp {
    url: String,
    token: String,
    room: String,
}

fn fetch_token(harness: &str) -> Result<TokenResp, Box<dyn std::error::Error>> {
    let url = format!("{}/token", harness.trim_end_matches('/'));
    let resp = ureq::get(&url)
        .query("identity", IDENTITY)
        .query("publish", "1")
        .call()?
        .body_mut()
        .read_json::<TokenResp>()?;
    Ok(resp)
}

#[derive(Default)]
struct Stats {
    captured: AtomicU64,
    sent: AtomicU64,
    empty: AtomicU64,
}

fn now_us() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_micros() as i64)
        .unwrap_or(0)
}

// Yakalama döngüsü: ayrı thread'de çalışır (get_next_frame engelleyici).
fn capture_loop(
    opts: Options,
    source: NativeVideoSource,
    stats: Arc<Stats>,
    stop: Arc<AtomicBool>,
) {
    let mut capturer = match Capturer::build(opts) {
        Ok(c) => c,
        Err(e) => {
            eprintln!("Capturer kurulamadı: {e}");
            stop.store(true, Ordering::SeqCst);
            return;
        }
    };
    capturer.start_capture();

    while !stop.load(Ordering::Relaxed) {
        let frame = match capturer.get_next_frame() {
            Ok(f) => f,
            Err(_) => break, // kanal kapandı
        };
        let Frame::Video(ScapVideoFrame::BGRA(f)) = frame else {
            continue;
        };
        // scap, ekran değişmediğinde 0x0 boş kare gönderir; sayıp geçiyoruz.
        if f.width == 0 || f.height == 0 {
            stats.empty.fetch_add(1, Ordering::Relaxed);
            continue;
        }
        stats.captured.fetch_add(1, Ordering::Relaxed);

        let (w, h) = (f.width as u32, f.height as u32);
        // scap BGRA'yı satır dolgusu olmadan verir: stride = 4 * genişlik.
        let mut buf = I420Buffer::new(w, h);
        let (sy, su, sv) = buf.strides();
        let (y, u, v) = buf.data_mut();
        // libyuv "ARGB" = bellekte B,G,R,A sırası, yani macOS BGRA.
        yuv_helper::argb_to_i420(&f.data, w * 4, y, sy, u, su, v, sv, w as i32, h as i32);

        let mut vf = VideoFrame::new(VideoRotation::VideoRotation0, buf);
        vf.timestamp_us = now_us();
        source.capture_frame(&vf);
        stats.sent.fetch_add(1, Ordering::Relaxed);
    }
    capturer.stop_capture();
}

fn print_stats_loop(stats: Arc<Stats>, stop: Arc<AtomicBool>) {
    use sysinfo::{ProcessRefreshKind, ProcessesToUpdate, System};
    let pid = sysinfo::get_current_pid().ok();
    let mut sys = System::new();
    let kind = ProcessRefreshKind::nothing().with_cpu().with_memory();
    if let Some(p) = pid {
        sys.refresh_processes_specifics(ProcessesToUpdate::Some(&[p]), true, kind);
    }
    let mut last = Instant::now();
    let (mut last_cap, mut last_sent) = (0u64, 0u64);
    while !stop.load(Ordering::Relaxed) {
        std::thread::sleep(Duration::from_secs(5));
        let el = last.elapsed().as_secs_f64();
        last = Instant::now();
        let cap = stats.captured.load(Ordering::Relaxed);
        let sent = stats.sent.load(Ordering::Relaxed);
        let empty = stats.empty.load(Ordering::Relaxed);
        let (mut cpu, mut rss_mb) = (0.0f32, 0.0f64);
        if let Some(p) = pid {
            sys.refresh_processes_specifics(ProcessesToUpdate::Some(&[p]), true, kind);
            if let Some(pr) = sys.process(p) {
                cpu = pr.cpu_usage(); // tek çekirdek = %100
                rss_mb = pr.memory() as f64 / 1024.0 / 1024.0;
            }
        }
        println!(
            "[istatistik] yakalanan {:.1} fps | gönderilen {:.1} fps | toplam gönderilen {} | boş kare {} | CPU %{:.1} | RSS {:.1} MB",
            (cap - last_cap) as f64 / el,
            (sent - last_sent) as f64 / el,
            sent,
            empty,
            cpu,
            rss_mb
        );
        last_cap = cap;
        last_sent = sent;
    }
}

#[tokio::main]
async fn main() {
    let args = parse_args();

    if args.check {
        run_check(&args);
        return;
    }

    if !scap::is_supported() {
        eprintln!("Bu sistemde ekran yakalama desteklenmiyor (macOS 12.3+ gerekir).");
        std::process::exit(1);
    }
    if !scap::has_permission() {
        // İzin penceresini açar; onaydan sonra macOS süreci yeniden başlatmayı ister.
        scap::request_permission();
        eprintln!(
            "Ekran kaydı izni yok; izin istendi.\n\
             Sistem Ayarları > Gizlilik ve Güvenlik > Ekran ve Sistem Ses Kaydı bölümünden \
             çalıştıran uygulamaya (ör. Terminal) izin verin, sonra programı yeniden başlatın."
        );
        std::process::exit(3);
    }

    let opts = capture_options(&args);
    let [w, h] = scap::capturer::get_output_frame_size(&opts);
    println!(
        "Yakalama: ana ekran, {w}x{h} @ {} fps (--res {})",
        args.fps, args.res_label
    );

    let tok = match fetch_token(&args.harness) {
        Ok(t) => t,
        Err(e) => {
            eprintln!("Token alınamadı ({}): {e}", args.harness);
            std::process::exit(1);
        }
    };
    println!("LiveKit: {} oda {} kimlik {IDENTITY}", tok.url, tok.room);

    let (room, mut events) =
        match Room::connect(&tok.url, &tok.token, RoomOptions::default()).await {
            Ok(r) => r,
            Err(e) => {
                eprintln!("Odaya bağlanılamadı: {e}");
                std::process::exit(1);
            }
        };
    println!("Bağlandı.");

    let source = NativeVideoSource::new(VideoResolution { width: w, height: h }, true);
    let track =
        LocalVideoTrack::create_video_track("screen", RtcVideoSource::Native(source.clone()));
    let publish = room
        .local_participant()
        .publish_track(
            LocalTrack::Video(track),
            TrackPublishOptions {
                source: TrackSource::Screenshare,
                video_codec: VideoCodec::VP8,
                simulcast: false,
                ..Default::default()
            },
        )
        .await;
    match publish {
        Ok(p) => println!("Yayında: track {}", p.sid()),
        Err(e) => {
            eprintln!("Track yayınlanamadı: {e}");
            std::process::exit(1);
        }
    }

    let stats = Arc::new(Stats::default());
    let stop = Arc::new(AtomicBool::new(false));

    let cap_thread = {
        let (s, st, src) = (stats.clone(), stop.clone(), source.clone());
        std::thread::spawn(move || capture_loop(opts, src, s, st))
    };
    {
        let (s, st) = (stats.clone(), stop.clone());
        std::thread::spawn(move || print_stats_loop(s, st));
    }

    println!("Çalışıyor. Durdurmak için Ctrl+C.");
    loop {
        tokio::select! {
            _ = tokio::signal::ctrl_c() => break,
            ev = events.recv() => match ev {
                Some(RoomEvent::Disconnected { reason }) => {
                    eprintln!("Bağlantı koptu: {reason:?}");
                    break;
                }
                Some(_) => {}
                None => break,
            },
            _ = tokio::time::sleep(Duration::from_millis(500)) => {
                if stop.load(Ordering::Relaxed) { break; }
            }
        }
    }

    stop.store(true, Ordering::SeqCst);
    let _ = room.close().await;
    let _ = cap_thread.join();
    println!(
        "Bitti. Toplam yakalanan {}, gönderilen {}.",
        stats.captured.load(Ordering::Relaxed),
        stats.sent.load(Ordering::Relaxed)
    );
}
