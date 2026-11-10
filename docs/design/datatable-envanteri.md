# DataTable envanteri: ortak çekirdeği kullanmayan ekranlar (CF-178)

Bu belge yalnız tespittir; kod değiştirmez. CF-179 (issue #39) bu belgeyi iş listesi olarak kullanır ve taşıdığı her satırın **Durum** hücresini `taşındı (PR #…)` yapar.

- Tarama tarihi: 2026-11-06, `main` @ `5ce4191d`
- Kapsam: web paneli (`frontend/src/app/(cms)`, `(platform)`, `(setup)` altındaki 100 rota ve `frontend/src/features/**`). `mobile/`, pazarlama sayfaları, yardım merkezi ve misafir sayfaları kapsam dışı.
- Ortak çekirdek: `frontend/src/components/tables/*` (`DataTable`) ve `frontend/src/components/entity/*` (`EntityTable`, `useServerListState`, `EntityPage`, `EntityToolbar`, `EntityFilters`, `EntityRowActions`, `EntityDeleteDialog`).

## 1. Özet

| Karar | Adet | Açıklama |
|---|---|---|
| **Taşı** | 45 liste | 22'si ham `<table>` (9'u `BillingTable` üzerinden), 23'ü `.map` ile elle çizilen liste |
| **Bileşen eksik** | 3 liste | Çekirdeğe önce yetenek eklenmeli (bölüm 7) |
| **Düzelt** (yarım kullanım) | 3 tablo | Çekirdeği kullanıyor ama görünen arama kutusu sunucuya gitmiyor |
| **Kalsın** | 13 ham tablo dosyası + 82 elle liste | İçeriği gereği tablo/liste çekirdeğine uymuyor |

Başlangıç listesindeki sayılar taramayla aynı çıktı: çekirdeği import eden 86 dosya, ham `<table>` içeren 27 feature dosyası, bunların 25'i çekirdeksiz, 2'si karışık. Farklar:

- `components/ui/table` diye bir dosya **yok**; ham kullanımların tamamı düz `<table>` etiketidir.
- 86 dosya çekirdeği import ediyor ama yalnız **18 dosya** gerçekten `<DataTable>` / `<EntityTable>` çiziyor; kalanı `EntityPage`, `EntitySectionCard` gibi kabuk parçalarını kullanıyor.
- Başlangıç listesinde olmayan en büyük kalem: `features/billing/components/billing-table.tsx` yerel bir tablo soyutlamasıdır ve **9 ekranda** kullanılır. Bu ekranların dosyalarında `<table>` geçmediği için dosya taraması onları göstermez.
- Ham tablo dışında, `.map` ile elle çizilen **107 kayıt listesi** bulundu: 23 Taşı, 2 Bileşen eksik, 82 Kalsın (bölüm 4 ve 5).

Dikkat çeken bulgular:

1. **Çekirdekte sunucu modunda sayaç hatası.** `data-table-pagination.tsx:35` toplamı `table.getFilteredRowModel().rows.length` ile hesaplıyor. Sunucu sayfalamada bu, o sayfadaki satır sayısıdır; 2. sayfada "21–20 / 20" gibi bir metin üretir. Kod okumasıyla bulundu, tarayıcıda denenmedi. Bugün `EntityTable` kullanan 18 ekranı da etkiler; CF-179'un ilk işi olmalı.
2. **Sessiz veri kesilmesi.** Birçok liste sabit `limit` + `offset: 0` ile tek sayfa çekiyor ve sayfa çevirme sunmuyor: şirketler (100), video odaları (50), video kayıtları (50), kurallar (100), iş akışları (100), oncall uyarıları (50), izleyiciler (100), API anahtarları (100), webhook'lar (100), prosedürler (100), dosya gezgini (100). Sınırdan sonraki kayıtlar ekranda görünmez.
3. **Onaysız yıkıcı işlemler**: WhatsApp şablonu, video kaydı, tatil, sözleşmeli firma, paylaşım bağlantısı, hesap notu silme (bölüm 8.3).
4. **Ölü arama kutusu**: Gizlilik (Talepler, Rızalar) ve Destek erişimi tablolarında `EntityTable` varsayılanı arama kutusunu gösteriyor ama sorguya yalnız `limit`/`offset` gidiyor.

## 2. Tarama komutları

`frontend/src` dizininden çalıştırılır. macOS `grep`'inde `\b` çalışmadığı için kelime sınırı `([ >]|$)` ile verilmiştir.

```bash
cd frontend/src

# A. Çekirdeği import eden dosyalar → 86
grep -rlE 'from "@/components/(tables|entity)' features app --include='*.tsx' --include='*.ts' | wc -l

# B. Çekirdek tabloyu gerçekten çizen dosyalar → 18
grep -rlE '<(DataTable|EntityTable)([ >]|$)' features app --include='*.tsx' | sort

# C. Ham tablo içeren dosyalar → features altında 27
#    (components altında 2 meşru kullanım daha çıkar: tables/table-view.tsx, markdown/markdown-content.tsx)
grep -rlE '<(table|Table)([ >]|$)' features app components --include='*.tsx' | sort

# D. C ∩ çekirdek import eden → 2 (voice-calls-page, platform-health-page)
grep -rlE '<(table|Table)([ >]|$)' features app --include='*.tsx' \
  | xargs grep -lE 'from "@/components/(tables|entity)'

# E. Yerel tablo soyutlaması kullanımları → 9
grep -rnE '<BillingTable' features app --include='*.tsx'

# F. .map içeren ve çekirdek tablo çizmeyen dosyalar → 302 (aday havuzu; elle okunur)
comm -23 <(grep -rlE '\.map\(' features app --include='*.tsx' | sort) \
         <(grep -rlE '<(DataTable|EntityTable)([ >]|$)' features app --include='*.tsx' | sort) | wc -l

# G. Elle sayfalama kalıbı (offset ± sayfa boyutu)
grep -rnE 'setOffset\(|setPage\(' features --include='*.tsx' | grep -v 'components/tables'

# H. Sabit limit + offset 0 (sayfa çevirmeyen listeler)
grep -rnE 'limit: ?[0-9]+, ?offset: ?0|offset: ?0' features --include='*.ts' --include='*.tsx'

# I. Başlık ve onay yolları
grep -rlE '<EntityPage' features app --include='*.tsx' | wc -l       # 45
grep -rlE 'PageHeader' features app --include='*.tsx' | wc -l        # 45
grep -rlE 'SettingsPageHeader' features app --include='*.tsx' | wc -l # 8
grep -rnE 'ConfirmDialog|DeleteDialog|EntityDeleteDialog|confirmDelete|useDialogs' features --include='*.tsx'
```

CF-179 bitiş kontrolü: taşınan modülde C ve G komutları boş dönmelidir.

### Okuma derinliği

Kararlar grep sonucuna değil dosya okumasına dayanır; derinlik aynı değildir:

- Ham tablo içeren 27 dosya ve çekirdek tabloyu çizen 18 dosya: tablo, veri sorgusu, sayfalama ve durum dalları tek tek okundu.
- Panel modüllerindeki 405 feature dosyasının 44'ü yukarıdaki iki gruptadır (bir dosya iki grupta da sayılmaz; `auth`, `landing`, `docs`, `help-center`, `booking`, `status-subscription`, `step-up-engine` kapsam dışıdır). Kalan 361 dosya üç paralel okuyucuyla tarandı. Kayıt listesi içeren yaklaşık 120 dosyada ilgili bileşen satır satır okundu; yalnız sabit dizi / form alanı `.map`'i içeren ya da hiç `.map` içermeyen dosyalarda `.map` satırlarının bağlamına ve durum kalıplarına bakıldı, dosyanın tamamı okunmadı.
- **Taşı** ve **Bileşen eksik** kararı verilen her satırın dosya, satır ve veri çağrısı ikinci kez doğrulandı.
- Satır sayısı tahminleri koda ve alan bilgisine dayanır; üretim verisine bakılmadı. Backend incelenmedi: "sayfalama yok" ifadeleri frontend servis çağrısına göredir.

## 3. Ham tablo kullanan dosyalar (başlangıç listesi)

Öncelik: **P1** her gün kullanılan ya da veri kaybı görüntüsü veren ekran, **P2** sık kullanılan, **P3** seyrek / platform yöneticisi. Efor: küçük ≤ yarım gün, orta ≈ 1 gün, büyük > 2 gün.

### 3.1 Ana ekranlar

| # | Ekran ve rota | Dosya | Bugünkü yapı | Veri | Eksikler | Karar | Efor | Öncelik | Durum |
|---|---|---|---|---|---|---|---|---|---|
| T01 | Kişiler `/contacts` | `features/contacts/components/contacts-page.tsx:138` | Ham tablo; elle arama, elle önceki/sonraki | Sunucu: `limit 50`, `offset`, `q`, `company`. Binlerce satır | Sıralama, filtre (şirket yalnız URL'den), sayfa boyutu, sütun tercihi, toplu işlem, hata durumu yok; dar ekranda sütun gizleme elle | **Taşı**: `EntityTable` + `useServerListState`. `?c=` (çekmece) ve `?company=` parametreleri korunur | orta | P1 | taşındı (PR #51): ad ve son görülmeye göre sunucu sıralaması açıldı (uç destekliyor) |
| T02 | Otomasyon kuralları `/rules` | `features/automation-rules/components/rules-page.tsx:90` | Ham tablo; satır içi aç/kapa anahtarı | `useRules` sabit `limit 100, offset 0`; sayfa çevirme yok. Onlarca satır | Arama, durum filtresi, sayfalama, dar ekran görünümü yok. Sıra (`position`) salt okunur gösteriliyor | **Taşı**: `DataTable` istemci modu; varsayılan sıralama `position` | küçük | P2 | taşındı (PR #60) |
| T03 | Kural çalıştırma günlüğü `/rules` (alt bölüm) | `features/automation-rules/components/rules-page.tsx:240` | Ham tablo; elle önceki/sonraki | Sunucu: `limit 25`, `offset`. Binlerce satır | Durum / kural filtresi, sıralama, sayfa boyutu yok | **Taşı**: `EntityTable`; kural ve durum faceted filtre | küçük | P2 | taşındı (PR #60): uç kural / durum parametresi almadığı için süzgeç eklenmedi |
| T04 | İş akışları `/workflows` | `features/workflows/components/workflows-page.tsx:101` | Ham tablo | `workflowsService.list(100, 0)` sabit; satır başına ayrı "son çalıştırma" sorgusu (N+1) | Arama, durum filtresi, sıralama, sayfalama yok | **Taşı**: `DataTable` istemci modu. N+1 sorgusu backend notu (bölüm 9) | küçük | P2 | taşındı (PR #62): uç sayfaladığı için sunucu modu |
| T05 | Kılavuzlar `/platform/guides` | `features/guides/components/platform-guides-page.tsx:200` | Ham tablo; elle arama + 2 `Select` filtre + elle önceki/sonraki | Sunucu: `limit`, `offset`, `q`, `surface`, `locale`. Yüzlerce satır | Sıralama, sütun tercihi, sayfa boyutu yok; boş durum elle | **Taşı**: `EntityTable` + `EntityFilters` | orta | P3 | |
| T06 | Ekip durumu `/team-status` → Canlı | `features/routing/components/team-status-page.tsx:162` | Ham tablo; satır içi kapasite limiti düzenleme; 15 sn'de yenileme | `routingService.capacity`, sayfalama yok; temsilci sayısı kadar satır (5–200) | Arama, duruma / açık konuşmaya göre sıralama yok; yükleniyor elle | **Taşı**: `DataTable` istemci modu, limit sütunu `editVariant: "number"` | orta | P2 | taşındı (PR #79): sınır düzenlemesi mevcut kalem akışıyla korundu |
| T07 | Pazarlama kaynakları `/platform/settings/marketing/sources` | `features/settings/components/marketing-sources-page.tsx:94` | Ham tablo; istemcide gruplanmış özet (kaynak, kampanya, adet, pay) | Tek sorgu, tarih aralığı; onlarca satır | Sıralama yok (adet azalan sabit) | **Kalsın**: tarih aralığına bağlı rapor özeti, kayıt listesi değil | — | — | |

### 3.2 Sekme ve bileşenler

| # | Ekran ve rota | Dosya | Bugünkü yapı | Veri | Eksikler | Karar | Efor | Öncelik | Durum |
|---|---|---|---|---|---|---|---|---|---|
| T08 | AI çalıştırma günlüğü: `/settings/ai` → Çalıştırmalar ve `/platform/organizations/[uuid]` → AI | `features/ai-settings/components/run-log.tsx:113` (kullanım: `runs-tab.tsx:29`, `organization-detail-page.tsx:446`) | Ham tablo; elle önceki/sonraki; satıra tıkla → ayrıntı | Sunucu: `fetchPage(offset, limit)`. Binlerce satır | Durum filtresi, tarih aralığı, sıralama, sayfa boyutu yok | **Taşı**: `EntityTable`; tek taşıma iki ekranı düzeltir | orta | P2 | taşındı (PR #64) |
| T09 | AI araç çağrıları `/settings/ai` → Araçlar (alt bölüm) | `features/ai-settings/components/tools-tab.tsx:215` | Ham tablo; elle önceki/sonraki | Sunucu: `toolCalls(offset, limit)` | Araç / durum filtresi, sıralama yok | **Taşı**: `EntityTable`. Aynı dosyadaki araç kartları (`:136`) Kalsın | küçük | P3 | taşındı (PR #64) |
| T10 | Faturalama tabloları (9 ekran, aşağıda) | `features/billing/components/billing-table.tsx:86` | Yerel tablo soyutlaması: sütun tanımı, iskelet, boş / hata durumu, `limit`/`offset` önceki/sonraki | Ekrana göre değişir | Sıralama, sütun tercihi, mobil kart, sayfa boyutu, `persistKey` yok; filtreler elle `Select` | **Taşı**: `BillingColumn` → `ColumnDef`; sonunda `billing-table.tsx` silinir | büyük (ekran başına küçük) | P2 | |
| T11 | Kalibrasyonlar `/coaching` → Kalibrasyon | `features/coaching/components/calibrations-tab.tsx:87` | Ham tablo; elle önceki/sonraki | Sunucu: `limit`, `offset` | Sıralama, durum filtresi yok | **Taşı**: `EntityTable` | küçük | P3 | taşındı (PR #129) |
| T12 | Liderlik tablosu `/coaching` → Liderlik | `features/coaching/components/leaderboard-tab.tsx:146` | Ham tablo + grafik; elle önceki/sonraki | Sunucu: `limit`, `offset`, dönem | Arama yok. Sıra içerik gereği sabit | **Taşı**: `EntityTable`, sıralama kapalı | küçük | P3 | taşındı (PR #129) |
| T13 | Puanlama kuralları `/coaching` → Kurallar | `features/coaching/components/rules-tab.tsx:108` | Ham tablo; satır işlemi | `coachingService.rules`, sayfalama yok; onlarca satır | Arama, sıralama yok | **Taşı**: `DataTable` istemci modu | küçük | P3 | taşındı (PR #129) |
| T14 | Rapor sonucu `/reports` (özel rapor ve karneler) | `features/reports/components/report-result.tsx:122` | Ham tablo; dinamik metrik sütunları; elle istemci sıralaması | Rapor çalıştırma çıktısı | — | **Kalsın**: rapor çıktısı; sütunlar çalışma anında belirlenir | — | — | |
| T15 | Zamanlanmış raporlar `/reports` → Zamanlamalar | `features/reports/components/schedules-tab.tsx:139` | Ham tablo; satır işlemi | `reportsService.schedules`, sayfalama yok; <20 satır | Sıralama, dar ekran görünümü yok | **Taşı**: `DataTable` istemci modu | küçük | P3 | taşındı (PR #131) |
| T16 | Dosya gezgini `/platform/settings/storage` (liste görünümü) | `features/storage/components/storage-file-list.tsx:52` (ızgara ikizi `storage-file-grid.tsx:52`) | Ham tablo + ayrı ızgara; seçim, sağ tık menüsü, klasöre girme, sürükle-bırak yükleme | `useStorageFiles` sunucu arama / filtre / sıralama; **sabit `limit 100, offset 0`** | 100'den sonrası görünmez | **Kalsın**: dosya yöneticisi; sağ tık menüsü, çift tıkla klasöre girme ve yükleme alanı tek ekrana özel. Sayfalama eksiği ayrı iş | — | — | |
| T17 | Tahmin tablosu `/workforce` → Tahmin | `features/workforce/components/forecast-tab.tsx:485` | Ham tablo (gün başına satır) + grafik | Hesap çıktısı | — | **Kalsın**: hesaplanan rapor çıktısı | — | — | |
| T18 | Mesai çizelgeleri `/workforce` → Çizelge | `features/workforce/components/timesheets-tab.tsx:186` | Ham tablo; temsilci satırı tıklanınca konuşma alt satırları açılır | Tarih aralığı sorgusu; istemcide temsilciye göre toplanır | Sıralama, arama yok | **Bileşen eksik**: satır genişletme (bölüm 7.2) | orta | P3 | |

T10 kapsamındaki ekranlar (her biri ayrı satır olarak taşınır):

| # | Ekran ve rota | Dosya | Veri | Ek not | Durum |
|---|---|---|---|---|---|
| T10a | Abonelikler `/platform/billing` | `billing/components/platform/subscriptions-panel.tsx:214` | Sunucu, 20'şer, `q` + durum | Elle arama ve `Select` filtre | taşındı (PR #126) |
| T10b | Ödemeler `/platform/billing` | `billing/components/platform/payments-panel.tsx:300` | Sunucu, 20'şer, durum | Satır işlemi: iade | taşındı (PR #126) |
| T10c | Havaleler `/platform/billing` | `billing/components/platform/transfers-panel.tsx:238` | Sunucu, 20'şer, durum | Onayla / reddet elle `Dialog` | taşındı (PR #126) |
| T10d | Planlar `/platform/billing` | `billing/components/platform/plans-panel.tsx:303` | Sunucu, 50'şer | Satıra tıkla → plan düzenleyici | taşındı (PR #126) |
| T10e | Banka hesapları `/platform/billing` | `billing/components/platform/bank-accounts-panel.tsx:132` | Sayfasız | | taşındı (PR #126) |
| T10f | Kredi ürünleri `/platform/billing` | `billing/components/platform/catalog/credit-products-panel.tsx:285` | Sayfasız; birime göre 3 ayrı tablo | Üç ayrı `DataTable` olarak kalabilir | taşındı (PR #126) |
| T10g | Organizasyon cüzdanı `/platform/organizations/[uuid]` → Cüzdan | `billing/components/platform/organization-wallet-tab.tsx:355` | Sunucu sayfalı | | taşındı (PR #126) |
| T10h | Faturalar `/billing` | `billing/components/portal/billing-portal-page.tsx:558` | Sunucu, 10'ar | Satır işlemi: öde, PDF | taşındı (PR #125) |
| T10i | Kredi hareketleri `/billing` | `billing/components/portal/wallet/wallet-ledger-card.tsx:106` | Sunucu sayfalı | | taşındı (PR #125) |

### 3.3 Diyalog, yardım ve panel

| # | Ekran | Dosya | Bugünkü yapı | Karar ve gerekçe |
|---|---|---|---|---|
| T19 | Bildirim tercihleri (`/profile`, `/platform/profile`) | `features/account/components/notification-matrix-form.tsx:146` | Olay × kanal onay kutusu matrisi | **Kalsın**: form matrisi |
| T20 | Entegrasyon yardım kutusu (`/settings/integrations`) | `features/automation-integrations/components/connect-help.tsx:66` | Sabit uç nokta başvuru tablosu | **Kalsın**: statik içerik |
| T21 | E-posta alan adı sihirbazı (`/channels`) | `features/channels/components/email-domain-wizard.tsx:87` | 3–5 DNS kaydı, kopyala düğmesi | **Kalsın**: sihirbaz adımı |
| T22 | Kalibrasyon sonuçları diyaloğu (`/coaching`) | `features/coaching/components/calibration-dialog.tsx:368`, `:423` | Ölçüt × değerlendirici iki matris | **Kalsın**: karşılaştırma matrisi |
| T23 | Tanılama paneli (`/inbox` bağlam paneli) | `features/inbox/components/diagnostics-panel.tsx:282` | 11 px ağ isteği mini tablosu | **Kalsın**: dar panel içi teknik döküm |
| T24 | SLA politika diyaloğu (`/settings/sla`) | `features/sla/components/policy-dialog.tsx:391` | Öncelik × metrik giriş matrisi | **Kalsın**: form matrisi |
| T25 | Karşılaştırma sayfası `/compare` | `features/landing/components/compare-page.tsx` | Pazarlama karşılaştırma tablosu | **Kalsın**: kapsam dışı (pazarlama) |
| T26 | Fiyatlandırma | `features/landing/components/pricing.tsx:202` | Plan özellik matrisi | **Kalsın**: kapsam dışı (pazarlama) |

### 3.4 Çekirdek ile ham tabloyu birlikte kullananlar

| # | Ekran ve rota | Dosya | Bugünkü yapı | Veri | Eksikler | Karar | Efor | Öncelik | Durum |
|---|---|---|---|---|---|---|---|---|---|
| T27 | Sesli aramalar `/channels/[uuid]/calls` | `features/channels/components/voice-calls-page.tsx:75` | `EntityPage` + ham tablo; elle önceki/sonraki | Sunucu: sayfa numarası. Binlerce satır | Yön / durum filtresi, sıralama, sayfa boyutu yok | **Taşı**: `EntityTable` | küçük | P2 | taşındı (PR #66) |
| T28 | Platform sağlığı `/platform/health` | `features/platform-health/platform-health-page.tsx:243` | `EntityPage` + kuyruk tablosu (8 sayısal sütun) + kanal ve AI sağlayıcı kartları; otomatik yenileme | Tek sağlık özeti; ~10 sabit kuyruk | — | **Kalsın**: izleme panosu, sabit küçük küme | — | — | |

## 4. `.map` ile elle çizilen listeler: Taşı ve Bileşen eksik

| # | Ekran ve rota | Dosya | Bugünkü yapı | Veri | Eksikler | Karar | Efor | Öncelik | Durum |
|---|---|---|---|---|---|---|---|---|---|
| M01 | Şirketler `/companies` | `features/contacts/components/companies-page.tsx:97` | 3 sütun kart ızgarası; elle arama | `useCompanyList({ q, limit: 100, offset: 0 })`; uç `limit`/`offset`/`q` alıyor ama **sayfa çevrilmiyor** | Sayfalama (100 sonrası yalnız aramayla bulunur), sıralama (ad, MRR), filtre (kademe, sağlık), hata durumu, tablo görünümü | **Taşı**: `EntityTable`; kart görünümü `renderGridItem` + `viewMode` ile kalır | orta | P1 | taşındı (PR #51): varsayılan görünüm kart, sayfa boyutu 50; sıralama ve filtre uçta olmadığı için kapalı |
| M02 | Makaleler `/settings/knowledge` | `features/knowledge/components/articles-tab.tsx:224` | Bağlantı satırları; elle arama, 4 elle filtre, elle sayfalama | Sunucu: `limit 20`, `offset`, `q`, dil, görünürlük, durum, koleksiyon | Sıralama, toplu yayınla / arşivle, sütun tercihi, sayfa boyutu | **Taşı**: `EntityTable` + `EntityFilters`; birebir örtüşüyor | orta | P1 | taşındı (PR #57): süzgeçler çekmece yerine araç çubuğunda kaldı; uç `sort` almadığı için sıralama kapalı |
| M03 | Video odaları `/video` | `features/video/components/video-rooms-page.tsx:111` | `Card` satırları; durum sekmeleri; 10 sn'de yenileme | `videoService.rooms({ status, limit: 50, offset: 0 })`; **sayfa çevrilmiyor** | Sayfalama, arama, sıralama, yükleniyor ve hata durumu yok | **Taşı**: `EntityTable`; durum sekmesi faceted filtre olur | orta | P1 | taşındı (PR #52): durum sekmeleri korundu (uç tek durum alıyor; faceted filtre yeniden tasarım olurdu) |
| M04 | Video kayıtları `/settings/video` → Kayıtlar | `features/video/components/video-room-page.tsx:469` | `<li>` satırları; indir / sil | `videoService.recordings({ room, limit: 50, offset: 0 })`; **sayfa çevrilmiyor** | Sayfalama, arama, durum filtresi, yükleniyor / hata, **silme onayı** (`:506`) | **Taşı**: `EntityTable` + `EntityRowActions` + `EntityDeleteDialog` | küçük | P2 | taşındı (PR #52): silme onayı eklendi |
| M05 | Hazır yanıtlar `/replies` | `features/macros/components/replies-page.tsx:115` | Düğme satırları; tıkla → düzenleme diyaloğu | `useCannedResponses()` hepsini çeker; servis `q` destekliyor ama ekran göndermiyor. Onlarca–yüzlerce | Arama, kapsam / kategori filtresi, kullanım sayısına göre sıralama, hata durumu | **Taşı**: `DataTable` istemci modu | orta | P2 | taşındı (PR #59) |
| M06 | Makrolar `/replies` → Makrolar | `features/macros/components/replies-page.tsx:334` | 2 sütun kart | `useMacros()` hepsini çeker | Arama, yükleniyor, hata durumu | **Taşı**: M05 ile aynı kalıp, tek PR | küçük | P2 | taşındı (PR #59) |
| M07 | İş akışı çalıştırmaları `/workflows/[uuid]` → Çalıştırmalar | `features/workflows/components/runs-panel.tsx:57` | Tıklanabilir `<li>`; "daha fazla" düğmesi | `workflowsService.runs(uuid, limit, 0)`: düğme `limit`'i büyütüp **baştan tümünü yeniden çekiyor**; 30 sn'de yenileme | Durum filtresi, gerçek sayfalama | **Taşı**: `EntityTable`. Dar yan panelde durduğu için yerleşim kararı PR'da verilir | orta | P2 | taşındı (PR #62): tek sütunlu, araç çubuksuz tablo |
| M08 | WhatsApp şablonları `/channels` → kanal kartı → diyalog | `features/channels/components/whatsapp-templates-dialog.tsx:100` | `<ul>`; altta yeni şablon formu | `useWhatsAppTemplates`, sayfalama yok; 5–100+ | Arama, durum / kategori / dil filtresi, hata durumu, **silme onayı** (`:127`) | **Taşı**: `DataTable` istemci modu + `EntityDeleteDialog` | orta | P2 | taşındı (PR #66): silme onayı eklendi |
| M09 | API anahtarları `/settings/integrations` | `features/automation-integrations/components/api-keys-tab.tsx:111` | `<ul className="divide-y">` | Servis sabit `limit 100, offset 0`, `total` atılıyor | Durum filtresi, arama, sıralama | **Taşı**: `DataTable` istemci modu | küçük | P2 | taşındı (PR #78): uç sayfaladığı için sunucu modu |
| M10 | Webhook abonelikleri `/settings/integrations` | `features/automation-integrations/components/hooks-tab.tsx:79` | `<ul className="divide-y">` | Servis sabit `limit 100, offset 0` | Arama, olay filtresi | **Taşı**: M09 ile tek PR | küçük | P2 | taşındı (PR #78): uç sayfaladığı için sunucu modu |
| M11 | Takım üyeleri `/teams/[uuid]` → Üyeler | `features/teams/components/team-members-tab.tsx:93` | `<li>`; satır içi lider anahtarı | Takım detayındaki `members` dizisi; 3–50 | Arama, sıralama, yetenek filtresi | **Taşı**: `DataTable` istemci modu | küçük | P2 | taşındı (PR #79) |
| M12 | Oncall uyarıları `/oncall` → Uyarılar | `features/oncall/components/alerts-tab.tsx:104` | `<li>`; satır işlemi "Onayla"; 30 sn'de yenileme | Servis sabit `limit 50, offset 0` | Durum filtresi, sayfalama | **Taşı**: `EntityTable` | orta | P2 | taşındı (PR #127) |
| M13 | Forum konuları `/forum` → Konular | `features/forum/components/topics-tab.tsx:145` | Düğme satırları; elle arama, kategori `Select`, modüle özel `Pager` | Sunucu: `limit 20`, `offset`, `q`, `category` | Sıralama (oy, tarih), sayfa boyutu | **Taşı**: `EntityTable` + `EntityFilters`; `forum/components/pager.tsx` silinir | orta | P2 | taşındı (PR #128) |
| M14 | Yönlendirme geçmişi `/team-status` → Geçmiş | `features/routing/components/team-status-page.tsx:762` | `<ul>`; 30 sn'de yenileme | `routingService.events`, parametresiz | Sayfalama, neden filtresi, arama | **Taşı**: `DataTable` istemci modu (T06 ile tek PR) | küçük | P3 | taşındı (PR #79) |
| M15 | Uptime izleyicileri `/oncall` → İzleyiciler | `features/oncall/components/monitors-tab.tsx:115` | Izgara satır; sparkline; düzenle / sil | Servis sabit `limit 100`; satır başına ayrı sonuç sorgusu (N+1) | Durum filtresi, arama, sıralama | **Taşı**: `DataTable` istemci modu; sparkline özel hücre | orta | P3 | taşındı (PR #127) |
| M16 | AI prosedürleri `/settings/ai` → Prosedürler | `features/ai-settings/components/procedures-tab.tsx:178` | `Card` satırları | Servis sabit `limit 100` | Arama, dil ve yayın durumu filtresi | **Taşı**: `DataTable` istemci modu | küçük | P3 | taşındı (PR #64): uç sayfaladığı için sunucu modu |
| M17 | Özel alanlar `/settings/custom-fields` | `features/custom-fields/components/settings/field-list.tsx:174` | Liste satırı; yukarı / aşağı ok ile sıralama | Hepsi çekilir; onlarca | Sürükle-sırala, arama | **Taşı**: `DataTable` istemci modu + `rowReorder` | orta | P3 | |
| M18 | Talep formları `/settings/request-forms` | `features/request-forms/components/request-forms-page.tsx:108` | `Card` satırları; düzenle / sil | Hepsi çekilir; birkaç–onlarca | Arama, kanal filtresi; boş durum elle | **Taşı**: `DataTable` istemci modu | küçük | P3 | |
| M19 | Tatiller `/settings/sla` → Tatiller | `features/sla/components/sla-settings-page.tsx:536` | `<li>`; sil | Hepsi çekilir; yılda 5–30 | Yükleniyor / hata, tarih sıralaması, **silme onayı** (`:553`) | **Taşı**: `DataTable` istemci modu | küçük | P3 | taşındı (PR #132) |
| M20 | Sözleşmeli firmalar `/settings/sla` → Politikalar altı | `features/sla/components/contracted-companies.tsx:127` | `<li>`; kaldır | Hepsi çekilir; onlarca–yüzlerce | Arama, politika filtresi, sayfalama, yükleniyor / hata, **kaldırma onayı** (`:149`) | **Taşı**: `DataTable` istemci modu | küçük | P3 | taşındı (PR #132) |
| M21 | Destek erişimi `/platform/support-access` | `features/support-access/platform-support-access-page.tsx:89` | `<li>`; sayfa zaten `EntityPage` | Parametresiz; birkaç–onlarca | Arama, bitişe göre sıralama | **Taşı**: yalnız liste gövdesi değişir | küçük | P3 | |
| M22 | Ürün geri bildirimi `/platform/product-feedback` | `features/guides/components/platform-feedback-page.tsx:120` | Kart; satır içi durum `Select`; elle 2 filtre, elle önceki/sonraki | Sunucu: `limit 25`, `offset`, tür, durum | Arama, sayfa göstergesi | **Taşı**: `EntityTable` + `EntityFilters`; mesaj kısaltılmış sütun + ayrıntı çekmecesi. Mesajın tamamı listede kalacaksa 7.2'ye bağlıdır | orta | P3 | |
| M23 | Video paketleri `/platform/video` → Paketler | `features/video/components/platform-video-page.tsx:278` | Hizasız `flex-wrap` satırlar | Hepsi çekilir; <10 | Sütun hizası, yükleniyor / hata | **Taşı**: `DataTable` istemci modu | küçük | P3 | taşındı (PR #52) |
| B01 | Moderasyon kuyruğu `/forum` → Moderasyon | `features/forum/components/moderation-tab.tsx:126` | Zengin kart; 4 işlem düğmesi (onaysız); modüle özel `Pager` | Sunucu: `limit 20`, `offset` | Toplu onay / ret, tür ve bayrak filtresi | **Bileşen eksik**: satır genişletme (7.2). O zamana kadar yalnız `Pager` ortaklaştırılır | orta | P3 | |
| B02 | Aboneleri taşı diyaloğu `/platform/billing/plans/[uuid]` | `features/billing/components/platform/catalog/migrate-subscribers-dialog.tsx:200` | Onay kutulu `<li>`; elle arama, elle seçim, "tümünü seç" | Sayfalama yok; sunucu `truncated` bayrağı dönüyor | Kesilen abonelere ulaşılamıyor; sütun sıralaması | **Bileşen eksik**: satır bazlı seçim koşulu ve seçim tavanı (7.3) | orta | P3 | |

T18 (mesai çizelgeleri) üçüncü **Bileşen eksik** kaydıdır.

## 5. `.map` ile elle çizilen listeler: Kalsın

Hepsi okundu; karar gerekçesi tek satırdır. Bu listelerdeki boş / yükleniyor / hata / onay eksikleri bölüm 8'dedir.

| Ekran | Dosya | Gerekçe |
|---|---|---|
| `/inbox` konuşma listesi | `inbox/components/conversation-list.tsx:124` | Sanallaştırılmış, sonsuz kaydırmalı, klavye gezinmeli ana-detay listesi |
| `/inbox` bağlam paneli: bağlantılar, yan konuşmalar, oturum kayıtları, geçmiş, tanılama öğeleri | `inbox/components/conversation-links.tsx:100`, `side-conversations.tsx:77`, `session-replay-panel.tsx:107`, `context-panel.tsx:190`, `diagnostics-panel.tsx:87` | Dar yan panelde birkaç satırlık özet |
| `/inbox` hatırlatıcılar, kayıtlı görünümler | `inbox/components/reminder-menu.tsx:97`, `filter-bar.tsx:297` | Açılır menü öğeleri |
| `/companies/[uuid]` zaman çizelgesi | `contacts/components/account-page.tsx:268` | Heterojen olay akışı |
| `/companies/[uuid]` konuşmalar ve kişiler kartı | `contacts/components/account-page.tsx:444`, `:497` | En çok 12 / 20 satırlık özet, tam listeye bağlantı var |
| Gelir geçmişi kartı | `contacts/components/company-revenue-card.tsx:154` | Tek kaydın alt geçmişi |
| Son içe aktarmalar (diyalog) | `contacts/components/contacts-io-toolbar.tsx:208` | Diyalog içi yardımcı liste |
| `/settings/custom-objects` | `custom-fields/components/settings/custom-objects-settings-page.tsx:152` | Plan sınırlı az sayıda ayar kartı |
| `/forum` konu yanıtları (diyalog), kategoriler | `forum/components/topics-tab.tsx:280`, `categories-tab.tsx:76` | Uzun gövdeli yanıt akışı; işlem içermeyen birkaç kart |
| `/settings/knowledge` koleksiyonlar | `knowledge/components/collections-tab.tsx:161` | İki seviyeli ağaç |
| Sürüm geçmişi (makale, widget, iş akışı) | `knowledge/components/version-history.tsx:76`, `widget/components/version-history.tsx:93`, `workflows/components/versions-panel.tsx:128` | Editör yan paneli kartları |
| Bildirim zili | `notifications/components/notification-inbox.tsx:162` | Açılır pencere, 15 kayıt |
| `/oncall` olaylar, politikalar, nöbet çizelgeleri | `oncall/components/alerts-tab.tsx:176`, `policies-tab.tsx:111`, `rotas-tab.tsx:110` | Yan liste; alt adımlı ve takvim şeritli zengin kartlar |
| Organizasyon değiştirici | `organizations/components/organization-switcher.tsx:162` | Menü |
| `/platform/organizations/[uuid]` denetim | `platform-organizations/components/organization-audit-tab.tsx:61` | Ortak `Timeline` ile yazılmış zaman çizelgesi |
| `/getting-started` plan gözden geçirme, şablon galerisi | `onboarding/components/smart-setup/plan-review.tsx:182`, `template-gallery.tsx:70` | Sihirbaz adımı |
| `/settings/brands` | `brands/components/brands-page.tsx:118` | Plan sınırlı 1–10 zengin kart |
| `/settings/appointments` randevu türleri | `calendar/components/appointment-types-page.tsx:104` | 1–15 zengin kart |
| `/channels` kanal kartları | `channels/components/channels-page.tsx:98` | Zengin durum kartı |
| Kanal ve sayfa seçiciler | `channels/components/slack-channel-picker.tsx:87`, `google-chat-space-picker.tsx:95`, `oauth-callback.tsx:141`, `add-channel-dialog.tsx:350` | Seçim listesi. Slack seçicide arama eksik (not) |
| `/profile` oturumlar, passkey'ler, bağlı hesaplar, yetkilerim | `account/components/session-manager.tsx:158`, `passkey-manager.tsx:390`, `github-identity-manager.tsx:129`, `profile-permissions-list.tsx:55` | Birkaç satırlık kişisel güvenlik listeleri |
| Asistan sohbet geçmişi | `assistant/components/assistant-drawer.tsx:446` | Dar çekmece gezinme listesi |
| `/coaching` → Koçluk | `coaching/components/coaching-tab.tsx:155` | Hedef → görev ağacı ve not günlüğü |
| `/settings/ai` araç kartları | `ai-settings/components/tools-tab.tsx:136` | Az sayıda yapılandırma kartı |
| `/billing` kartlar, bekleyen havaleler, kredi paketleri, kullanım ölçerleri | `billing/components/portal/billing-portal-page.tsx:601`, `:641`, `portal/wallet/credit-packs-card.tsx:281`, `usage-meters.tsx:45` | Ayar listesi, bilgi notu, fiyat vitrini, gösterge |
| `/platform/organizations/[uuid]` kullanım | `billing/components/platform/organization-billing-tab.tsx:1060` | Ölçer çubuklu ayar satırları |
| Plan sürüm şeridi; plan ve fiyat seçiciler | `billing/components/platform/plan-editor-page.tsx:137`, `portal/plan-change-dialog.tsx:155`, `platform/org/change-plan-dialog.tsx:144`, `portal/pay-dialog.tsx:258` | Seçici |
| Randevu kartı, yaklaşan randevular, ajanda, saat seçici | `calendar/components/linked-appointments-card.tsx:88`, `upcoming-widget.tsx:80`, `views/agenda-view.tsx:56`, `slot-picker.tsx:70` | Yan panel özeti, pano bileşeni, takvim görünümü |
| Örnek konuşma seçiciler | `automation-rules/components/rule-test-dialog.tsx:69`, `workflows/components/test-dialog.tsx:87` | Seçici |
| `/reports` özel raporlar | `reports/components/custom-reports-tab.tsx:121` | Ana-detay seçici |
| `/settings/apps` yüklü uygulamalar; `/apps` şeridi; uygulama paneli | `sidebar-apps/components/sidebar-apps-settings-page.tsx:227`, `nav-apps-page.tsx:59`, `sidebar-apps-panel.tsx:46` | <10 zengin kart; gezinme; iframe barındırıcı |
| `/settings/sla` politikalar, mesai saatleri, resmî tatiller | `sla/components/sla-settings-page.tsx:168`, `:346`, `:570` | İç içe hedef ızgaralı kart; 1–5 kayıtlık haftalık özet kartı; salt okunur bilgi |
| `/settings/security` IP izin listesi | `security-settings/security-page.tsx:601` | Form içi değer listesi |
| `/settings/privacy` veri envanteri | `compliance/privacy-page.tsx:961` | Kategori kartları, CSV dışa aktarımı var |
| Dosya ayrıntıları: sürümler, paylaşılanlar, bağlantılar; paylaş diyaloğu; yükleme paneli | `storage/components/storage-details.tsx:246`, `:290`, `:316`, `storage-share-dialog.tsx:310`, `storage-upload-manager.tsx:56` | Yan panel ve diyalog içi kısa listeler |
| `/platform/users` rol seçimi, kullanıcı rolleri | `users/components/role-multi-select.tsx:52`, `user-detail-page.tsx:265` | Form alanı; tek kaydın alt alanı |
| `/video/[uuid]` katılımcılar; dakika paketleri; plan geçersiz kılmaları | `video/components/video-room-page.tsx:322`, `video-settings-page.tsx:248`, `platform-video-page.tsx:155` | Canlı lobi listesi; fiyat kartları; seçici + form |
| `/widget` widget seçici, görüldüğü siteler | `widget/components/widget-admin-page.tsx:160`, `sections-security.tsx:430` | Sekme seçici; ayar içi durum listesi |
| İş akışı çalıştırma adımları | `workflows/components/runs-panel.tsx:191` | Tek kaydın adım izi |
| `/team-status` yönlendirme kuralları | `routing/components/team-status-page.tsx:531` | Az sayıda sıralı kural kartı |
| Platform sağlığı kanal ve AI kartları | `platform-health/platform-health-page.tsx:328`, `:422` | İzleme panosu (T28) |

## 6. Çekirdeği kullanan ekranlar: yarım kullanım

`<DataTable>` / `<EntityTable>` çizen 18 dosyanın hepsi `useServerListState` + `EntityTable` kalıbını izliyor; hiçbiri kendi sayfalamasını ya da araç çubuğunu yazmıyor. Sorunlu olanlar:

| # | Ekran ve rota | Dosya | Bulgu | Karar | Efor | Öncelik | Durum |
|---|---|---|---|---|---|---|---|
| Y01 | Gizlilik → Talepler `/settings/privacy` | `features/compliance/privacy-page.tsx:463` | `EntityTable` varsayılanı arama kutusunu gösteriyor; sorguya yalnız `limit`/`offset` gidiyor (`:325`). Tüm sütunlarda sıralama kapalı | **Düzelt**: `features={{ globalFilter: false, columnFilters: false }}` ya da uca `q` | küçük | P2 | düzeltildi (PR #48): arama ve sütun filtresi kapatıldı |
| Y02 | Gizlilik → Rızalar `/settings/privacy` | `features/compliance/privacy-page.tsx:831` | Aynı (`:756`) | **Düzelt** | küçük | P2 | düzeltildi (PR #48) |
| Y03 | Destek erişimi geçmişi `/settings/support-access` | `features/support-access/support-access-page.tsx:315` | Aynı (`:215`) | **Düzelt** | küçük | P2 | düzeltildi (PR #48) |
| Y04 | 18 ekranın tamamı | — | `pageCount` her sayfada elle hesaplanıyor (`Math.ceil(total / pageSize)`); sayaç hatası (7.1) hepsini etkiliyor | Çekirdek düzeltmesiyle birlikte kalkar | — | — | düzeltildi (PR #48): ekranlar `total` veriyor. `logs/purge-rules-panel` sayfasız veri çektiği için istemci moduna alındı |

Uygun kullananlar (değişiklik gerekmez): `audit/audit-page.tsx`, `custom-fields/components/records/object-records-page.tsx`, `invitations/components/invitations-table.tsx`, `io/components/activity-page.tsx`, `exports-page.tsx`, `imports-page.tsx`, `logs/components/logs-page.tsx`, `purge-rules-panel.tsx`, `notifications/components/notifications-page.tsx`, `platform-organizations/components/organization-members-tab.tsx`, `organizations-page.tsx`, `roles/components/roles-page.tsx`, `teams/components/teams-page.tsx`, `tenant-members/components/members-page.tsx`, `tenant-roles/components/tenant-roles-page.tsx`, `users/components/users-page.tsx`. `exports-page` ve `imports-page` desteklenmeyen arama ve filtreyi açıkça kapatıyor; Y01–Y03 için örnek budur.

## 7. Çekirdekte eksik yetenekler

CF-179 kuralı gereği tek ekrana özel çözüm çekirdeğe konmaz; aşağıdakilerin her biri birden çok ekranı ilgilendirir.

| # | Eksik | Kanıt | Kimi etkiler | Öneri |
|---|---|---|---|---|
| 7.1 | **Sunucu modunda toplam kayıt sayısı** | `components/tables/data-table-pagination.tsx:35` toplamı sayfadaki satır sayısından hesaplıyor; `DataTableProps` içinde `total` / `rowCount` yok | Bugünkü 18 ekran ve taşınacak tüm sunucu sayfalı listeler | `DataTableProps`'a `rowCount` ekle, `pageCount`'u ondan türet; `EntityTable` `total` alsın. **CF-179'un ilk PR'ı**. **Durum: eklendi (PR #48).** Hata birim testiyle doğrulandı (`components/tables/pagination-summary.test.mjs`): 45 kayıtlı listenin 2. sayfasında sayaç "21–20 / 20" çıkıyordu |
| 7.2 | **Satır genişletme** (alt satır / ayrıntı satırı) | `components/tables` içinde `getExpandedRowModel` yok | T18 mesai çizelgeleri, B01 moderasyon kuyruğu, M22 geri bildirim mesajı | `renderSubRow` + genişletme sütunu |
| 7.3 | **Satır bazlı seçim koşulu ve seçim tavanı** | `use-data-table.ts:249` `enableRowSelection: features.rowSelection` yalnız boolean | B02 aboneleri taşı; ileride engelli satır içeren toplu işlemler | `features.rowSelection` için `(row) => boolean`; isteğe bağlı `maxSelection` |
| 7.4 | **`EntityTable` varsayılanları sunucunun desteklemediği denetimleri açıyor** | `entity-table.tsx` `globalFilter`, `columnFilters`, `sorting` varsayılan `true` | Y01–Y03; taşınacak her yalın `limit`/`offset` ucu | Varsayılanı değiştirmek yerine taşıma PR'larında açıkça kapat; `EntityTable` belgesine not. **Durum: not eklendi (PR #48)** |
| 7.5 | **Durumun URL ile eşitlenmesi** | `components/tables` ve `use-server-list-state.ts` içinde `useSearchParams` yok | T01 (`?company=`), paylaşılabilir filtreli liste bağlantıları | Taşımayı engellemez: mevcut URL parametreleri sayfa bileşeninde tutulur. İsteğe bağlı `useServerListState({ syncUrl })` ayrı iş |
| 7.6 | **Araç çubuğu ve sayfalama satırı olmayan gömülü kip** | Doğrulanmadı: bütün `features` kapatıldığında araç çubuğunun boş satır bırakıp bırakmadığına bakılmalı | Kart ve diyalog içine gömülecek küçük tablolar (M08, M19, M20, M23, T10e) | İlk gömülü taşımada denenir; gerekirse `variant="embedded"`. **Durum: eklendi (PR #52)**: araç çubuğu boş satır bırakıyordu (sıfırlama menüsü hep çiziliyor); `features.toolbar: false` ile kapatılır |

Çekirdekte **var olan** ve taşımalarda kullanılacak yetenekler: istemci ve sunucu modu, faceted filtre, `renderGridItem` + `viewMode` (kart görünümünü koruyan M01, M06), `mobileAutoCards`, `rowReorder` (M17), hücre içi düzenleme (T06), `onRowClick`, `persistKey`, `bulkActions`, `onExportReady`, sütun `cellClassName` (para sütunu hizası).

## 8. Diğer tekrarlar (CF-179 kapsamına kendiliğinden girmez)

### 8.1 Sayfa başlığı: üç ayrı yol

- `EntityPage` (`components/entity`): 45 dosya. Başlık, breadcrumb ve yetki kapısı birlikte.
- `PageHeader` (`components/layout/page-header`): 45 dosya. Yetki reddi her sayfada elle `<p>` ile yazılıyor (ör. `contacts-page.tsx:98`, `companies-page.tsx:50`, `video-rooms-page.tsx:77`, `sla-settings-page.tsx:57`, `reports-page.tsx:37`, `oncall-page.tsx:57`, `knowledge-page.tsx:67`, `replies-page.tsx:47`, `forum-admin-page.tsx:34`, `widget-admin-page.tsx:136`, `workforce-page.tsx:43`, `request-forms-page.tsx:88`, `brands-page.tsx:99`).
- `SettingsPageHeader` (`features/settings/components/settings-page-header.tsx`): 8 dosya, platform ayar sayfaları.
- Tamamen elle `<header>` / `<h1>`: `contacts/components/account-page.tsx:92`.

### 8.2 Onay diyaloğu: beş ayrı yol

`EntityDeleteDialog` (5 kullanım: `organizations-page.tsx:301`, `organization-detail-page.tsx:459`, `team-pages.tsx:159`, `teams-page.tsx:140`, `tenant-roles-page.tsx:142`), `ConfirmDialog` (`components/dialogs`, en yaygın), `DeleteDialog` (`components/dialogs`; oncall, api-keys, hooks, özel raporlar), `useDialogs().confirm` / `confirmDelete` (`providers/dialog-provider`; guides, custom-fields, request-forms, sla, widget), ham `Dialog` ile elle (`account/components/session-manager.tsx:226`, `:263`, `passkey-manager.tsx:210`, `totp-manager.tsx:228`, `storage/components/storage-confirm-dialog.tsx:14`). Tarayıcı `confirm()` kullanımı yok.

### 8.3 Onaysız yıkıcı işlemler

| İşlem | Dosya |
|---|---|
| WhatsApp şablonu sil: **onay eklendi (PR #66)** | `channels/components/whatsapp-templates-dialog.tsx` |
| Video kaydı sil: **onay eklendi (PR #52)**; katılımcı çıkar (hâlâ onaysız) | `video/components/video-room-page.tsx` |
| Plan geçersiz kılmasını sil | `video/components/platform-video-page.tsx:175` |
| Tatil sil: **onay eklendi (PR #132)** | `sla/components/sla-settings-page.tsx:553` |
| Sözleşmeli firmayı kaldır: **onay eklendi (PR #132)** | `sla/components/contracted-companies.tsx:149` |
| Paylaşım bağlantısını iptal et | `storage/components/storage-share-dialog.tsx:373`, `storage-details.tsx:360` |
| Hesap notu sil | `contacts/components/account-page.tsx:413` |
| Konuşma bağlantısını kaldır; kayıtlı görünüm sil; hatırlatıcı iptal | `inbox/components/conversation-links.tsx:124`, `filter-bar.tsx:324`, `reminder-menu.tsx:109` |
| Moderasyonda reddet / spam | `forum/components/moderation-tab.tsx:172` |
| OAuth bağlantısını kes | `account/components/github-identity-manager.tsx:192` |

### 8.4 Elle sayfalama: 12 ayrı uygulama

`contacts-page.tsx:235`, `rules-page.tsx:316`, `platform-guides-page.tsx:300`, `ai-settings/components/run-log.tsx:187`, `tools-tab.tsx:264`, `calibrations-tab.tsx:130`, `leaderboard-tab.tsx:216`, `billing-table.tsx:144`, `voice-calls-page.tsx:156`, `forum/components/pager.tsx`, `knowledge/components/articles-tab.tsx:237`, `platform-feedback-page.tsx:178`. "Daha fazla yükle" kalıbı: `account-page.tsx:277`, `organization-audit-tab.tsx:76`, `runs-panel.tsx` (son ikisi `limit`'i büyütüp baştan çekiyor). Bölüm 3 ve 4'teki taşımalarla hepsi kalkar.

### 8.5 Elle arama kutusu (ikonlu `Input` + `setTimeout` / `useDebounce`)

`contacts-page.tsx:82`, `companies-page.tsx:44`, `platform-guides-page.tsx:153`, `topics-tab.tsx:66`, `articles-tab.tsx:117`, `subscriptions-panel.tsx`, `conversation-links.tsx:270`, `migrate-subscribers-dialog.tsx`.

### 8.6 Elle boş durum, yükleniyor ve hata

- `EmptyState` yerine `<p>` / `Card` / özel `div`: yaklaşık 55 dosya. En yoğun modüller: video (6 yer), storage (5), sla (4), contacts (6), inbox (6), calendar (5), billing (4), account (3; passkey ve oturum yöneticilerinde aynı özel `div` kopyalanmış).
- Yükleniyor durumu hiç olmayan listeler: `video-rooms-page.tsx:111`, `video-room-page.tsx:322`, `:469`, `video-settings-page.tsx:248`, `platform-video-page.tsx:278`, `sla-settings-page.tsx:536`, `:570`, `contracted-companies.tsx:127`, `replies-page.tsx:334`, `storage-details.tsx:246`.
- Hata durumu hiç ele alınmayan sorgular: `companies-page.tsx`, `contacts-page.tsx` (liste), `replies-page.tsx` (iki liste), `video-rooms-page.tsx`, `video-room-page.tsx`, `video-settings-page.tsx`, `platform-video-page.tsx`, `sla-settings-page.tsx` (mesai, tatil), `contracted-companies.tsx`, `whatsapp-templates-dialog.tsx`, `appointment-types-page.tsx` (türler), `session-manager.tsx`, `passkey-manager.tsx`, `conversation-links.tsx`, `side-conversations.tsx`.
- Yerel sarmalayıcı: `ai-settings/components/parts.tsx:132` `QueryState` (ortak `Loading` + `ErrorState`'i birleştirir); benzeri ortak katmanda yok.

### 8.7 Aynı bileşenin iki kez yazıldığı yerler

- Sürüm geçmişi: `knowledge/components/version-history.tsx` ve `widget/components/version-history.tsx`.
- Paylaşım bağlantısı listesi: `storage-share-dialog.tsx:310` ve `storage-details.tsx:316` (ikincisinde durum metni çevrilmeden basılıyor, `:332`).
- İçe aktarma satır hataları: `io/components/import-detail-page.tsx:264`, `import-wizard.tsx:540`, `contacts-io-toolbar.tsx:271`.
- Düzenle + sil ikon çifti (`EntityRowActions` yerine): `custom-reports-tab.tsx:146`, `request-forms-page.tsx:136`, `sla-settings-page.tsx:201`, `:376`, `sidebar-apps-settings-page.tsx:398`.
- `softphone/member-sip.tsx:41–100`: görünen metinler çeviri anahtarı değil, sabit Türkçe.

## 9. Backend notları (değişiklik bu işin kapsamında değil)

Frontend servis çağrılarından çıkarılmıştır; uçların kendisi incelenmedi.

| Not | Kaynak |
|---|---|
| Sunucu sayfalı listelerin çoğunda `sort` parametresi gönderilmiyor; taşımada sıralama açılacaksa uç `pkg/apiquery` `sort` kuralını desteklemeli | T01, T03, T08, T09, T11, T27, M02, M13, M22 |
| Yalın `limit`/`offset` uçları `q` almıyor | Y01–Y03 (veri sahibi talepleri, rızalar, destek erişimi) |
| Satır başına ek sorgu (N+1): iş akışı son çalıştırması, izleyici sonuçları | `workflows-page.tsx:140`, `monitors-tab.tsx` |
| `limit`/`offset` parametresi hiç gönderilmeyen listeler; uç sayfalıyorsa varsayılanla kesiliyor olabilir | hazır yanıtlar, makrolar, yönlendirme olayları, SLA tatilleri, sözleşmeli firmalar, WhatsApp şablonları, talep formları |
| Hazır yanıt ucu `q` destekliyor ama ekran göndermiyor | `replies-page.tsx` |
| Abonelik taşıma listesi `truncated` dönüyor, devamını isteme yolu yok | `migrate-subscribers-dialog.tsx:254` |
| Dosya listesi 100 kayıtla sınırlı, ekranda sayfa çevirme yok | `storage-explorer.tsx:97` |

## 10. CF-179 için önerilen PR sırası

Her PR tek modülü taşır. Sıra: önce çekirdek, sonra veri kaybı görüntüsü veren ve en sık kullanılan ekranlar, en sonda platform yöneticisi ekranları ve çekirdek yeteneği bekleyenler.

| Sıra | PR | Satırlar | Efor |
|---|---|---|---|
| 0 | Çekirdek: sunucu modunda toplam (`rowCount`), `EntityTable` `total` | 7.1, Y04 | küçük |
| 1 | Gizlilik ve destek erişimi: ölü arama kutusu | Y01, Y02, Y03 | küçük |
| 2 | Kişiler ve şirketler | T01, M01 | orta + orta |
| 3 | Video: odalar, kayıtlar, paketler | M03, M04, M23 | orta |
| 4 | Bilgi bankası makaleleri | M02 | orta |
| 5 | Hazır yanıtlar ve makrolar | M05, M06 | orta |
| 6 | Otomasyon kuralları ve çalıştırma günlüğü | T02, T03 | küçük |
| 7 | İş akışları ve çalıştırmalar | T04, M07 | orta |
| 8 | AI ayarları: çalıştırma günlüğü, araç çağrıları, prosedürler | T08, T09, M16 | orta |
| 9 | Kanallar: sesli aramalar, WhatsApp şablonları | T27, M08 | orta |
| 10 | Entegrasyonlar: API anahtarları, webhook'lar | M09, M10 | küçük |
| 11 | Ekip: takım üyeleri, ekip durumu canlı tablo ve geçmiş | M11, T06, M14 | orta |
| 12 | Faturalama (portal): faturalar, kredi hareketleri | T10h, T10i | küçük |
| 13 | Faturalama (platform): 7 panel, `billing-table.tsx` silinir | T10a–T10g | büyük |
| 14 | Oncall: uyarılar, izleyiciler | M12, M15 | orta |
| 15 | Forum konuları (`pager.tsx` yalnız moderasyonda kalır) | M13 | orta |
| 16 | Koçluk: kalibrasyonlar, liderlik, kurallar | T11, T12, T13 | küçük |
| 17 | Raporlar: zamanlamalar | T15 | küçük |
| 18 | SLA: tatiller, sözleşmeli firmalar | M19, M20 | küçük |
| 19 | Ayarlar: özel alanlar, talep formları | M17, M18 | orta |
| 20 | Platform: kılavuzlar, ürün geri bildirimi, destek erişimi | T05, M22, M21 | orta |
| 21 | Çekirdek: satır genişletme → mesai çizelgeleri, moderasyon | 7.2, T18, B01 | orta |
| 22 | Çekirdek: satır bazlı seçim → aboneleri taşı | 7.3, B02 | orta |

## 11. Rota kapsamı

Panelin 100 rotasının tamamı gözden geçirildi. Yukarıdaki tablolarda geçmeyen rotalar kayıt listesi içermez (form, ayar, ayrıntı, editör ya da pano):

- Liste içeren ve çekirdeği doğru kullanan rotalar: `/settings/audit`, `/settings/members`, `/settings/roles`, `/teams`, `/objects/[typeUuid]`, `/platform/users`, `/platform/organizations`, `/platform/settings/roles`, `/platform/settings/activity`, `/platform/settings/exports`, `/platform/settings/imports`, `/platform/settings/logs`, `/platform/notifications`.
- Liste içermeyen rotalar: `/dashboard` (pano; yalnız yaklaşan randevular), `/calendar` (takvim), `/setup`, `/getting-started`, `/profile` (T19 matrisi ve bölüm 5'teki güvenlik listeleri), `/profile/password`, `/profile/preferences`, `/channels/connect/[type]`, `/rules/[uuid]`, `/settings/knowledge/[uuid]`, `/settings/knowledge/new`, `/settings/organization`, `/settings/customer-health`, `/settings/roles/[uuid]`, `/settings/roles/create`, `/teams/create`, `/widget` (editör), `/platform` (genel bakış), `/platform/assistant`, `/platform/profile/*`, `/platform/organizations/create`, `/platform/organizations/[uuid]/edit`, `/platform/users/create`, `/platform/users/[uuid]`, `/platform/users/[uuid]/edit`, `/platform/notifications/[uuid]`, `/platform/settings` ve altındaki `access`, `auth`, `environment`, `general`, `integrations`, `maintenance`, `marketing`, `system`, `/platform/settings/exports/[uuid]`, `/platform/settings/imports/[uuid]`, `/platform/settings/roles/create`, `/platform/settings/roles/[uuid]`, `/platform/settings/roles/[uuid]/edit`.
- `app/(cms)`, `app/(platform)` ve `app/(setup)` altındaki `page.tsx` / `layout.tsx` dosyalarının hiçbirinde satır içi liste yok; hepsi bir feature bileşenini sarar.
