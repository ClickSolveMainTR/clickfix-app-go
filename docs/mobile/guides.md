# Mobil rehberler, ClickFix Asistan ve sallayarak geri bildirim (CF-142)

Mobil uygulama, CF-141'in API sözleşmesini (`/v1/tenant/guides*`, `/v1/tenant/assistant/*`, `/v1/tenant/product-feedback`) kullanır. Bu belge rehber içeriği yazanlar (platform yöneticisi, `POST /v1/platform/guides/generate`) için mobilin beklediği **rota kalıplarını** ve **hedef kimliklerini** listeler.

## Davranış

- **Kurulum turu**: oturum açıldıktan sonra Gelen kutusu ekranında `GET /v1/tenant/guides/setup` sıradaki bitmemiş rehberi (`new` / `in_progress`) başlatır; organizasyon başına bu telefonda bir kez kendiliğinden gelir. Bir rehber bitince sıradaki başlar. Daha fazla → Yardım → "Kurulum rehberi" ve Ayarlar'dan her zaman açılır.
- **Ekran turu**: bir ekrana ilk girişte o rotanın `kind: "tour"` ve `progress.status: "new"` olan ilk rehberi (en küçük `order`) başlar.
- **"Bu ekran ne işe yarar?"**: her ekranın başlığındaki "?" menüsü; rotanın `kind: "page"` rehberini, yoksa ilk turunu oynatır.
- **İlerleme**: her adımda `PUT /v1/tenant/guides/{key}/progress` (`in_progress`, `step_index`), sonunda `completed`, kapatınca `skipped`. Ayarlar → "Rehberleri yeniden göster" `POST /v1/tenant/guides/reset` çağırır.
- **Adım**: `target` boşsa veya hedef ekranda yoksa kart ortada açılır. `placement` `top` / `bottom` sığarsa uygulanır; `left` / `right` telefonda `auto` gibi davranır. `action.kind: "navigate"` "İleri"de `action.route`'a gider (mobil rota, örn. `/more/settings`); `open_assistant` adımlarında "Asistana sor" bağlantısı asistanı adım bağlamıyla açar.
- **404**: sunucuda rota yoksa (CF-141 henüz yayında değilse) ilgili giriş noktaları gizlenir.

## Rota kalıpları (`route`)

expo-router segmentlerinden gruplar ve `index` atılarak üretilir.

| Ekran | `route` |
|---|---|
| Gelen kutusu | `/inbox` |
| Konuşma | `/conversation/[uuid]` |
| Konuşma ayrıntıları | `/conversation-details/[uuid]` |
| Kişiler | `/contacts` |
| Kişi / şirket | `/contact/[uuid]`, `/company/[uuid]` |
| Ajanda | `/agenda` |
| Bildirimler | `/notifications` |
| Daha fazla | `/more` |
| Ayarlar | `/more/settings` |
| Diğer Daha fazla ekranları | `/more/<ad>` (`team`, `team-chat`, `oncall`, `reports`, `engineering`, `incidents`, `channels`, `members`, `ai`, `billing`, `video`, `sessions`, `notifications`) |
| Arama | `/search` |
| ClickFix Asistan | `/assistant` |
| Kurulum rehberi | `/setup-guide` |

## Hedef kimlikleri (`target`)

| Ekran | Kimlik | Ne |
|---|---|---|
| Gelen kutusu | `inbox.search` | arama alanı |
| | `inbox.views` | görünüm seçici |
| | `inbox.tabs` | görünüm ve durum sekmeleri |
| | `inbox.filters` | başlıktaki filtre düğmesi |
| | `inbox.select` | toplu seçim düğmesi |
| | `inbox.row` | ilk konuşma satırı |
| | `inbox.swipe` | ilk satır (kaydırma işlemlerini anlatmak için) |
| Konuşma | `conversation.timeline` | mesaj akışı |
| | `conversation.composer` | yanıt alanı |
| | `conversation.mode` | Yanıt / İç not geçişi |
| | `conversation.copilot` | AI copilot düğmesi |
| | `conversation.status` | çöz / yeniden aç |
| | `conversation.details` | ayrıntılar |
| | `conversation.menu` | işlem menüsü |
| Kişiler | `contacts.search`, `contacts.tabs`, `contacts.row` | arama, Kişiler/Şirketler, ilk satır |
| Ajanda | `agenda.scope`, `agenda.list` | benim / organizasyon, randevu listesi |
| Bildirimler | `notifications.filter`, `notifications.row` | Tümü / Okunmamış, ilk bildirim |
| Daha fazla | `more.profile` | kullanıcı ve organizasyon |
| | `more.find`, `more.team`, `more.work`, `more.admin`, `more.help`, `more.app` | bölüm grupları |
| Ayarlar | `settings.help` | Yardım ve geri bildirim bölümü |

Yeni hedef eklemek: görünümü `GuideTarget` ile sarın ya da `ref={useGuideTarget('<kimlik>')}` verin (`mobile/src/ui/guide-target.tsx`); başlık düğmesinde `HeaderButton guide="<kimlik>"`. Kimliği bu tabloya ekleyin.

## ClickFix Asistan

Daha fazla → Yardım, her ekranın "?" menüsü (rota bağlam olarak gider) ve rehber adımlarındaki "Asistana sor". İlk mesajda `POST /threads` (`context: { surface: "mobile", route, guide_key? }`), sonra `POST /threads/{uuid}/messages` (yanıt aynı çağrıda döner). Önerilen işlemler (`actions`) yerel onay penceresinden sonra `confirm`, ya da `reject`; işlem üyenin kendi yetkileriyle çalışır. Geçmiş sohbetler `GET /threads`; destek ekibine iletme `POST /threads/{uuid}/escalate`. `status.enabled: false` iken neden gösterilir, günlük sınır dolunca yazma kapanır (429 da aynı mesajı verir).

## Sallayarak geri bildirim

`expo-sensors` ivmeölçeri yalnızca uygulama ön plandayken, ayar açıkken ve rehber / kilit ekranı yokken dinlenir; ~0,9 sn içinde üç sert sarsıntı bir sallamadır (`mobile/src/features/feedback/shake-core.ts`). Sallayınca `react-native-view-shot` ile ekran görüntüsü alınır, form açılır: görüntü önizlemesi (kaldırılabilir), tür (Hata / Öneri / Diğer), not, cihaz bilgisi anahtarı. Görüntü önce `POST /v1/tenant/attachments` ile yüklenir (`tenant.attachments.write` yoksa yalnız metin gider), sonra `POST /v1/tenant/product-feedback` (`route`, `app_version`, isteğe bağlı `device`, `attachment_uuids`). Ayarlar → "Sallayarak geri bildirim" kapatır; Daha fazla ve "?" menüsünde "Geri bildirim gönder" de var.

Yerel modüller (dev client yeniden derlenmeli): `expo-sensors`, `react-native-view-shot`, `expo-blur`.

## main sonrası UI entegrasyonu

Asistan ve inbox aynı `ui/message-composer.tsx` ile `features/inbox/thread/message-row.tsx` balon yüzeyini kullanır; asistan yanıtları ortak Markdown okuyucusunda kod ve yatay kaydırılabilir tablolarla çizilir. Geçmiş, destek aktarımı ve geri bildirim `ui/form-sheet` kullanır. Ekran görüntüsüne dokunmak inbox'ın `image-markup-editor` bileşenini açar. Native `ui/menu` yardım menüsündeki simgeleri iOS'ta SF Symbols ile gösterir.

PR #153 seed hedefleri için `inbox-filter` → `inbox.views`, `composer` → `conversation.composer` takma adları ölçüm katmanında tutulur; `assistant-button` yardım menüsüne bağlıdır. Kurulum rehberi hedef rotaya giderek başlar. Yeni OpenAPI tipleri kullanılır; geçici `contractRequest` kaldırılmıştır.
