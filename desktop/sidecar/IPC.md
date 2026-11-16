# Sidecar IPC sözleşmesi (`--mode=support`, CF-238)

ClickFix Destek arayüzü (CF-239) sidecar ile kabuk (`crates/shell`) üzerinden konuşur. Bu belge yöntemleri, olayları ve hata kodlarını tanımlar. Taşıma katmanı `internal/ipc` paketindedir: satır sınırlı JSON-RPC, istek `{"id","method","params"}`, yanıt `{"id","result"}` ya da `{"id","error":{"code","message","data"}}`, olay `{"event","params"}`.

Sözleşme yalnız geriye uyumlu genişler: alan eklenebilir, alan silinmez ve anlamı değişmez. Arayüz bilinmeyen alanları ve olayları yok saymalıdır.

## İlkeler

- **Sırlar arayüze çıkmaz.** `client_secret`, LiveKit token'ı ve katılma bağlantısının token'ı yalnız sidecar belleğindedir. Hiçbir yanıt ya da olay bunları taşımaz; günlüklerde maskelenir.
- **Durum tek kaynaktan gelir.** Her değişiklikte sidecar `state` olayıyla **tam anlık görüntüyü** gönderir. Arayüz ekranı yalnız son `state`'e göre çizer; yöntem yanıtları da aynı anlık görüntüyü döner (`session.resolve` hariç, o ek olarak çözülen oturumu taşır).
- **Bir anda bir oturum.** Yeni `session.resolve`, önceki oturum `ended` ya da `idle` değilse `-32008` döner.
- **Zaman aşımı.** API çağrısı yapan yöntemler (`session.resolve`, `session.consent`, `session.stop`, `control.revoke`, `session.report`) yeniden denemelerle birlikte en çok 15 sn sürer. Kabuk bu yöntemler için en az **20 sn** beklemelidir; `SidecarSpec::call_timeout` varsayılanı (5 sn) yetmez. `session.start` uzun sürmez: hızlı denetimleri yapıp döner, bağlantı ilerlemesi `state` ile gelir.

## Durum modeli

### `phase`

| Değer | Anlam | Sonraki |
|---|---|---|
| `idle` | Oturum yok (açılış) | `resolved` |
| `resolved` | Kod çözüldü; kimlik ve onay ekranları. İzleme onayı `consent_deadline`'a kadar verilmeli | `ready`, `ended` |
| `ready` | İzleme onayı verildi; izinler tamamsa `session.start` çağrılabilir | `starting`, `ended` |
| `starting` | Token alınıyor, LiveKit'e bağlanılıyor, yakalama başlıyor | `active`, `ready` (hata), `ended` |
| `active` | Ekran yayında | `reconnecting`, `ending`, `ended` |
| `reconnecting` | Bağlantı koptu, yeniden deneniyor. Kontrol kapısı kapalıdır | `active`, `ended` |
| `ending` | Durdurma sürüyor | `ended` |
| `ended` | Oturum bitti; `summary` dolu | `idle` (yeni `session.resolve`) |

### `control`

Tasarımdaki kontrol durumları (Paylaşım çubuğu, Kontrol onayı ekranı):

| Değer | Tasarımda | Koşul |
|---|---|---|
| `view_only` | Yalnız izleme | Temsilci yalnız izleme istedi ya da müşteri "Yalnız izlesin" dedi |
| `requested` | İstendi | Temsilci kontrol istedi, müşteri henüz onaylamadı |
| `pending_permission` | (İzin ekranı, Erişilebilirlik adımı) | Kontrol onaylı ama macOS Erişilebilirlik izni yok; girdi uygulanamaz |
| `active` | Kontrol temsilcide | Kontrol onaylı, izin var; kapı oturum etkinken açıktır |
| `revoked` | Geri alındı | Müşteri onayı geri aldı (`control.revoke` ya da `session.consent {control:false}` önceden verilmiş onayı kaldırınca) |

Kontrol kapısı (`internal/control`) yalnız `phase = active`, `control = active`, gönderen temsilci kimliği ve hız sınırı birlikte sağlanınca girdi uygular (tasarım 5. bölüm).

### `state` olayının biçimi

```json
{
  "phase": "active",
  "session": {
    "uuid": "6f1c…",
    "organization": {"uuid": "…", "name": "Atlas Teknik Servis", "logo_url": null, "verified": true},
    "agent": {"uuid": "…", "name": "Elif Kaya", "avatar_url": null},
    "mode_requested": "control",
    "recording_requested": true,
    "max_duration_minutes": 60,
    "consent_deadline": "2026-11-16T14:30:00Z",
    "warning": "ClickFix çalışanları ve işletmeler …",
    "locale": "tr"
  },
  "consent": {"view": true, "control": true, "recording": false},
  "control": "active",
  "connection": "connected",
  "agent_present": true,
  "started_at": "2026-11-16T14:16:02Z",
  "ends_at": "2026-11-16T15:16:02Z",
  "permissions": {"screen_recording": "granted", "accessibility": "granted"},
  "summary": null
}
```

| Alan | Tür | Not |
|---|---|---|
| `phase` | metin | Yukarıdaki tablo |
| `session` | nesne \| null | `idle`'da null. Alanlar API'nin `RemoteSupportResolved` yanıtından, `client_secret` hariç |
| `consent.view` / `consent.control` | boolean | Verilmiş ve yürürlükte olan onaylar |
| `consent.recording` | boolean \| null | null: sorulmadı ya da organizasyon kaydetmiyor |
| `control` | metin | Yukarıdaki tablo |
| `connection` | `none` \| `connecting` \| `connected` \| `reconnecting` | LiveKit bağlantısı |
| `agent_present` | boolean | Temsilci odada mı (bağlantı gelmeden çubukta "bekleniyor" gösterilebilir) |
| `started_at` | RFC 3339 \| null | Ekran yayınının başladığı an; çubuktaki süre sayacı buradan |
| `ends_at` | RFC 3339 \| null | En geç bitiş (sunucunun `ends_at` değeri) |
| `permissions` | nesne | `permissions.status` ile aynı biçim |
| `summary` | nesne \| null | Yalnız `ended`'da dolu, aşağıda |

### `summary` (oturum bitiş özeti)

Tasarımdaki "Oturum bitti" ekranının satırları: Temsilci, Süre, Kontrol, Kayıt, Bitiren.

```json
{
  "agent_name": "Elif Kaya",
  "organization_name": "Atlas Teknik Servis",
  "started_at": "2026-11-16T14:16:02Z",
  "ended_at": "2026-11-16T14:30:34Z",
  "duration_seconds": 872,
  "control": {"granted": true, "seconds": 360, "revoked_by": "customer"},
  "recorded": false,
  "ended_reason": "customer",
  "ended_by": "customer",
  "reported": false
}
```

| Alan | Not |
|---|---|
| `duration_seconds` | Ekran yayınının süresi (`started_at` → `ended_at`); yayın hiç başlamadıysa 0 |
| `control.granted` | Oturumda kontrol onayı hiç verildi mi |
| `control.seconds` | Kontrolün yürürlükte kaldığı toplam süre |
| `control.revoked_by` | `customer` (müşteri geri aldı) ya da null (oturum bitene kadar sürdü) |
| `recorded` | Kayıt onayı verildi ve oturum yayına geçti |
| `ended_reason` | API'nin `ended_reason` değeri: `agent`, `customer`, `declined`, `code_expired`, `timeout`, `attempts`, `max_duration`, `conversation_closed`, `platform`, `reported`; sunucuya ulaşılamadıysa `connection_lost` |
| `ended_by` | Kısa özet: `customer` (customer, declined, reported), `agent` (agent, conversation_closed), `time` (timeout, max_duration, code_expired, attempts), `platform`, `connection` |
| `reported` | "Bildir" ile dolandırıcılık bildirimi yapıldı |

## Yöntemler

Issue'daki altı yöntem (`session.resolve`, `session.consent`, `session.start`, `session.stop`, `control.revoke`, `permissions.status`) ve tasarımın gerektirdiği üç ek yöntem (`permissions.request`, `session.report`, `sysinfo.get`). `ping` ve `version` her modda vardır (`internal/ipc/builtin.go`).

### `session.resolve`

Kodu ya da katılma bağlantısını çözer (`POST /v1/public/remote-support/resolve`).

```json
{"code": "123 456 789"}
{"link": "clickfix-support://join?t=…"}
{"code": "123456789", "share_system_info": true}
```

- Tam olarak biri: `code` (9 hane; boşluk ve tire yok sayılır) ya da `link` (tam `clickfix-support://join?t=…` adresi ya da yalnız `t` değeri).
- `share_system_info` (varsayılan false): true ise API'ye ayrıntılı işletim sistemi sürümü ve mimari gider (`client_os`, ör. `macOS 27.2 arm64`). False ise yalnız `macos` / `windows`. Uygulama sürümü her zaman gider (kullanıcı ajanında da vardır). Kişisel veri (kullanıcı adı, bilgisayar adı, IP listesi) hiç toplanmaz.
- Yanıt: `{"state": <state>}`; `state.phase = resolved`, `state.session` dolu.
- Hatalar: `-32001` kod geçersiz, `-32002` çok deneme, `-32004` organizasyonda kapalı, `-32005` ağ.

### `session.consent`

Onayları verir ya da kontrol/kayıt onayını kaldırır (`POST /v1/public/remote-support/{uuid}/consent`). Her alan isteğe bağlıdır ama en az biri verilir.

```json
{"view": true}
{"control": true, "recording": false}
{"control": false}
```

- `view` yalnız `true` olabilir ve ilk verilmesi gereken onaydır. Vazgeçmek `session.stop`'tur.
- `control: true` yalnız `mode_requested = control` iken kabul edilir. `control: false` daha önce verilmiş onayı kaldırır (`control.revoke` ile aynı) ya da isteği reddeder ("Yalnız izlesin": `control = view_only`).
- `recording` yalnız `recording_requested = true` iken; `false` kaydı reddeder ya da durdurur.
- Kapı, API yanıtı beklenmeden yerelde kapanır (geri alma her zaman önce yerelde geçerlidir).
- Yanıt: `{"state": <state>}`.

### `session.start`

Ekran paylaşımını başlatır. `phase` `ready` olmalı.

- Hızlı denetimler: Ekran Kaydı izni (`granted` değilse `-32006`, `data.permission = "screen_recording"`), platform desteği (`-32007`).
- Kontrol onaylı ama Erişilebilirlik izni yoksa başlatma sürer, `control = pending_permission` olur; izin gelince (`permissions.status` yeniden denetlenince) `active`'e geçer.
- Yanıt hemen döner: `{"state": <state>}` (`phase = starting`). Bağlantı ve yakalama sonucu `state` (`active`) ya da `error` olayıyla gelir.

### `session.stop`

Oturumu bitirir (`POST /v1/public/remote-support/{uuid}/end`). Her `phase`'te çağrılabilir; `resolved`'da onay vermeden vazgeçmek sunucuda `declined` sayılır. Kapı hemen kapanır, basılı tuş ve düğmeler bırakılır, yakalama durur, LiveKit bağlantısı kapanır. API'ye ulaşılamazsa yerelde yine biter (`ended_reason = connection_lost`) ve hata olayı gönderilir.

- Parametre: `{}`.
- Yanıt: `{"state": <state>}` (`phase = ended`, `summary` dolu).
- Kabuğun genel kısayolu (`Ctrl+Alt+Shift+S`) ve pencerenin kapanması da bunu çağırmalıdır.

### `control.revoke`

"Kontrolü geri al". Kapı **önce yerelde** kapanır ve basılı her şey bırakılır; sonra API'ye `control: false` gider (sunucu temsilcinin veri iznini kaldırır). API hatası kapıyı yeniden açmaz.

- Parametre: `{}`. Yanıt: `{"state": <state>}` (`control = revoked`).
- Kontrol yürürlükte değilse hata vermez, durumu döner.

### `permissions.status`

İzinleri yeniden denetler ("Yeniden denetle"). İstem penceresi açmaz.

```json
{"platform": "darwin", "screen_recording": "granted", "accessibility": "denied", "restart_required": false}
```

| Değer | Anlam |
|---|---|
| `granted` | Var |
| `denied` | Yok (macOS "sorulmadı" ile "reddedildi"yi ayırmaz) |
| `not_required` | Bu platformda izin gerekmez |
| `unsupported` | Platform desteklenmiyor (Windows: yakalama henüz yok) |

- `restart_required`: Ekran Kaydı izni bu süreç açıldıktan sonra verildiyse macOS izni ancak uygulama yeniden açılınca tanır. Sidecar bunu `permissions.request` sonrası izni hâlâ göremiyorsa true yapar; arayüz "Çık ve Yeniden Aç" adımını gösterir.
- Durum değiştiyse ayrıca `state` olayı gelir.

### `permissions.request` (ek)

macOS izin istemini açar. İzinler sidecar'ı başlatan uygulamaya bağlandığı için istem sidecar'dan gelmelidir.

```json
{"kind": "screen_recording"}
{"kind": "accessibility"}
```

- Yanıt: `permissions.status` biçimi.
- Sistem Ayarları'nı açmak kabuğun işidir: `x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture` ve `…?Privacy_Accessibility`.

### `session.report` (ek)

"Bir sorun mu vardı? Bildir" (`POST /v1/public/remote-support/{uuid}/report`). Oturum sürüyorsa biter. `ended`'da da çağrılabilir (aynı oturum için).

- Parametre: `{"reason": "…"}` (isteğe bağlı, en çok 1000 karakter).
- Yanıt: `{"state": <state>}` (`summary.reported = true`).

### `sysinfo.get` (ek)

Onay ekranında "paylaşılacak bilgiler"i göstermek için; hiçbir yere göndermez.

```json
{"os": "macOS", "os_version": "27.2", "arch": "arm64", "app_version": "0.2.0", "screen": {"width": 2940, "height": 1912, "scale": 2}}
```

## Olaylar

| Olay | Ne zaman | Biçim |
|---|---|---|
| `ready` | Sidecar açılınca (her modda) | `{"mode","version"}` |
| `state` | Her durum değişikliğinde | Yukarıdaki anlık görüntü |
| `control` | Kontrol durumu değişince ve kapı bildirimlerinde | aşağıda |
| `error` | Bir yöntemin yanıtına bağlı olmayan hata (bağlantı, yakalama, arka plan API çağrısı) | aşağıda |

### `control`

```json
{"state": "revoked", "previous": "active", "by": "customer", "at": "2026-11-16T14:22:10Z"}
{"state": "active", "previous": "active", "notice": "blocked_shortcut", "at": "…"}
```

- `by`: `customer`, `agent` (temsilci ayrıldı ya da veri izni gitti), `system` (bağlantı koptu, izin kalktı, oturum bitti).
- `notice` (isteğe bağlı, durum değişmeden): `blocked_shortcut` (engelli bir kısayol atıldı; 5 sn'de en çok bir kez), `rate_limited` (saniyede 200 olay aşıldı; oturumda bir kez), `accessibility_missing` (girdi geldi ama izin yok; oturumda bir kez).

### `error`

```json
{"code": -32005, "kind": "NETWORK", "message": "Sunucuya ulaşılamıyor. Bağlantınızı denetleyin.", "fatal": false}
```

- `fatal: true` ise oturum bitmiştir (ardından `phase = ended` gelir).

## Hata kodları

Yöntem hatası `{"code", "message", "data": {"kind", ...}}` taşır. Kabuk (`CallError::Rpc`) yalnız `code` ve `message`'ı iletir; arayüz **koda** göre dallanmalıdır. `message` müşteriye gösterilebilir Türkçe metindir.

| Kod | `kind` | Anlam |
|---|---|---|
| -32602 | `INVALID_PARAMS` | Parametre eksik ya da hatalı |
| -32603 | `INTERNAL` | Beklenmeyen hata |
| -32001 | `CODE_INVALID` | Kod ya da bağlantı geçersiz, kullanılmış ya da süresi dolmuş (API 404) |
| -32002 | `RATE_LIMITED` | Çok fazla deneme (API 429) |
| -32003 | `ENDED` | Oturum bitmiş (API 410) |
| -32004 | `UNAVAILABLE` | Uzaktan destek bu işletmede kapalı ya da askıya alınmış (API 403) |
| -32005 | `NETWORK` | Sunucuya ulaşılamıyor ya da zaman aşımı |
| -32006 | `PERMISSION_REQUIRED` | macOS izni eksik; `data.permission`: `screen_recording` \| `accessibility` |
| -32007 | `UNSUPPORTED` | Bu platformda desteklenmiyor (ör. Windows'ta ekran yakalama) |
| -32008 | `INVALID_STATE` | Yöntem bu `phase`'te çağrılamaz |
| -32009 | `SERVER` | Sunucu hatası (5xx ya da beklenmeyen yanıt) |
| -32010 | `UNAUTHORIZED` | Oturum sırrı geçersiz (API 401) |
| -32011 | `CONFLICT` | Sunucu çakışma bildirdi (API 409), ör. izleme onayı yokken token |
| -32012 | `CAPTURE_FAILED` | Ekran yakalama ya da kodlama başlatılamadı |
| -32013 | `LIVEKIT` | Medya sunucusuna bağlanılamadı |

## Yapılandırma

- API kökü `CLICKFIX_APP_URL` ortam değişkeninden gelir (kabukla aynı kural: https; yalnız localhost için http). Varsayılan `https://getclickfix.com`. Çağrılar `<kök>/api/v1/public/remote-support/...` adresine gider.
- `--log-dir`: günlük dizini. Verilmezse işletim sisteminin önbellek dizini altında `ClickFix/sidecar-logs`. Dosya 5 MB'da döner, 3 eski dosya tutulur. Kod, token ve sırlar maskelenir; aynı satırlar stderr'e de (kabuk günlüğüne) gider.
