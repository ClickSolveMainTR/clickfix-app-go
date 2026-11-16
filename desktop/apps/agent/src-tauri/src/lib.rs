//! ClickFix Masaüstü (temsilci uygulaması), docs/design/masaustu.md §4, §6.
//!
//! Ana pencere önce paketlenmiş yerel sayfayı (`ui/index.html`) açar. Sayfa
//! `connect` komutunu çağırır:
//!
//! - uzak kök (`CLICKFIX_APP_URL`, varsayılan https://getclickfix.com)
//!   erişilemiyorsa yerel sayfa hata ve "Yeniden dene" gösterir;
//! - anahtar zincirinde oturum yoksa "Tarayıcıda giriş yap" gösterir
//!   (`start_login`, sistem tarayıcısı + PKCE, `clickfix://auth/callback`);
//! - oturum varsa webview oturumunu kurar (`/desktop/session`) ve web
//!   uygulamasına geçer.
//!
//! Uzak köke IPC yalnız çalışma anında eklenen `remote-app` capability'si
//! ile, yalnız o kök ve yalnız köprü komutları için açılır
//! ([`capability`]).

mod auth;
mod capability;
mod commands;
mod links;
mod settings;
mod tray;
mod web;

use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

use clickfix_shell::plugins::{self, MAIN_WINDOW};
use clickfix_shell::{AppConfig, Sidecar, SidecarSpec, net};
use serde::Serialize;
use tauri::webview::{NewWindowResponse, PageLoadEvent};
use tauri::{
    AppHandle, Manager, RunEvent, State, Webview, WebviewUrl, WebviewWindowBuilder, WindowEvent,
};
use tauri_plugin_autostart::{MacosLauncher, ManagerExt as _};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_global_shortcut::{GlobalShortcutExt, ShortcutState};
use tauri_plugin_notification::NotificationExt;
use tauri_plugin_opener::OpenerExt;
use tauri_plugin_window_state::StateFlags;
use url::Url;

use crate::auth::{AuthError, Keychain, PendingLogin, Pkce, Session};
use crate::links::DeepLink;
use crate::settings::SettingsStore;
use crate::web::DesktopEvent;

const CONNECT_TIMEOUT: Duration = Duration::from_secs(5);
/// Web uygulaması giriş sayfasına düşerse oturum en fazla bu sıklıkla
/// yeniden kurulur; daha sık düşerse döngü yerine yerel hata gösterilir.
const HANDOFF_COOLDOWN: Duration = Duration::from_secs(30);
/// Yerel bildirimden sonra pencere bu süre içinde öne gelirse bildirimin
/// sayfası açılır (masaüstünde bildirim tıklaması uygulamaya iletilmez).
const NOTIFICATION_CLICK_WINDOW: Duration = Duration::from_secs(20);
const MAX_TITLE: usize = 120;
const MAX_BODY: usize = 400;

/// Uygulamanın paylaşılan durumu.
struct Agent {
    config: AppConfig,
    session: Session,
    settings: SettingsStore,
    pending_login: Mutex<Option<PendingLogin>>,
    /// Oturum kurulunca açılacak sayfa (derin bağlantıdan).
    pending_next: Mutex<Option<String>>,
    /// `/desktop/session` yüklenince çalışacak devir betiği.
    handoff: Mutex<Option<String>>,
    last_handoff: Mutex<Option<Instant>>,
    last_notification: Mutex<Option<(Option<String>, Instant)>>,
    quitting: AtomicBool,
}

fn lock<T>(m: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    m.lock().unwrap_or_else(std::sync::PoisonError::into_inner)
}

/// Yerel sayfanın görünümleri (`ui/app.js`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum LocalView {
    SignedOut,
    Error,
}

fn local_url(view: LocalView, message: Option<&str>) -> Url {
    // Windows yerel varlıkları http://tauri.localhost'tan sunar.
    let base = if cfg!(target_os = "windows") {
        "http://tauri.localhost/index.html"
    } else {
        "tauri://localhost/index.html"
    };
    let mut url = Url::parse(base).expect("sabit adres");
    url.query_pairs_mut().append_pair(
        "view",
        match view {
            LocalView::SignedOut => "signed-out",
            LocalView::Error => "error",
        },
    );
    if let Some(m) = message {
        url.query_pairs_mut().append_pair("message", m);
    }
    url
}

fn show_local(app: &AppHandle, view: LocalView, message: Option<&str>) {
    if let Some(w) = app.get_webview_window(MAIN_WINDOW)
        && let Err(e) = w.navigate(local_url(view, message))
    {
        log::error!("yerel sayfa açılamadı: {e}");
    }
}

/// Ana pencereyi gösterir, küçültülmüşse açar ve öne getirir.
pub(crate) fn show_main(app: &AppHandle) {
    if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
}

fn toggle_main(app: &AppHandle) {
    if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
        if w.is_visible().unwrap_or(false) && w.is_focused().unwrap_or(false) {
            let _ = w.hide();
        } else {
            show_main(app);
        }
    }
}

/// Ana pencere şu an uygulama kökünde mi (web uygulaması yüklü mü)?
fn on_app_origin(app: &AppHandle, config: &AppConfig) -> Option<Url> {
    let w = app.get_webview_window(MAIN_WINDOW)?;
    let url = w.url().ok()?;
    config.is_app_origin(&url).then_some(url)
}

/// Web uygulamasına `clickfix:desktop` olayı gönderir; uygulama yüklü
/// değilse hiçbir şey yapmaz.
fn dispatch(app: &AppHandle, event: &DesktopEvent) -> bool {
    let agent = app.state::<Agent>();
    if on_app_origin(app, &agent.config).is_none() {
        return false;
    }
    match app.get_webview_window(MAIN_WINDOW) {
        Some(w) => w.eval(web::event_script(event)).is_ok(),
        None => false,
    }
}

/// Webview oturumunu kurar: pencere `/desktop/session`'a gider (GET, boş
/// sayfa), yüklenince devir betiği form ile POST eder ve Next rotası 303
/// ile `next`'e (ya da panele) yönlendirir.
fn start_handoff(app: &AppHandle, access_token: &str) {
    let agent = app.state::<Agent>();
    let next = lock(&agent.pending_next).take();
    *lock(&agent.handoff) = Some(web::handoff_script(access_token, next.as_deref()));
    *lock(&agent.last_handoff) = Some(Instant::now());
    let target = agent
        .config
        .app_url
        .join(web::HANDOFF_PATH.trim_start_matches('/'))
        .expect("sabit yol");
    if let Some(w) = app.get_webview_window(MAIN_WINDOW)
        && let Err(e) = w.navigate(target)
    {
        log::error!("oturum sayfası açılamadı: {e}");
    }
}

/// Oturum varsa webview'e devreder, yoksa yerel giriş ekranını gösterir.
async fn resume_session(app: AppHandle) {
    let agent = app.state::<Agent>();
    match agent.session.access_token().await {
        Ok(Some(token)) => start_handoff(&app, &token),
        Ok(None) => show_local(&app, LocalView::SignedOut, None),
        Err(e) => {
            log::error!("oturum yenilenemedi: {e}");
            show_local(&app, LocalView::Error, Some(&user_message(&e)));
        }
    }
}

fn user_message(e: &AuthError) -> String {
    match e {
        AuthError::Network(_) => {
            "ClickFix sunucusuna ulaşılamıyor. Bağlantınızı denetleyip yeniden deneyin.".into()
        }
        AuthError::Rejected(_) => {
            "Giriş bağlantısının süresi dolmuş ya da kullanılmış. Yeniden giriş yapın.".into()
        }
        AuthError::Keychain(_) => {
            "Oturum anahtar zincirine kaydedilemedi. Sistem anahtar zincirine erişim izni verin."
                .into()
        }
        AuthError::Unexpected(_) | AuthError::Random(_) => {
            "Beklenmeyen bir hata oluştu. Ayrıntı günlükte.".into()
        }
    }
}

/// Yüklenen her sayfadan sonra: bekleyen oturum devri ve web oturumunun
/// düşmesi.
fn after_page_load(app: &AppHandle, url: &Url) {
    let agent = app.state::<Agent>();
    if !agent.config.is_app_origin(url) {
        return;
    }
    let path = url.path();
    if path == web::HANDOFF_PATH {
        match lock(&agent.handoff).take() {
            Some(script) => {
                if let Some(w) = app.get_webview_window(MAIN_WINDOW)
                    && let Err(e) = w.eval(script)
                {
                    log::error!("oturum devri çalıştırılamadı: {e}");
                }
            }
            // POST sonucu bu adreste kaldıysa devir reddedildi (başarıda 303).
            None => {
                log::warn!("webview oturumu kurulamadı");
                show_local(
                    app,
                    LocalView::Error,
                    Some(
                        "Oturum açılamadı. Yeniden deneyin; sürerse çıkış yapıp tekrar giriş yapın.",
                    ),
                );
            }
        }
        return;
    }
    // Yalnız ana giriş sayfası; `/login/sso` gibi alt sayfalar bilerek açılmıştır.
    if path == "/login" {
        // Web oturumu düştü (süresi doldu, başka yerden kapatıldı): uygulama
        // oturumu varsa yeniden devret, yoksa yerel giriş ekranı.
        let recent = lock(&agent.last_handoff).is_some_and(|t| t.elapsed() < HANDOFF_COOLDOWN);
        if recent {
            show_local(
                app,
                LocalView::Error,
                Some("Oturum açılamadı. Yeniden deneyin; sürerse çıkış yapıp tekrar giriş yapın."),
            );
        } else {
            tauri::async_runtime::spawn(resume_session(app.clone()));
        }
    }
}

/// `clickfix://` bağlantıları (soğuk açılış, ikinci örnek, macOS olayı).
fn handle_deep_links(app: &AppHandle, urls: Vec<Url>) {
    for url in urls {
        let Some(link) = links::parse(&url) else {
            continue;
        };
        show_main(app);
        match link {
            DeepLink::AuthCallback { code, state } => {
                let agent = app.state::<Agent>();
                let pending = {
                    let mut slot = lock(&agent.pending_login);
                    match slot.as_ref() {
                        Some(p) if p.matches(&state, Instant::now()) => slot.take(),
                        _ => None,
                    }
                };
                let Some(pending) = pending else {
                    log::warn!("beklenmeyen giriş dönüşü yok sayıldı");
                    continue;
                };
                tauri::async_runtime::spawn(finish_login(app.clone(), code, pending));
            }
            DeepLink::Open { path } => open_path(app, &path),
            DeepLink::Focus => {}
        }
    }
}

async fn finish_login(app: AppHandle, code: String, pending: PendingLogin) {
    let agent = app.state::<Agent>();
    let result = match agent.session.api().exchange(&code, &pending.pkce).await {
        Ok(tokens) => agent.session.sign_in(tokens).await,
        Err(e) => Err(e),
    };
    match result {
        Ok(access) => {
            log::info!("giriş tamamlandı");
            start_handoff(&app, &access);
        }
        Err(e) => {
            log::error!("giriş takası başarısız: {e}");
            show_local(&app, LocalView::Error, Some(&user_message(&e)));
        }
    }
}

/// Uygulama içi bir sayfayı açar; web uygulaması yüklü değilse oturum
/// kurulunca açılmak üzere saklar.
fn open_path(app: &AppHandle, path: &str) {
    let agent = app.state::<Agent>();
    let Some(target) = links::resolve(&agent.config.app_url, path) else {
        return;
    };
    let loaded = on_app_origin(app, &agent.config)
        .is_some_and(|u| u.path() != "/login" && !u.path().starts_with("/desktop/"));
    if loaded {
        if let Some(w) = app.get_webview_window(MAIN_WINDOW)
            && let Err(e) = w.navigate(target)
        {
            log::error!("sayfa açılamadı: {e}");
        }
    } else {
        *lock(&agent.pending_next) = links::app_path(path);
    }
}

// --- Yerel sayfa komutları (capabilities/local.json) ---

#[derive(Serialize)]
#[serde(tag = "status", rename_all = "snake_case")]
enum ConnectResult {
    SignedIn,
    SignedOut,
}

/// Uzak köke erişilebiliyorsa oturumu dener: varsa webview'e devreder,
/// yoksa `signed_out` döner. Erişilemiyorsa hata metni döner.
#[tauri::command]
async fn connect(app: AppHandle, agent: State<'_, Agent>) -> Result<ConnectResult, String> {
    let probe = agent.config.app_url.clone();
    tauri::async_runtime::spawn_blocking(move || net::reachable(&probe, CONNECT_TIMEOUT))
        .await
        .map_err(|e| e.to_string())??;
    match agent.session.access_token().await {
        Ok(Some(token)) => {
            start_handoff(&app, &token);
            Ok(ConnectResult::SignedIn)
        }
        Ok(None) => Ok(ConnectResult::SignedOut),
        Err(e) => {
            log::error!("oturum yenilenemedi: {e}");
            Err(user_message(&e))
        }
    }
}

/// Sistem tarayıcısında giriş sayfasını açar (PKCE + state).
#[tauri::command]
fn start_login(app: AppHandle, agent: State<'_, Agent>) -> Result<(), String> {
    let pkce = Pkce::generate().map_err(|e| e.to_string())?;
    let url = auth::login_url(&agent.config.app_url, &pkce);
    *lock(&agent.pending_login) = Some(PendingLogin::new(pkce));
    app.opener()
        .open_url(url.as_str(), None::<&str>)
        .map_err(|e| format!("tarayıcı açılamadı: {e}"))?;
    log::info!("giriş tarayıcıda başlatıldı");
    Ok(())
}

#[tauri::command]
fn cancel_login(agent: State<'_, Agent>) {
    *lock(&agent.pending_login) = None;
}

// --- Web köprüsü komutları (remote-app capability, yalnız uygulama kökü) ---

/// Capability zaten kökü sınırlar; komutlar yine de çağıranın kökünü
/// denetler (savunma derinliği).
fn require_app_origin(webview: &Webview, agent: &Agent) -> Result<(), String> {
    match webview.url() {
        Ok(u) if agent.config.is_app_origin(&u) => Ok(()),
        _ => Err("izin yok".into()),
    }
}

fn truncate(s: &str, max: usize) -> String {
    s.trim().chars().take(max).collect()
}

/// Yerel bildirim. Pencere öndeyse gösterilmez (web uygulaması kendi
/// bildirimini zaten gösterir) ve `false` döner.
#[tauri::command]
fn notify(
    app: AppHandle,
    webview: Webview,
    agent: State<'_, Agent>,
    title: String,
    body: Option<String>,
    path: Option<String>,
) -> Result<bool, String> {
    require_app_origin(&webview, &agent)?;
    let title = truncate(&title, MAX_TITLE);
    if title.is_empty() {
        return Err("başlık gerekli".into());
    }
    if let Some(w) = app.get_webview_window(MAIN_WINDOW)
        && w.is_visible().unwrap_or(false)
        && w.is_focused().unwrap_or(false)
    {
        return Ok(false);
    }
    let mut n = app.notification().builder().title(title);
    if let Some(b) = body
        .as_deref()
        .map(|b| truncate(b, MAX_BODY))
        .filter(|b| !b.is_empty())
    {
        n = n.body(b);
    }
    n.show()
        .map_err(|e| format!("bildirim gösterilemedi: {e}"))?;
    let path = path.as_deref().and_then(links::app_path);
    *lock(&agent.last_notification) = Some((path, Instant::now()));
    Ok(true)
}

/// Okunmamış atanmış konuşma sayısı (dock rozeti, görev çubuğu, tepsi).
#[tauri::command]
fn set_badge(
    app: AppHandle,
    webview: Webview,
    agent: State<'_, Agent>,
    count: u32,
) -> Result<(), String> {
    require_app_origin(&webview, &agent)?;
    tray::set_badge(&app, count.min(9999));
    Ok(())
}

/// Web'deki durum (presence) değişti: tepsi menüsünde işaretlenir.
#[tauri::command]
fn set_presence(
    app: AppHandle,
    webview: Webview,
    agent: State<'_, Agent>,
    status: Option<String>,
) -> Result<(), String> {
    require_app_origin(&webview, &agent)?;
    let status = status.filter(|s| tray::is_presence(s));
    if let Some(h) = app.try_state::<tray::TrayHandles>() {
        h.set_presence(status.as_deref());
    }
    Ok(())
}

/// Dış bağlantıyı sistem tarayıcısında açar (yalnız http, https, mailto).
#[tauri::command]
fn open_external(
    app: AppHandle,
    webview: Webview,
    agent: State<'_, Agent>,
    url: String,
) -> Result<(), String> {
    require_app_origin(&webview, &agent)?;
    let url = links::external_url(&url).ok_or("bu bağlantı açılamaz")?;
    app.opener()
        .open_url(url.as_str(), None::<&str>)
        .map_err(|e| format!("tarayıcı açılamadı: {e}"))
}

#[derive(Serialize)]
struct VersionInfo {
    version: String,
    bridge: u32,
    os: &'static str,
}

#[tauri::command]
fn version(
    app: AppHandle,
    webview: Webview,
    agent: State<'_, Agent>,
) -> Result<VersionInfo, String> {
    require_app_origin(&webview, &agent)?;
    Ok(VersionInfo {
        version: app.package_info().version.to_string(),
        bridge: web::BRIDGE_VERSION,
        os: auth::client_os(),
    })
}

/// Web uygulamasından çıkış: uygulama oturumu da kapanır ve yerel giriş
/// ekranı açılır.
#[tauri::command]
async fn sign_out(app: AppHandle, webview: Webview, agent: State<'_, Agent>) -> Result<(), String> {
    require_app_origin(&webview, &agent)?;
    if let Err(e) = agent.session.sign_out().await {
        log::error!("çıkış: {e}");
    }
    show_local(&app, LocalView::SignedOut, None);
    Ok(())
}

// --- Tepsi ve menü ---

fn handle_menu(app: &AppHandle, id: &str) {
    use tray::ids;
    let agent = app.state::<Agent>();
    match id {
        ids::TOGGLE => toggle_main(app),
        ids::CLOSE_TO_TRAY => {
            let s = agent
                .settings
                .update(|s| s.close_to_tray = !s.close_to_tray);
            if let Some(h) = app.try_state::<tray::TrayHandles>() {
                let _ = h.close_to_tray.set_checked(s.close_to_tray);
            }
        }
        ids::AUTOSTART => {
            let s = agent.settings.update(|s| s.autostart = !s.autostart);
            apply_autostart(app, s.autostart);
            if let Some(h) = app.try_state::<tray::TrayHandles>() {
                let _ = h.autostart.set_checked(s.autostart);
            }
        }
        ids::SIGN_OUT => {
            let app = app.clone();
            tauri::async_runtime::spawn(async move {
                let agent = app.state::<Agent>();
                if let Err(e) = agent.session.sign_out().await {
                    log::error!("çıkış: {e}");
                }
                // Web oturumunu web uygulaması kapatır; yüklü değilse doğrudan
                // yerel giriş ekranı.
                if !dispatch(&app, &DesktopEvent::SignOut) {
                    show_local(&app, LocalView::SignedOut, None);
                }
            });
        }
        ids::QUIT => {
            agent.quitting.store(true, Ordering::SeqCst);
            app.exit(0);
        }
        ids::CHECK_UPDATES => {
            // CF-240 güncelleyiciyi ekleyene kadar yer tutucu.
            let _ = app
                .notification()
                .builder()
                .title("Güncellemeler")
                .body("Otomatik güncelleme yakında geliyor. Yeni sürümü getclickfix.com/indir adresinden indirebilirsiniz.")
                .show();
        }
        ids::OPEN_LOGS => match app.path().app_log_dir() {
            Ok(dir) => {
                let _ = std::fs::create_dir_all(&dir);
                if let Err(e) = app.opener().open_path(dir.to_string_lossy(), None::<&str>) {
                    log::error!("günlük dizini açılamadı: {e}");
                }
            }
            Err(e) => log::error!("günlük dizini bulunamadı: {e}"),
        },
        other => {
            if let Some(status) = tray::presence_from_id(other) {
                if let Some(h) = app.try_state::<tray::TrayHandles>() {
                    h.set_presence(Some(status));
                }
                if !dispatch(
                    app,
                    &DesktopEvent::Presence {
                        status: status.to_owned(),
                    },
                ) {
                    show_main(app);
                }
            }
        }
    }
}

fn apply_autostart(app: &AppHandle, enabled: bool) {
    let mgr = app.autolaunch();
    let current = mgr.is_enabled().unwrap_or(false);
    let result = match (enabled, current) {
        (true, false) => mgr.enable(),
        (false, true) => mgr.disable(),
        _ => Ok(()),
    };
    if let Err(e) = result {
        log::error!("otomatik başlatma ayarlanamadı: {e}");
    }
}

fn register_shortcut(app: &AppHandle, shortcut: &str) {
    let shortcut = shortcut.trim();
    if shortcut.is_empty() {
        log::info!("genel kısayol kapalı");
        return;
    }
    let gs = app.global_shortcut();
    let result = gs.register(shortcut).or_else(|e| {
        log::warn!("kısayol {shortcut} kaydedilemedi ({e}), varsayılan deneniyor");
        gs.register(settings::DEFAULT_SHORTCUT)
    });
    if let Err(e) = result {
        log::warn!("genel kısayol kaydedilemedi: {e}");
    }
}

/// Bildirimden kısa süre sonra pencere öne geldiyse bildirimin sayfasını aç.
fn on_main_focused(app: &AppHandle) {
    let agent = app.state::<Agent>();
    let recent = lock(&agent.last_notification)
        .take()
        .filter(|(_, at)| at.elapsed() <= NOTIFICATION_CLICK_WINDOW);
    if let Some((Some(path), _)) = recent {
        let same = on_app_origin(app, &agent.config).is_some_and(|u| {
            let current = match u.query() {
                Some(q) => format!("{}?{q}", u.path()),
                None => u.path().to_owned(),
            };
            current == path
        });
        if !same {
            open_path(app, &path);
        }
    }
}

/// Ana pencere yalnız yerel sayfaya ve uygulama köküne gidebilir; diğer
/// adresler engellenir. Dış bağlantılar yeni pencere isteği
/// (`target=_blank`) ya da köprünün `openExternal` komutuyla sistem
/// tarayıcısında açılır.
fn navigation_allowed(config: &AppConfig, url: &Url) -> bool {
    config.is_app_origin(url) || is_local_asset(url)
}

fn is_local_asset(url: &Url) -> bool {
    match url.scheme() {
        "tauri" => true,
        // Windows ve Android yerel varlıkları http(s)://tauri.localhost'tan sunar.
        "http" | "https" => url.host_str() == Some("tauri.localhost"),
        _ => false,
    }
}

/// Yeni pencere isteği (`window.open`, `target=_blank`): uygulama kökündeki
/// adres ana pencerede, dış adres sistem tarayıcısında açılır. Ek pencere
/// açılmaz.
fn new_window_target(config: &AppConfig, url: &Url) -> NewWindowTarget {
    if config.is_app_origin(url) {
        NewWindowTarget::Main
    } else if links::external_url(url.as_str()).is_some() {
        NewWindowTarget::Browser
    } else {
        NewWindowTarget::Ignore
    }
}

#[derive(Debug, PartialEq, Eq)]
enum NewWindowTarget {
    Main,
    Browser,
    Ignore,
}

fn store_identifier(app: &tauri::App) -> String {
    app.config().identifier.clone()
}

pub fn run() {
    let config = match AppConfig::from_env() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("clickfix-agent: {e}");
            std::process::exit(2);
        }
    };

    let app = tauri::Builder::default()
        // Tek örnek ilk sırada; `deep-link` özelliğiyle ikinci açılışın
        // argümanlarını derin bağlantı eklentisine iletir.
        .plugin(plugins::single_instance())
        .plugin(tauri_plugin_deep_link::init())
        .plugin(plugins::logging())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_autostart::init(
            MacosLauncher::LaunchAgent,
            None,
        ))
        .plugin(
            tauri_plugin_window_state::Builder::default()
                // Görünürlük saklanmaz: tepsiye gizlenmiş pencere bir sonraki
                // açılışta gizli başlamasın.
                .with_state_flags(StateFlags::all() & !StateFlags::VISIBLE)
                .build(),
        )
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(|app, _shortcut, event| {
                    if event.state == ShortcutState::Pressed {
                        show_main(app);
                        dispatch(app, &DesktopEvent::CommandPalette);
                    }
                })
                .build(),
        )
        .invoke_handler(tauri::generate_handler![
            connect,
            start_login,
            cancel_login,
            notify,
            set_badge,
            set_presence,
            open_external,
            version,
            sign_out
        ])
        .setup(move |app| {
            log::info!("ClickFix Masaüstü başlıyor, kök {}", config.app_url);
            let handle = app.handle().clone();

            let user_agent = format!(
                "ClickFixDesktop/{} ({})",
                app.package_info().version,
                auth::client_os()
            );
            let api = auth::Api::new(&config.app_url, &user_agent)?;
            let store = Keychain::new(&store_identifier(app), &config.app_url);
            let settings = SettingsStore::open(&app.path().app_config_dir()?);
            let current = settings.get();
            app.manage(Agent {
                config: config.clone(),
                session: Session::new(api, Box::new(store)),
                settings,
                pending_login: Mutex::new(None),
                pending_next: Mutex::new(None),
                handoff: Mutex::new(None),
                last_handoff: Mutex::new(None),
                last_notification: Mutex::new(None),
                quitting: AtomicBool::new(false),
            });

            // Uzak köke IPC: yalnız bu kök, yalnız ana pencere, yalnız köprü
            // komutları.
            app.add_capability(capability::remote(&config))?;

            app.set_menu(tray::build_app_menu(&handle)?)?;
            app.on_menu_event(|app, event| handle_menu(app, event.id().as_ref()));
            let handles = tray::build_tray(&handle, &current)?;
            app.manage(handles);

            apply_autostart(&handle, current.autostart);
            register_shortcut(&handle, &current.shortcut);

            let nav = config.clone();
            let popup = config.clone();
            let popup_app = handle.clone();
            let load_app = handle.clone();
            let window =
                WebviewWindowBuilder::new(app, MAIN_WINDOW, WebviewUrl::App("index.html".into()))
                    .title("ClickFix Masaüstü")
                    .inner_size(1280.0, 800.0)
                    .min_inner_size(900.0, 600.0)
                    .initialization_script(web::init_script(
                        &config.app_url.origin().ascii_serialization(),
                        &app.package_info().version.to_string(),
                        auth::client_os(),
                    ))
                    .on_navigation(move |url| {
                        let ok = navigation_allowed(&nav, url);
                        if !ok {
                            log::warn!(
                                "gezinme engellendi: {}",
                                url.origin().ascii_serialization()
                            );
                        }
                        ok
                    })
                    .on_new_window(move |url, _features| {
                        match new_window_target(&popup, &url) {
                            NewWindowTarget::Main => {
                                if let Some(w) = popup_app.get_webview_window(MAIN_WINDOW) {
                                    let _ = w.navigate(url);
                                }
                            }
                            NewWindowTarget::Browser => {
                                if let Err(e) =
                                    popup_app.opener().open_url(url.as_str(), None::<&str>)
                                {
                                    log::error!("tarayıcı açılamadı: {e}");
                                }
                            }
                            NewWindowTarget::Ignore => {
                                log::warn!("yeni pencere engellendi: {}", url.scheme());
                            }
                        }
                        NewWindowResponse::Deny
                    })
                    .on_page_load(move |_, payload| {
                        if payload.event() == PageLoadEvent::Finished {
                            after_page_load(&load_app, payload.url());
                        }
                    })
                    .build()?;

            let close_app = handle.clone();
            window.on_window_event(move |event| match event {
                WindowEvent::CloseRequested { api, .. } => {
                    let agent = close_app.state::<Agent>();
                    if !agent.quitting.load(Ordering::SeqCst) && agent.settings.get().close_to_tray
                    {
                        api.prevent_close();
                        if let Some(w) = close_app.get_webview_window(MAIN_WINDOW) {
                            let _ = w.hide();
                        }
                    }
                }
                WindowEvent::Focused(true) => on_main_focused(&close_app),
                _ => {}
            });

            // Derin bağlantılar: geliştirmede (ve AppImage'da) şemayı çalışma
            // anında kaydet; paketli kurulum bunu kurulumda yapar.
            #[cfg(any(target_os = "linux", target_os = "windows"))]
            if cfg!(debug_assertions)
                && let Err(e) = app.deep_link().register_all()
            {
                log::warn!("clickfix:// şeması kaydedilemedi: {e}");
            }
            let link_app = handle.clone();
            app.deep_link()
                .on_open_url(move |event| handle_deep_links(&link_app, event.urls()));
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                handle_deep_links(&handle, urls);
            }

            match SidecarSpec::bundled("agent") {
                Ok(spec) => {
                    app.manage(Sidecar::spawn(spec));
                }
                Err(e) => log::error!("sidecar yolu bulunamadı: {e}"),
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("ClickFix Masaüstü başlatılamadı");

    app.run(|handle, event| match event {
        RunEvent::Exit => {
            if let Some(sc) = handle.try_state::<Sidecar>() {
                sc.shutdown();
            }
        }
        #[cfg(target_os = "macos")]
        RunEvent::Reopen { .. } => show_main(handle),
        _ => {}
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    fn url(s: &str) -> Url {
        Url::parse(s).expect("url")
    }

    #[test]
    fn navigation_policy() {
        let c = AppConfig::from_value(None).expect("yapılandırma");
        assert!(navigation_allowed(
            &c,
            &url("https://getclickfix.com/inbox")
        ));
        assert!(navigation_allowed(&c, &url("tauri://localhost/index.html")));
        assert!(navigation_allowed(
            &c,
            &url("http://tauri.localhost/index.html")
        ));
        assert!(!navigation_allowed(
            &c,
            &url("https://accounts.google.com/")
        ));
        assert!(!navigation_allowed(
            &c,
            &url("https://getclickfix.com.evil.test/")
        ));
        assert!(!navigation_allowed(&c, &url("file:///etc/passwd")));
    }

    #[test]
    fn new_window_policy() {
        let c = AppConfig::from_value(None).expect("yapılandırma");
        assert_eq!(
            new_window_target(&c, &url("https://getclickfix.com/inbox?c=1")),
            NewWindowTarget::Main
        );
        assert_eq!(
            new_window_target(&c, &url("https://docs.example.com/")),
            NewWindowTarget::Browser
        );
        assert_eq!(
            new_window_target(&c, &url("mailto:a@example.com")),
            NewWindowTarget::Browser
        );
        assert_eq!(
            new_window_target(&c, &url("file:///etc/passwd")),
            NewWindowTarget::Ignore
        );
        assert_eq!(
            new_window_target(&c, &url("clickfix://open")),
            NewWindowTarget::Ignore
        );
    }

    #[test]
    fn local_page_urls() {
        let u = local_url(LocalView::Error, Some("Bağlantı & hata"));
        assert_eq!(u.path(), "/index.html");
        let q: Vec<(String, String)> = u.query_pairs().into_owned().collect();
        assert_eq!(
            q,
            vec![
                ("view".into(), "error".into()),
                ("message".into(), "Bağlantı & hata".into())
            ]
        );
        assert!(is_local_asset(&local_url(LocalView::SignedOut, None)));
    }

    #[test]
    fn truncates_on_char_boundaries() {
        assert_eq!(truncate("  çğüşöı  ", 3), "çğü");
    }

    #[test]
    fn user_messages_carry_no_details() {
        let m = user_message(&AuthError::Network("10.0.0.1:443 refused".into()));
        assert!(!m.contains("10.0.0.1"));
    }
}
