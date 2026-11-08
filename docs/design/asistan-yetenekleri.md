# Asistan yetenekleri (CF-183)

ClickFix Asistan (CF-141) bugün sayfayı anlatıyor, doküman arıyor ve küçük bir eylem listesinden öneri yapıyor. Bu doküman, asistanın her sayfada ve her modülde **iş yapan** bir yardımcıya dönüşmesinin tasarımıdır: kullanıcı ne istediğini yazar, asistan eksik bilgiyi sorar, eylemi önerir, kullanıcı onaylar, asistan uygular ve sonucu doğrular.

Bağlayıcı kabul ölçütleri issue #81'deki beş senaryodur. Alt işler #82–#92 (CF-184–CF-194) olarak açıldı; aşama planı en sondadır.

## 0. Bugünkü durum

| Parça | Yer | Not |
|---|---|---|
| Asistan servisi | `backend/internal/modules/assistant` | Araçlar `search_docs`, `get_setup_state`, `propose_action`, `escalate_to_support`. Öneriler `assistant_messages.actions` JSON'unda durur, `confirm` uç noktası kullanıcının yetkisiyle çalıştırır. |
| Eylem listesi | `assistant/actions.go` (`Registry`, `Executor`) | `open_route`, `connect_channel`, `enable_ai`, `create_channel_email`, `install_widget`, `invite_member`, `set_business_hours`, `create_sla`, `escalate_to_support`. Parametre şeması yalnız düz metin açıklama; doğrulama elle yazılmış. |
| MCP kataloğu | `backend/internal/modules/mcp/catalog.go` | Altı araç (konuşmalar, kişiler). Yetki slug'ı, `Write` bayrağı, `/v1` isteği kurucusu. Çalıştırma `apibridge` ile süreç içinde, kullanıcının kimliğiyle. |
| API köprüsü | `backend/internal/platform/apibridge` | `/v1` yolunu kullanıcının JWT'si ve `X-Organization` başlığıyla süreç içinde çağırır. Yolun kendi yetki, modül, doğrulama ve denetim kaydı kuralları aynen işler. |
| Organizasyon AI ajanı | `backend/internal/modules/aiagents` | Müşteriye yanıt veren ajan. Araç çağırma, onay ve kuru çalıştırma kalıbı buradan okunur; kapsam dışıdır. |
| Sayfa rehberleri | `frontend/src/features/guides`, `backend/internal/modules/guides` | Rota → rehber eşlemesi, `data-guide="<hedef>"` öznitelikli alanlar, tur katmanı. |

İki sorun var. Asistan eylemleri ile MCP araçları iki ayrı katalogdur. Eylemlerin çoğu da yalnız sayfaya yönlendirir; gerçek iş yapmaz.

## 1. Yetenek modeli

### 1.1 Tek arayüz

Her modül yeteneklerini tek bir tipte tanımlar. Paket: `backend/internal/platform/capability` (platform katmanında, böylece hem `assistant` hem `mcp` hem de modüller içe aktarabilir).

```go
type Kind string // "read" | "write" | "destructive"

type Capability struct {
    Name        string   // "channels.api.create": modül.nesne.fiil, MCP'de "_" ile
    Module      string   // "channels", "automation", ... (sayfa eşlemesi ve envanter)
    Title       string   // kart başlığı için kısa ad (en); i18n anahtarı TitleKey
    TitleKey    string   // "assistant.cap.channels_api_create"
    Description string   // modele giden açıklama (en)
    Surfaces    []string // "web", "platform", "widget"; boş = yalnız "web"
    Permission  string   // tenant.* slug'ı; boş = her üye
    Kind        Kind
    Input       *jsonschema.Schema // girdi şeması; kayıtta bir kez çözülür
    Secrets     []Secret           // gizli alanlar: modele gösterilmez, modelden kabul edilmez
    Routes      []string           // ilgili sayfalar (rota önekleri) — sıralama ve rehber için
    Focus       *Focus             // "sayfaya git ve vurgula" hedefi (rota + data-guide)

    Validate func(Params) error                       // şemanın ötesindeki kurallar
    Read     func(ctx, Exec, Params) (Result, error)  // read yetenekleri
    DryRun   func(ctx, Exec, Params) (Result, error)  // isteğe bağlı; AĞA ÇIKMAZ, veri değiştirmez
    Apply    func(ctx, Exec, Params) (Result, error)  // write/destructive
    Verify   func(ctx, Exec, Params, Result) (Check, error) // isteğe bağlı; uygulama sonrası durum denetimi
    Undo     string // geri alma: başka bir yeteneğin adı ya da insan okunur yol ("Kanalı kapat")
}
```

Kurallar:

- `read`: veri değiştirmez. Asistanda doğrudan araçtır; model çağırır, sonuç modele döner.
- `write`: veri değiştirir. Asistanda yalnız **öneri** üretir; onay kartı olmadan hiçbir şey çalışmaz. MCP'de `mcp:write` kapsamı ister.
- `destructive`: silme, kapatma, iptal, anahtar döndürme. `write` kurallarına ek olarak kart kırmızı uyarı gösterir, MCP'de `destructiveHint`; gerekiyorsa adım yükseltme (`step-up-engine`).
- Onay gereksinimi türden türetilir: `write` ve `destructive` her zaman onaylı. Ayrı bir "onaysız yazma" bayrağı yoktur.
- `DryRun` ağa çıkmaz ve veri yazmaz. Yerel doğrulama yapar ve "ne olacak" önizlemesi döner. Dış uç noktaya istek atan testler (`channels.test`) ayrı bir `write` yeteneğidir.
- `Verify`, `Apply`'dan sonra durumu yeniden okur (ör. kanal sağlık denetimi, iş akışının yayında olması). Sonuç kartta "Doğrulandı / Doğrulanamadı" satırıdır; doğrulama başarısızsa eylem geri alınmaz, kullanıcıya ne yapacağı söylenir.

`Exec` yürütme bağlamıdır: organizasyon kapsamı, kullanıcı, yerel ayar ve bir `API` arayüzü. Uygulamalar mümkün olduğunca `API` üzerinden `/v1` çağırır (`apibridge`). Böylece yolun kendi yetki, modül hakkı (entitlement), doğrulama ve `activity` kaydı aynen çalışır; yetenek, panelin yapamadığı hiçbir şeyi yapamaz.

### 1.2 Girdi şeması

Şema `github.com/google/jsonschema-go/jsonschema` tipidir (takwim-app-go `modules/mcp/catalog.go` ile aynı). Kayıtta bir kez çözülür; bozuk şema başlangıçta paniğe yol açar. Modelden gelen her parametre önce şemayla, sonra `Validate` ile denetlenir. `additionalProperties: false` zorunludur.

Form ipuçları şemada standart alanlarla taşınır: `title` (etiket), `description` (yardım), `enum`, `format: "uri"`, `maxLength`. Kart formu bunlardan üretilir.

### 1.3 Gizli alanlar

`Secret{Key, Generate}`:

- Gizli alan modele giden şemada **yoktur**. Model gönderirse çağrı reddedilir ("gizli değerler sohbetten alınmaz").
- `Generate` doluysa değer sunucuda, **uygulama anında** üretilir (ör. REST API kanalının imza anahtarı). Değer yalnız onay yanıtında `secret_once` olarak bir kez döner; mesajlara, `actions` JSON'una, denetim kaydına, loga ve modele yazılmaz.
- Kullanıcı kendi değerini girmek isterse kart formunda parola alanı olarak girer. Değer onay isteğinin gövdesinde gelir, yalnız `Apply`'a geçer, saklanmaz.
- Kullanıcının sohbete yapıştırdığı anahtar biçimli metinler (`sk-…`, `Bearer …`, 32+ karakterlik onaltılık/base64 dizgiler, `secret=`, `token=`, `password=`) modele gitmeden ve saklanmadan önce maskelenir.

### 1.4 Ortak kaynak: tek tanım, iki sunum

| | Asistan | MCP |
|---|---|---|
| `read` | araç; sonuç modele | `tools/call`, `readOnlyHint: true` |
| `write` | öneri aracı → onay kartı → `confirm` | `tools/call`, `mcp:write` kapsamı, istemci onayı |
| `destructive` | kırmızı kart, gerekirse adım yükseltme | `destructiveHint: true` |
| Gizli alan | şemada yok; sunucu üretir / kart formu | şemada yok; sunucu üretir, sonuçta bir kez |
| Yürütme | `apibridge` | `apibridge` (bugünkü gibi) |

MCP kataloğu CF-192'de defterden üretilir; o zamana kadar iki sunum yan yana durur ama yeni araçlar yalnız deftere yazılır.

### 1.5 Asistan döngüsü

1. İstek gelir: `Caller` (kapsam, yetkiler), `Context{surface, route}`.
2. Defter süzülür: yüzeyde izinli **ve** kullanıcının yetkisi olan yetenekler. Sayfaya uyanlar (`Routes`) öne alınır ve sistem mesajında "bu sayfada" diye işaretlenir. Araç sayısı üst sınırı aşarsa (24) önce sayfadakiler kalır.
3. Model araçları: sabit dört araç + her `read` yeteneği bir araç + her `write`/`destructive` yeteneği bir **öneri aracı** (aynı ad; açıklaması "kullanıcıya onay kartı olarak gösterilir" ile başlar).
4. Öneri aracı çağrılınca: şema + `Validate`; geçerse öneri kaydedilir (`ProposedAction`), modele "kart gösterildi, kullanıcıyı bekle" döner. Geçmezse modele düzeltilebilir hata döner ("webhook_url gerekli; kullanıcıya sor").
5. Kullanıcı kartta parametreleri düzenler, isterse **Kuru çalıştır**, sonra **Onayla**.
6. `confirm`: eylem hâlâ beklemede mi, yetenek hâlâ var mı, kullanıcının yetkisi var mı (modelin değil), düzenlenmiş parametreler şema + `Validate`'ten geçiyor mu; sonra `Apply`, `Verify`, denetim kaydı, sonuç mesajı.

Eski `Executor` eylemleri bir bağdaştırıcıyla deftere girer; CF-188/CF-189 bunları birer birer gerçek yeteneklere çevirir.

## 2. Sayfa bağlamı

Rota → modül eşlemesi defterin `Routes` alanından gelir (`/channels` → `channels`, `/automation/workflows` → `automation`). Sayfa açıldığında asistanın bildikleri:

- **Sayfa rehberi**: `guides.Page(surface, route)` (bugün de var).
- **İlgili yetenekler**: o modülün yetkiye göre süzülmüş yetenekleri.
- **Sayfa durumu** (CF-187): modül başına bir `PageState` sağlayıcısı.

```go
type PageState struct {
    Module   string
    Missing  []Step  // kullanıcının eksik bıraktıkları: {key, title, capability, focus}
    Warnings []Step  // ör. "kanal hata durumunda", "iş akışı taslakta, yayında değil"
    Tests    []string // bu sayfada çalıştırılabilecek test yetenekleri
}
```

Bugünkü `get_setup_state` organizasyon genelinde eksikleri verir; sayfa durumu bunu modül düzeyine indirir (ör. kanallar sayfasında "e-posta kanalının alan adı doğrulanmadı"). Asistan açıldığında ilk öneri sohbeti bu listeden kurulur ("Bu sayfada 2 eksik var").

## 3. Onay kartı v2

`ProposedAction` alanları genişler (geriye uyumlu):

| Alan | Anlamı |
|---|---|
| `capability` | yetenek adı (eski `kind` korunur ve aynı değeri taşır) |
| `risk` | `write` / `destructive` |
| `form` | düzenlenebilir alanlar: `key`, `label`, `type` (`string`/`integer`/`boolean`/`enum`/`secret`/`json`), `required`, `options`, `help` |
| `dry_run` | kuru çalıştırma var mı |
| `preview` | son kuru çalıştırmanın sonucu (`summary`, `items`) |
| `verification` | `ok` / `failed` / `skipped` + kısa metin |
| `details` | uygulama sonucu: başlık, satırlar, kod blokları (örnek istek) |
| `focus` | `{route, target}`: "sayfaya git ve vurgula" |

Uç noktalar:

- `POST /v1/tenant/assistant/threads/{uuid}/actions/{action_id}/dry-run` gövde `{params?}`: düzenlenmiş parametrelerle kuru çalıştırır, sonucu karta yazar. Veri değişmez.
- `POST .../confirm` gövde `{params?}` (isteğe bağlı): düzenlenmiş parametrelerle uygular. Yanıt mesajı + bir kez `secret_once`.

**Sayfaya git ve vurgula**: `open_route` yeteneğine isteğe bağlı `focus` parametresi eklenir. Rota yalnız iç yol olabilir (`^/[a-z0-9/_\-\[\]]{0,150}$`, `//` ile başlayamaz, şema yok, sorgu ya da parça yok). Hedef `^[a-z0-9][a-z0-9_.-]{0,59}$`. Ön yüz `?focus=<hedef>` ile gider; ortak `FocusHighlighter` bileşeni `[data-guide="<hedef>"]` öğesini bulur (rehber turuyla aynı öznitelik), görünür alana kaydırır ve birkaç saniye halka ile vurgular. Hedef bulunamazsa sessizce geçer. Kullanıcı sohbette "şu düğmeyi göster" dediğinde model, sayfa rehberindeki hedef listesinden seçer; liste dışı hedef reddedilmez ama vurgulanamaz.

## 4. Dosya ve bağlantı girdisi (CF-191)

Senaryo 1'de yazılımcı kendi API dokümanını paylaşır.

- **Dosya**: en fazla 2 MB; `text/plain`, `text/markdown`, `application/json`, `application/yaml`, `application/pdf`. Yalnız metin çıkarılır, en fazla 60 000 karakter, kalanı kesilir. Dosya `attachments` deposunda organizasyon kapsamlı saklanır, 7 gün sonra silinir.
- **Bağlantı**: yalnız `https`. Çekme `platform/netguard` ile korunur: özel, yerel, bağlantı-yerel ve meta veri adresleri reddedilir; her yönlendirmede ve bağlanma anında IP yeniden denetlenir (DNS yeniden bağlama); en fazla 3 yönlendirme, 10 sn, 2 MB.
- Çıkarılan metin modele `ConversationData` sarmalıyla **veri** olarak gider; içindeki talimatlar uygulanmaz.
- Gizli değer maskeleme (§1.3) dosya ve bağlantı metnine de uygulanır.
- Model dokümandan parametre çıkarır (webhook adresi, kimlik doğrulama türü, örnek gövde); bunlar yalnız onay kartında ön dolgu olarak görünür. Dokümandaki anahtarlar hiçbir zaman ön dolgu olmaz.

## 5. Modül başına yetenek envanteri

Öncelik: P0 = aşama 1, P1 = aşama 2–3, P2 = sonrası.

### 5.1 Kanallar (P0, CF-185)

| Yetenek | Tür | Yetki | Not |
|---|---|---|---|
| `channels.list` | read | `tenant.channels.read` | uuid, tür, ad, durum, sağlık, gelen webhook adresi; kimlik bilgisi ipuçları **yok** |
| `channels.api.sample_request` | read | `tenant.channels.read` | gelen istek için `curl` örneği, imza tarifi, giden yük biçimi; anahtar yer tutucu (`$CLICKFIX_SIGNING_SECRET`) |
| `channels.api.create` | write | `tenant.channels.manage` | `{name, webhook_url}`; imza anahtarı sunucuda üretilir (`secret_once`); kuru çalıştırma: URL biçimi + SSRF kuralları + aynı adda kanal var mı; doğrulama: sağlık denetimi |
| `channels.api.update` | write | `tenant.channels.manage` | `{channel_uuid, name?, webhook_url?, rotate_secret?}`; `rotate_secret` destructive sayılır (eski anahtar çalışmaz) |
| `channels.test` | write | `tenant.channels.manage` | sağlık denetimi (`POST /v1/tenant/channels/{uuid}/check`); dış uca gider, onaylıdır |
| `channels.connect` | write | `tenant.channels.manage` | eski `connect_channel`: diğer kanal türlerinin bağlantı sayfasına yönlendirir (OAuth ve sağlayıcı formu asistan dışında) |
| `channels.email.test` | write | `tenant.channels.manage` | P1 (CF-187), e-posta test gönderimi |

### 5.2 İş akışları (P0, CF-186)

| Yetenek | Tür | Yetki | Not |
|---|---|---|---|
| `workflows.list` | read | `tenant.automations.read` | ad, revizyon, yayın, etkin |
| `workflows.reference` | read | `tenant.automations.read` | tetikleyiciler, koşul alanları, işleçler, eylem türleri ve gerekleri |
| `workflows.dry_run` | read | `tenant.automations.write` | kayıtlı iş akışını bir konuşma üzerinde simüle eder (varsayılan: son konuşma); veri değişmez |
| `workflows.create` | write | `tenant.automations.write` | doğrusal taslak `{name, trigger, schedule_minutes?, condition?, actions[], enable}` → sunucu grafiği kurar, `ValidateGraph`; kuru çalıştırma: grafiği yerelde doğrular ve adımları anlatır; doğrulama: kayıt + (etkinse) yayın + son konuşmada simülasyon |
| `workflows.enable` | write | `tenant.automations.write` | aç/kapat |

### 5.3 Diğer modüller

| Modül | Yetenekler | Öncelik | Alt issue |
|---|---|---|---|
| Otomasyon kuralları | listele, taslak, kaydet, kuru çalıştır, aç/kapat, sil (destructive) | P1 | CF-188 |
| SLA | politika oluştur/güncelle (eski `create_sla`), kuru çalıştır | P1 | CF-188 |
| Çalışma saatleri | oluştur/güncelle (eski `set_business_hours`) | P1 | CF-188 |
| Ekip | davet (eski `invite_member`), takım oluştur, üye ata | P1 | CF-189 |
| AI ayarları | durum, aç (eski `enable_ai`), kapat, ajan listesi | P1 | CF-189 |
| Widget kurulumu | oluştur, renk/karşılama, kurulum kodu, alan adı listesi | P1 | CF-189 |
| Webhook | abonelik oluştur/güncelle, test teslimatı | P2 | CF-190 |
| API anahtarı | oluştur (`secret_once`), iptal (destructive, adım yükseltme) | P2 | CF-190 |
| Entegrasyonlar | durum, bağlantı sayfasına yönlendirme | P2 | CF-190 |
| Gezinme | `open_route` (+`focus`) | P0 | CF-184 |
| Destek | `escalate_to_support` (değişmez) | P0 | — |

## 6. Yüzeyler ve izin listeleri

| Yüzey | Kim | Yetenekler |
|---|---|---|
| `web` (çekmece) | organizasyon üyesi | yetkiye göre tümü |
| `platform` (platform yöneticisi sayfası) | platform personeli | yalnız `read` ve platform ayarları; organizasyon verisine yazma yok |
| `widget` | organizasyonun **kimliği doğrulanmış** son kullanıcısı | açık izin listesi: `open_route` (yalnız widget rotaları), yardım merkezi araması, destek talebi; ziyaretçiye (kimliksiz) hiçbir `write` yeteneği verilmez |

Varsayılan kapalıdır: `Surfaces` boşsa yetenek yalnız `web`'dedir. Widget yüzeyi CF-193'te açılır; organizasyon ayarında widget için izin verilen yetenekler listelenir ve platform listesinin alt kümesi olmak zorundadır.

## 7. Güvenlik modeli ve denetim kaydı

1. **Yetki modelden gelmez.** Süzme kullanıcının kapsamından yapılır; onayda yeniden denetlenir; `/v1` yolu (`apibridge`) üçüncü kez denetler. Asistan kullanıcının yapamadığını yapamaz.
2. **Onaysız yazma yok.** `write`/`destructive` yetenekler yalnız öneri üretir; `Apply` yalnız `confirm` uç noktasından çağrılır. Onay, aynı iş parçacığında, aynı kullanıcının, beklemedeki bir eylemine bağlıdır.
3. **Parametre doğrulama.** Şema (`additionalProperties: false`, tür, uzunluk, enum, desen) + `Validate`. Kullanıcının düzenlediği parametreler de aynı denetimden geçer.
4. **Açık yönlendirme yok.** Rota yalnız iç yol; ön yüz `router.push` ile yalnız `/` ile başlayan yolu açar.
5. **SSRF.** Kullanıcı ya da model kaynaklı her URL `channelhttp.ValidateURL` (şema, yerel adlar, özel IP) ile; ağ çağrısı yapan yerler ayrıca `netguard`'lı istemciyle.
6. **Gizli değerler** (§1.3): sohbete, `actions` JSON'una, loga, Sentry'ye, denetim kaydına ve model bağlamına yazılmaz.
7. **Enjeksiyon.** Araç sonuçları, dokümanlar ve müşteri metni veridir; `ai.Guard` ve sistem talimatları korunur. Model metni hiçbir zaman SQL, yol ya da kabuk komutuna doğrudan girmez; yollar sabit şablon + doğrulanmış UUID ile kurulur.
8. **Hız ve maliyet.** Var olan dakika/gün sınırları geçerli; `dry-run` ve `confirm` için iş parçacığı başına dakikada 20 istek.

**Denetim kaydı** (`activity.Recorder`, kaynak `assistant_thread`):

| Olay | Ne zaman | Yük |
|---|---|---|
| `assistant.action_dry_run` | kuru çalıştırma | `capability`, `outcome` |
| `assistant.action_confirmed` | onay sonrası | `capability`, `kind`, `status`, `verification`, maskelenmiş parametreler |
| `assistant.action_rejected` | kullanıcı reddetti | `capability` |
| `assistant.action_forbidden` | yetkisiz onay denemesi | `capability` |

Yükte gizli alanlar hiç yer almaz; diğer alanlar `activity.Scrub`'dan geçer. Yeteneğin çağırdığı `/v1` yolu kendi denetim kaydını ayrıca yazar (ör. `channels.connected`), aktör onaylayan kullanıcıdır.

## 8. Aşama planı

| Aşama | Alt issue | Kapsam | Efor | Bağımlılık |
|---|---|---|---|---|
| 0 | CF-183 (#81) | bu doküman | 0,5 gün | — |
| 1a | CF-184 (#82) | çekirdek defter, onay kartı v2, odaklanma, üç eylemin deftere taşınması | 3 gün | 0 |
| 1b | CF-185 (#83) | kanallar (senaryo 1) | 2 gün | 1a |
| 1c | CF-186 (#84) | iş akışları (senaryo 2) | 2 gün | 1a |
| 2 | CF-187 (#85) | sayfa durumu, "neyi eksik bıraktın", sayfa içi testler (senaryo 3) | 3 gün | 1a |
| 2 | CF-191 (#89) | dosya ve bağlantı girdisi | 3 gün | 1a |
| 2 | CF-192 (#90) | MCP kataloğunu defterden üretme | 2 gün | 1a |
| 3 | CF-188 (#86) | otomasyon kuralları, SLA, çalışma saatleri | 2 gün | 1a |
| 3 | CF-189 (#87) | ekip, AI ayarları, widget kurulumu | 2 gün | 1a |
| 4 | CF-190 (#88) | webhook, API anahtarı, entegrasyonlar | 2 gün | 1a, CF-191 |
| 5 | CF-193 (#91) | widget yüzeyi (senaryo 5) | 3 gün | 1a, CF-187 |
| 6 | CF-194 (#92) | uçtan uca senaryo testleri | 2 gün | 1b, 1c, CF-187, CF-193 |

Aşama 1'in üç PR'ı `cf-toplu` tabanına ayrı açılır (`cf-183-cekirdek`, `cf-183-kanallar`, `cf-183-is-akislari`). Kanallar ve iş akışları PR'ları çekirdek PR'ın üzerine kurulur.

## 9. Karar gerekenler

Varsayılanlar uygulanır; değiştirmek isterseniz issue'ya yazın.

1. **İmza anahtarının gösterimi.** Varsayılan: sunucu üretir, onay yanıtında bir kez gösterilir, sonra yalnız döndürülerek (rotate) yenilenebilir. Alternatif: kullanıcı kendi anahtarını girer (kart formunda da mümkün).
2. **Sayfa içi testlerin onayı.** Varsayılan: dış uca istek atan her test (`channels.test`, e-posta test gönderimi, webhook test teslimatı) onaylıdır. Alternatif: yalnız okuma sayılıp onaysız çalışması.
3. **Kayıtlı iş akışının kuru çalıştırması.** Varsayılan: onaysız `read` (veri değiştirmez), ama `tenant.automations.write` ister.
4. **Araç sayısı üst sınırı.** Varsayılan 24; aşarsa sayfa dışı yetenekler düşer. Model maliyeti ölçülünce yeniden bakılır.
5. **Widget yüzeyinin varsayılanı.** Varsayılan: kapalı; organizasyon açıkça izin vermedikçe widget'ta yalnız gezinme ve yardım araması.
6. **Eski eylemler.** Varsayılan: bağdaştırıcıyla çalışmaya devam; CF-188/CF-189'da yeteneğe çevrilir.
