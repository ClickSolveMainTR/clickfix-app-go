# Bilgi katmanı (CF-213 ile CF-220)

ClickFix'in AI bilgi katmanının tasarım kaynağı. İki tüketicisi var: **müşteri AI'ı** (widget, kanallar, otomatik cevap) ve **temsilci asistanı** (gelen kutusu, taslak, özet). İkisi aynı bilgi birimlerini, aynı kapsam kurallarını, aynı önbelleği ve aynı vaka hafızasını kullanır; fark sadece görünürlüktedir (temsilci iç notları görür, müşteri görmez).

Bu belgenin iskeleti CF-213'te yazıldı. Her iş kendi bölümünü doldurur; bölüm başlıkları ve tablo adları burada sabitlenir ki paralel dallar çakışmasın.

| İş | Bölüm | Durum |
|----|-------|-------|
| CF-213 | [1. AI kütüphanesi](#1-ai-kütüphanesi-cf-213), iskelet | bu belge |
| CF-214 | [4. Vektör deposu](#4-vektör-deposu-pgvector-cf-214), marka süzgeci, marka personası | bekliyor |
| CF-215 | [3. Kapsam modeli](#3-kapsam-modeli) departman kısmı | bekliyor |
| CF-216 | [5. Bilgi birimleri](#5-bilgi-birimleri-cf-216), [6. Cevap önbelleği](#6-cevap-önbelleği-cf-216), [7. Vaka hafızası](#7-vaka-hafızası-cf-216) | bekliyor |
| CF-217 | [8. Geçmiş kayıt hattı](#8-geçmiş-kayıt-hattı-cf-217) | bekliyor |
| CF-218 | [9. Kategorizasyon](#9-kategorizasyon-cf-218) | bekliyor |
| CF-219 | [9.3 Sınıflandırma modeli](#93-sınıflandırma-modeli-cf-219-planlandı) | planlandı |
| CF-220 | [10. Ölçüm](#10-ölçüm-cf-220) | bekliyor |

## 0. Verilmiş kararlar

- Hem müşteri AI'ı hem temsilci asistanı bu katmanı kullanır.
- Kapsam sırası **organizasyon → marka → departman → ekip**.
- Vektörler **pgvector**'da; tek embedding modeli, sabit boyut. Model değişirse arka planda yeniden vektörleme.
- Sağlayıcılarla kendi HTTP katmanımız yerine **kütüphane** konuşur (CF-213, bölüm 1). Sağlayıcı çağrıları sadece `backend/internal/platform/ai` arayüzünden geçer; modüller SDK'yı doğrudan içe aktarmaz.
- Geçmiş kayıtlar (eski konuşmalar, dışarıdan yüklenen dökümler) **otomatik bir hattan** geçerek bilgi birimine dönüşür.
- Temsilci onayları (bir cevabı "doğru" işaretleme, düzeltme) pgvector'da saklanır ve aramada kanıt sayılır.
- **Tam kategorizasyon**: her kayıt bir departman kategorisi ve etiketler taşır.
- **Cevap önbelleği** ve **vaka hafızası** her iki AI'da da çalışır.

## 1. AI kütüphanesi (CF-213)

### 1.1 Deneme

Aynı küçük senaryo üç seçenekle yazıldı ve çalıştırıldı:

1. OpenAI uyumlu uç noktadan toplu embedding,
2. pgvector'a yazma ve organizasyon süzgeçli kosinüs araması (yerel PostgreSQL 16 + pgvector 0.6),
3. Claude ile araçlı sohbet: araç çağrısı, araç sonucu, son turda `tool_choice: none`; sistem istemi ve araçlarda istem önbelleği, `effort`, `between_tools` düşünme,
4. yapılandırılmış çıktı (JSON şeması) hem Claude hem OpenAI uyumlu sağlayıcıda.

Gerçek anahtar kullanılmadı. Deneme, Messages ve Chat Completions protokollerini konuşan sahte bir sunucuya karşı çalıştı ve her kütüphanenin **gerçekte gönderdiği istek gövdesini** denetledi. Deneme kodu repoya girmedi; sonuç aşağıda.

| Kontrol | resmi SDK'lar | Genkit (Go) | langchaingo |
|---|---|---|---|
| Embedding (toplu) | geçti | geçti | geçti |
| Embedding token kullanımı görünür (maliyet) | geçti | **yok** | **yok** |
| pgvector arama + organizasyon süzgeci | geçti (kendi tablomuz, tipli sütun) | geçti (eklentinin tablo düzeni, süzgeç ham SQL metni) | geçti (sabit `langchain_pg_*` tabloları, süzgeç JSONB metadata) |
| Claude araç çağrısı | geçti | geçti | geçti |
| Sistem istemi `cache_control` | geçti | **yok** (`ai.WithSystem` önbellek işaretini taşımıyor) | geçti |
| Araç `cache_control` (CF-211 kararlı araç kesme noktası) | geçti | **yok** | **yok** |
| `thinking: between_tools` | geçti | geçti (eklenti anthropic-sdk-go v1.23'ü sabitliyor; elle v1.84'e yükseltince) | **yok** (model tablosu `adaptive` gönderdi) |
| `output_config.effort` | geçti | geçti | **yok** |
| Önbellek token kullanımı görünür | geçti (tipli) | geçti | geçti (tipsiz `GenerationInfo` haritası) |
| `tool_choice: none` | geçti | geçti (yeni model için yetenek tablosunu elle yazınca) | **yok** |
| Claude yapılandırılmış çıktı (`output_config.format`) | geçti | geçti (aynı elle yetenek tablosuyla) | **yok** (sadece istem ipucu) |
| OpenAI yapılandırılmış çıktı (`json_schema`) | geçti | geçti | **yok** (`json_object`) |
| OpenAI araç çağrısı | geçti | geçti | geçti |
| **Toplam** | **14/14** | **11/14** | **7/14** |

Sürümler: `anthropic-sdk-go` v1.84.0, `openai-go/v3` v3.99.0, `pgvector-go` v0.4.1; Genkit `go` v1.14.0; `langchaingo` v0.1.15.

Bağımlılık ölçümü (deneme ikilisi; `go build`, şeritlenmemiş):

| | modül | paket | ikili |
|---|---|---|---|
| resmi SDK'lar + pgvector-go + pgx | 28 | 114 | 52,9 MB |
| Genkit (anthropic + compat_oai + postgresql eklentileri) | 58 | 388 | 70,3 MB |
| langchaingo | 10 | 43 | 15,9 MB |

### 1.2 Puan tablosu

1 (kötü) ile 5 (iyi) arası; ağırlıklar issue'daki ölçütlerden.

| Ölçüt | Ağırlık | resmi SDK'lar | Genkit | langchaingo |
|---|---|---|---|---|
| Claude'un yeni özellikleri (araç, düşünme, istem önbelleği, effort, yapılandırılmış çıktı) | 3 | 5 | 3 | 1 |
| OpenAI uyumlu sağlayıcı (sohbet, araç, şema, embedding) | 2 | 5 | 4 | 2 |
| pgvector desteği (çok kiracılı şemaya, migration ve sqlc'ye uyum) | 2 | 4 | 3 | 3 |
| Mevcut davranışın korunması (failover, PII maskeleme, maliyet/kredi, guard, yeniden deneme) | 3 | 5 | 2 | 2 |
| Bakım riski | 2 | 5 | 3 | 2 |
| Bağımlılık boyutu | 1 | 2 | 1 | 5 |
| **Ağırlıklı ortalama** | | **4,62** | **2,77** | **2,15** |

Gerekçeler:

- **Genkit** kendi araç döngüsünü, kendi ara katmanını ve kendi kullanım modelini getiriyor. Bizim sınırlı araç döngümüzü (onay gerektiren araç reddi, tur sınırı, araç sonucunda guard ve maskeleme) korumak için Genkit'in döngüsünü kapatmak (`WithReturnToolRequests`) gerekiyor; geriye ince bir SDK sarmalayıcısı kalıyor, üstüne 58 modül. Yeni Claude modelleri için yetenek tablosu elle yazılıyor ve eklenti eski bir SDK sürümünü sabitliyor.
- **langchaingo** Claude'un bizim kullandığımız özelliklerinin yarısını göndermiyor; embedding token kullanımını düşürüyor (embedding maliyeti ölçülemez); pgvector tabloları organizasyon sütunu olmadan sabit.
- **Resmi SDK'lar** her özelliği tipli alanla gönderiyor, kullanım ve önbellek token'larını tipli döndürüyor ve bir istek ara katmanı (middleware) veriyor; failover, yeniden deneme ve reddetme eşlemesi aynen korunabiliyor. Referans repolar (otopoly-go, olexfilms-ecosystem) zaten `anthropic-sdk-go` kullanıyor. RAG parçaları (parçalama, pgvector sorgusu, yeniden sıralama) zaten bizde; kütüphane çerçevesine gerek yok.

### 1.3 Karar

**Resmi SDK'lar**: Claude için `github.com/anthropics/anthropic-sdk-go`, OpenAI uyumlu sağlayıcı (sohbet, embedding, ses dökümü) için `github.com/openai/openai-go/v3`. pgvector için (CF-214) `github.com/pgvector/pgvector-go` + pgx.

Bedeli ikili boyutu. Dockerfile bayraklarıyla (`-trimpath -ldflags="-s -w"`) `cmd/server` 76,6 MB'tan 106,0 MB'a çıktı (+29,3 MB, %38); `cmd/worker` da aynı paketi içerdiği için imaj yaklaşık iki katı kadar büyür. Ayrı ölçümde (boş bir programa eklenince, şeritlenmiş) `anthropic-sdk-go` +5,8 MB, `openai-go/v3` +18,9 MB getiriyor; kalan ortak bağımlılıklar (OpenTelemetry API'si, gjson). Yani büyümenin çoğu `openai-go/v3`'ten geliyor (paket çok geniş ve bölünmüyor). `openai-go` v1/v2 2025'ten beri güncellenmediği için seçilmedi. İkili boyutu sorun olursa geri dönüş yolu: OpenAI uyumlu tarafı tekrar küçük bir el yazısı istemciye almak; arayüz değişmediği için sadece `platform/ai/openai.go` değişir.

### 1.4 `platform/ai` arayüzünün altındaki yapı

Arayüz (`Provider`, `Embedder`, `Transcriber`, `Request`, `Result`, `Chat`, `Failover`, `Route`) değişmedi; çağıranlar dokunulmadan çalışıyor.

| Dosya | İş |
|---|---|
| `provider.go` | `HTTPProvider`, ortak middleware (yeniden deneme, reddetme eşlemesi, boyut sınırı, günlük), hata eşlemesi, akış okuma, JSON şeması doğrulama |
| `anthropic.go` | `Request` → `anthropic.MessageNewParams`; cevap ve akış olayları |
| `openai.go` | `Request` → `openai.ChatCompletionNewParams`; cevap, akış parçaları, embedding |
| `transcribe.go` | ses dökümü (`openai-go` çok parçalı gönderim) |
| `chat.go`, `failover.go`, `structured.go`, `guard.go` | değişmedi: araç döngüsü, failover, şema onarımı, PII maskeleme ve guard |

Korunan davranışlar ve nasıl:

- **Yeniden deneme**: SDK'ların kendi yeniden denemesi kapalı (`WithMaxRetries(0)`), çünkü bağlantı hatalarını da tekrar gönderiyor; bağlantı hatası faturalanmış bir istek olabilir. Middleware sadece açık 429/503/529 cevaplarını 250 ms / 500 ms beklemeyle en fazla üç kez dener. Ses dökümü hiç tekrar edilmez.
- **Failover**: middleware 2xx olmayan her cevabı `*Rejected{Status, Detail}` yapar; `Failover` ve `NotAccepted` aynı türe bakmaya devam ediyor.
- **Ortam değişkenleri**: SDK'ların `ANTHROPIC_*` / `OPENAI_*` okuması kapalı (`WithoutEnvironmentDefaults`, OpenAI servisleri istemci olmadan kuruluyor). Anahtar ve taban URL sadece kompozisyon kökünden (`aisystem`) gelir.
- **Akış**: istekler SDK'yla gönderilir, SSE gövdesi bizde okunur ve her olay SDK tipiyle çözülür. SDK'nın akış okuyucusu kullanılmadı: kesilen bir akışı bitmiş akıştan ayırmıyor (`message_stop` / `[DONE]` gelmeden kopan akış `ErrUnavailable` olmalı, kısmi metin ve kullanım korunmalı).
- **Boyut sınırı**: cevap gövdesi 4 MiB (embedding 16 MiB) ile sınırlı; aşan gövde `ErrInvalidOutput`.
- **Maliyet**: kullanım alanları aynı hesapla doldurulur (Claude'da `input = input + cache_read + cache_create`), `Route.ResultCost` değişmedi.
- `BaseURL` artık API köküdür: Claude için `https://api.anthropic.com` (SDK `/v1/messages` ekler); OpenAI uyumlu taraf için değişmedi (`.../v1`).

## 2. Genel akış

```
           ┌──────────── kaynaklar ────────────┐
           │ makaleler · geçmiş kayıtlar (8)    │
           │ temsilci onayları · vaka özetleri  │
           └───────────────┬────────────────────┘
                           ▼
      parçalama → PII maskeleme → kategorizasyon (9) → embedding (platform/ai)
                           ▼
                 knowledge_units + pgvector (4, 5)
                           ▼
 soru ─▶ kapsam süzgeci (3) ─▶ cevap önbelleği (6) ─isabet─▶ cevap
                │                    │ ıska
                │                    ▼
                │      hibrit arama (vektör + Meilisearch) + vaka hafızası (7)
                │                    ▼
                └────────▶ Chat (platform/ai): guard, maskeleme, maliyet, kredi
                                     ▼
                         cevap + kaynaklar → ölçüm (10)
```

Her adım organizasyon kapsamında çalışır (`orgctx`); hiçbir sorgu `organization_id` süzgeci olmadan çalışmaz.

## 3. Kapsam modeli

Bir bilgi birimi ve her arama dört düzeyli bir kapsam taşır:

| Düzey | Sütun | NULL anlamı | Kaynak |
|---|---|---|---|
| Organizasyon | `organization_id` (zorunlu) | - | `organizations` |
| Marka | `brand_id` | organizasyonun tüm markaları | `brands` (CF-194 çoklu marka) |
| Departman | `department_id` | organizasyon geneli | CF-215 ile gelir; o birleşene kadar her yerde NULL |
| Ekip | `team_id` | departmanın tüm ekipleri | `teams` |

Kurallar:

- Arama, sorunun kapsamındaki birimleri ve **daha geniş** kapsamdaki birimleri getirir: marka X, departman D sorusu `(brand_id = X OR brand_id IS NULL) AND (department_id = D OR department_id IS NULL)` birimlerini görür. Daha dar kapsam (başka bir ekip, başka bir marka) görünmez.
- Sıralamada dar kapsam öne geçer (aynı skor yakınlığında ekip > departman > marka > organizasyon).
- Müşteri AI'ı kapsamı kanaldan alır (widget → marka; widget departman seçimi CF-215). Temsilci asistanı kapsamı konuşmadan alır.
- Görünürlük ayrı bir eksen: `visibility` (`public`, `internal`). Müşteri AI'ı sadece `public` görür.

CF-215 bu bölüme departman tablosunu, görünürlük kurallarını ve devir akışını ekler.

### 3.1 Departmanlar (CF-215)

Not: departman kimlikleri iç `BIGINT` anahtardır. `knowledge_chunks.department_id` CF-222 ile (`000233_vector_department_bigint`) `BIGINT` olur ve bileşik yabancı anahtar alır; parça, makalesinin departmanını tetikleyiciyle kopyalar. Vektör aramasında departman süzgeci parça düzeyinde çalışır, makale düzeyindeki `department_visible` görünürlük kuralı olarak kalır (ayrıntı: 4. bölüm, Kapsam süzgeci).

Departman bir iş birimidir (teknik destek, satış, faturalama); ekip değildir. Ekipler bir departmana bağlanır, insanlar ekiplere. Kapsam sırası **organizasyon → marka → departman → ekip**.

#### Tablolar (migration `000226_departments`)

| Tablo / sütun | Anlamı |
|---|---|
| `departments` | Organizasyon altında, isteğe bağlı `brand_id` (marka). `name` organizasyonda tekil. `widget_visible`, `sort_order`, `sla_policy_id` (UUID, `sla_policies(organization_id, uuid)`), `timezone` + `working_hours` (NULL = organizasyonun saatleri), `ai_enabled`, `ai_instructions`, `routing_hint`. |
| `teams.department_id` | Ekibin departmanı; departman silinince NULL. |
| `knowledge_articles.department_id`, `knowledge_sources.department_id`, `business_context_sections.department_id`, `knowledge_suggestions.department_id` | NULL = organizasyon geneli. Departman bilgi sahibiyken silinemez (yabancı anahtar NO ACTION); kullanım durumu bilgiyi önce organizasyon geneline ya da başka departmana taşır. Öneri, konuşmanın departmanını tetikleyiciyle devralır. |
| `conversations.department_id`, `department_source` (`customer`, `ai`, `agent`, `rule`), `department_confidence`, `department_routed_at` | Konuşmanın departmanı, kimin belirlediği, AI güven puanı ve yönlendirmenin bir kez yapıldığı an. |
| `department_knowledge_shares` | Bir departmanın tek bir bilgisinin (`article`, `source`, `context`) başka bir departmanla paylaşımı. |
| `conversation_department_transfers` | Departman değişiklikleri: kimden, kime, kim (`customer`, `ai`, `agent`), güven puanı, neden ve devralan ekibe bağlam özeti. |

**Sütun tipi.** `department_id` her tabloda `BIGINT` ve `(organization_id, department_id) → departments(organization_id, id)` bileşik yabancı anahtarıdır (ekipler ve konuşma ekibiyle aynı kalıp). Paralel dallarda NULL olarak eklenen `department_id` sütunları bu tipe uyar ve birleşmeden sonra aynı yabancı anahtarı alır. `UUID` tipiyle eklenmiş tek sütun (`knowledge_chunks.department_id`, `000225_pgvector`) `000233_vector_department_bigint` ile `BIGINT`'e çevrildi ve `fk_knowledge_chunks_department` aldı (CF-222); migration sütunun boş olduğunu doğrular, değilse durur.

**Yabancı anahtarlar (CF-224, migration `000235_knowledge_department_fks`).** CF-216–CF-220 dallarında FK'siz eklenen `department_id` sütunları bu migration'la anahtarlandı: `history_imports`, `history_clusters`, `history_cases`, `history_decisions` (000229); `categories`, `category_assignments`, `category_labels` (000227); `ai_evaluation_cases`, `ai_evaluations`, `ai_evaluation_results` (000230); `knowledge_units`, `ai_case_memories`, `knowledge_gap_questions` (000228). Migration önce satırın organizasyonunda karşılığı olmayan değerleri `NULL` (organizasyon geneli) yapar, sonra anahtarı ekler; başka organizasyonun departmanı artık yazılamaz. 000227, 000228, 000229 ve 000230'daki "FK'yi CF-215 ekleyecek" yorumları geçersizdir: CF-215 bu anahtarları eklemedi, CF-224 ekledi (uygulanmış migration'lar değiştirilmez). Tek istisna `knowledge_chunks.department_id` (`UUID`); CF-222 çevirir. `departments` paketindeki şema testi FK'siz departman sütunu kalmadığını denetler.

**Departman silinince.** İki sınıf vardır:

| Sınıf | Tablolar | Silmede |
|---|---|---|
| Departmanın sahip olduğu bilgi | `knowledge_articles`, `knowledge_sources`, `business_context_sections`, `knowledge_suggestions` | `NO ACTION`: bilgi varken departman silinemez; silme isteği `?move_to=` ile bilgiyi önce taşır. |
| Bilgi hakkındaki kayıtlar ve atamalar | `teams`, `conversations`, `conversation_department_transfers` ve CF-224'ün anahtarladığı tüm tablolar | `ON DELETE SET NULL (department_id)`: satır kalır, organizasyon geneline düşer. |
| Paylaşım | `department_knowledge_shares` | `CASCADE`: paylaşım silinir. |

#### Görünürlük kuralı

Varsayılan kapalıdır. Kural tek yerde, SQL fonksiyonu `department_visible(org, kind, id, department_id, viewer_departments)`:

1. Organizasyon geneli bilgi (`department_id IS NULL`) her departmanda yedek olarak devrededir.
2. Departmanın bilgisi o departmanda görünür.
3. Departman lideri (departmanın bir ekibinin lideri) ya da `tenant.departments.write` sahibi tek bir bilgiyi başka departmanla paylaşır; paylaşılan departman o bilgiyi görür.
4. `tenant.departments.read_all` (sahip, yönetici) tüm departmanları görür.

Sorgular iki argüman alır: `dept_restrict` (false = süzgeçsiz) ve `viewer_departments` (`bigint[]`). Kapsam Go tarafında `internal/platform/deptscope` paketiyle bağlamda (context) taşınır, böylece RAG araması ve iş bağlamı imza değiştirmez:

| Çağıran | Kapsam |
|---|---|
| Müşteri AI'ı, AI ön getirme, triage (`aiagents.Inbound`) | `deptscope.Of(conversation.department_id)`: konuşmanın departmanı + organizasyon geneli; departmansız konuşma yalnız organizasyon geneli |
| Temsilci asistanı (`Copilot`) | konuşmanın departmanı |
| Yönetim listeleri ve tekil okuma (makale, kaynak, iş bağlamı, öneri, `/knowledge/search`) | üyenin ekiplerinin departmanları; `read_all` sahibi süzgeçsiz. Görmediği tekil kayıt 404 döner. |
| Kapsamsız bağlam (yeni çağıran) | kapalı: yalnız organizasyon geneli |

Yazma: `read_all` olmayan üye bilgiyi yalnız kendi departmanlarına (ya da organizasyon geneline) kaydeder, aksi 403. Lexical (Meilisearch) adayları Postgres'te yeniden doğrulanır, departman süzgeci orada uygulanır.

pgvector deposu (CF-214) ve bilgi birimleri (CF-216) aynı kuralı kullanır: paylaşımı olmayan tablolar için `department_id IS NULL OR department_id = ANY(viewer_departments)`, paylaşılan türler için `department_visible(...)`. Kapsam `deptscope.Effective(ctx).Args()` ile okunur. Bilgi parçalarında (`knowledge_chunks`) kural vektör sorgusunun içinde parçanın kendi `department_id`'sine uygulanır (`department_visible(org, 'article', article_id, department_id, viewer)`); hidrasyon sorgusu (`KnowledgeChunksByIDs`) aynı kuralı makaleye yeniden uygular.

#### Departman belirleme

1. **Widget.** Bootstrap `departments` listesini döner (widget'ta görünür, markası widget'ın markası ya da boş). Müşteri yeni konuşmada seçerse ilk mesaj `department_uuid` taşır; konuşma `customer` kaynağıyla o departmana girer. Seçim isteğe bağlıdır ("Emin değilim").
2. **AI tahmini.** Müşteri seçmezse AI ajanı ilk müşteri mesajında bir kez (`department_routed_at`) departmanı tahmin eder: adaylar departman adı, açıklaması ve `routing_hint` (güvenilmeyen etiket olarak) ile verilir, çıktı yapılandırılmış şemadır (`department_uuid` ya da null, `confidence`, `reason`), çağrı `triage` görevi olarak ölçülür. Güven **0,6** (`PredictThreshold`) ve üstüyse konuşma `ai` kaynağıyla departmana girer; altındaysa organizasyon geneli kalır ve temsilci devreder. Bilinmeyen bir uuid yok sayılır.
3. **Devir.** Temsilci (`tenant.conversations.assign`) `POST /v1/tenant/conversations/{uuid}/department-transfer` ile, AI ise cevap sırasında `transfer_department` aracıyla konuşmayı başka departmana devreder. Neden ve bağlam özeti zaman çizelgesine (`department_transferred` olayı) ve devir geçmişine yazılır; başka departmanın ekibi atanmışsa ekip ataması kalkar, istenirse temsilci de bırakılır. SLA saatleri hemen yeniden hesaplanır.

#### Departman ayarlarının etkisi

- **SLA:** konuşmanın departmanının politikası marka ve şirket politikasından önce gelir.
- **Çalışma saatleri:** kendi saati olmayan ekip departmanın saatini devralır; ekipsiz konuşma departmanın saatini kullanır.
- **AI:** `ai_enabled = false` departmanda AI cevap yazmaz, konuşmayı temsilciye devreder; `ai_instructions` cevap talimatlarına eklenir.

#### API ve izinler

`/v1/tenant/departments` (liste, oluştur, getir, güncelle, sil `?move_to=none|<uuid>`), `/v1/tenant/departments/{uuid}/shares`, `/v1/tenant/conversations/{uuid}/department`, `/v1/tenant/conversations/{uuid}/department-transfer`. İzinler: `tenant.departments.read` (tüm roller), `tenant.departments.write` ve `tenant.departments.read_all` (sahip, yönetici).

## 4. Vektör deposu: pgvector (CF-214)

### Veritabanı

- Postgres imajı `pgvector/pgvector:0.8.7-pg18-trixie` (PostgreSQL 18.7 + pgvector 0.8.7). Yerel, prod ve CI aynı imajı kullanır.
- Migration `000225_pgvector`: `vector` eklentisi; `knowledge_chunks.embedding` artık `vector(1536)` ve HNSW (`vector_cosine_ops`) indeksli. Eski `REAL[]` vektörler 1536 boyutluysa olduğu gibi taşınır; başka boyuttakiler `NULL` kalır ve arka plan işi saklı (maskelenmiş) metinden yeniden üretir. `ai_cosine` kaldırıldı.
- `knowledge_chunks` kapsam sütunları: `brand_id` (makaleden tetikleyiciyle kopyalanır, makalenin markası değişince parçalar da değişir), `department_id BIGINT` (CF-222: makaleden tetikleyiciyle kopyalanır, makalenin departmanı değişince parçalar da değişir; `(organization_id, department_id) → departments(organization_id, id)`, NO ACTION).

### Vektör tablosu sözleşmesi

Vektör tutan her tablo aynı sütunları taşır; böylece arama, yerleştirme ve yeniden vektörleme tek kodla yapılır (`backend/internal/platform/vectorstore`):

| Sütun | Anlamı |
|-------|--------|
| `id BIGINT`, `organization_id BIGINT` | Satır ve kiracı |
| `brand_id UUID NULL`, `department_id BIGINT NULL` | Kapsam; `NULL` = organizasyon geneli. `department_id` bileşik yabancı anahtarla `departments(organization_id, id)`'ye bağlanır |
| `embedding vector(1536) NULL`, `embedding_model TEXT NOT NULL` | Aranan vektör ve modeli |
| `pending_embedding vector(1536) NULL`, `pending_model TEXT NULL` | Model değişiminde hazırlanan yeni vektör |
| metin sütunu | Vektörün üretildiği maskelenmiş metin (`Source.TextColumn`) |

Yeni tablo için: migration'da bu sütunlar + `USING hnsw (embedding vector_cosine_ops)` indeksi; kodda bir `vectorstore.Source` tanımı ve `aisystem` içindeki `Reembedder.Sources` listesine ekleme. `pending_*` taşımayan tablolar `VectorTables` kaydına girer (aşağıda, CF-223).

### Tek model

- Model ve boyut tek yerde: `platform/ai/embedding.go` (`DefaultEmbeddingModel = text-embedding-3-small`, `EmbeddingDimensions = 1536`). `AI_EMBEDDING_MODEL` başka bir 1536 boyutlu modeli seçebilir; `text-embedding-3-*` modellerinden 1536 boyut istenir.
- Sağlayıcı çağrıları `platform/ai` arayüzünden geçer (`Embedder`, ek olarak `ModelEmbedder.EmbedModel`).
- Model değişince `app:ai:vector_reembed` (5 dakikada bir, maintenance kuyruğu) eski modeldeki satırları organizasyon organizasyon `pending_*` sütunlarına yeniden vektörler. Bu sırada arama eski vektörlerle sürer: sorgu vektörü de eski modelle üretilir (`vectorstore.SearchModel`). Organizasyonun bütün satırları hazır olunca tek `UPDATE` ile yeni modele geçilir. Geçiş sırasında yayımlanan makalelerin vektörleri de `pending_*`'e yazılır (`vectorstore.Place`), geçişle birlikte aranır hale gelir. Embedding çağrıları ölçülü embedder'dan geçer (organizasyonun AI kredisi ve politikası); AI'ı kapalı organizasyon bekletilir.

### Yeniden gömme kaydı: bütün vektör tabloları (CF-223)

`knowledge_chunks` dışındaki vektör tabloları `pending_*` sütunu taşımaz; her satır kendi `embedding_model` değerini taşır ve bu tablolara giden her sorgu yalnız sorgu modelindeki satırları karşılaştırır (`embedding_model=@model`, sorgu modeli hedef model). Bu yüzden bu tablolar yerinde, satır satır yeniden gömülür: yeniden gömülen satır hemen aranır hale gelir, eski modelde kalan satır hedef model sorgusunda hiç dönmez. Karışık model karşılaştırması olmaz; geçiş bitene kadar bu katmanlar eksik sonuç verebilir (ör. bellek katmanı ıska döner, normal yol devam eder).

Kayıt listesi `backend/internal/aisystem/vectors.go` (`VectorTables`), kod `vectorstore.Table`. Her kayıt tablosunu, vektör sütununu ve vektörün üretildiği metni (SQL ifadesi, yazan kodla aynı) bildirir:

| Tablo | Vektör | Metin kaynağı |
|-------|--------|---------------|
| `knowledge_units` (model) + `knowledge_unit_questions` (vektör) | `embedding` | `content` (soru ve varyantları); birimin bütün soruları ve modeli tek işlemde güncellenir |
| `ai_answer_cache` | vektörü yok | önbellek satırı birime bağlıdır; birim yeniden gömülünce satırları silinir (skor eski modelden), ilk eşleşmede yeniden dolar. `AnswerCacheGet` eski modeldeki birimi döndürmez |
| `ai_case_memories` | `embedding` | `problem + '\n' + resolution` |
| `category_examples` | `embedding` | `content`; hedef modelde aynı metin zaten yazılmışsa (tembel üretim) eski satır silinir |
| `category_labels` | `embedding` | `left(btrim(content), 6000)`; vektörsüz kaydedilmiş etiket (`embedding_model=''`) de vektörlenir. Etiketler değişmez kalır; 000234 tetikleyicisi yalnız vektör ve model değişimine izin verir |
| `history_cases` | `embedding`, `resolution_embedding` | `left(btrim(subject + '\n' + problem), 4000)` ve `resolution` (boş çözümün vektörü `NULL`) |
| `history_clusters` | `centroid` | metni yok: **üyelerinden yeniden hesaplanır** (aşağıda) |
| `history_decisions` | `embedding` | `left(btrim(title + '\n' + body), 6000)` |

- **Sıra ve parça:** iş önce `knowledge_chunks`, sonra listeyi sırayla işler. Her tablo için organizasyon başına `Batch` satırlık (varsayılan 64) en fazla `MaxBatches` (varsayılan 20) embedding çağrısı yapılır. Seçim "hedef modelde olmayan satırlar, `id` sırasıyla" olduğundan bir sonraki çalışma kaldığı yerden devam eder; ayrı ilerleme kaydı gerekmez. Yazma eski modeli koşul olarak kullanır, arada sahibi tarafından yeniden yazılmış satır ezilmez.
- **Maliyet:** embedding çağrıları ölçülü embedder'dan geçer, organizasyonun AI kredisine yazılır; AI'ı kapalı ya da kredisi bitmiş organizasyon bekletilir.
- **Metni saklanmayan vektörler (küme merkezleri):** seçilen yol **üyelerden yeniden hesaplamadır**. Merkez, üye vakaların problem vektörlerinin ortalamasıdır (kümeleme kodu da artımlı ortalama tutar). Organizasyonun bütün `history_cases` satırları yeni modele geçince merkez `avg(embedding)` ile yeniden yazılır ve küme yeni modele geçer. Hiç üyesi kalmamış küme **geçersiz işaretlenir** (`embedding_model=''`): hiçbir sorgu onu eşleştirmez, yeni vakalar yeni kümeler açar. Bu iş AI çağrısı yapmaz.
- Bilgi birimlerinin `source_hash` değeri modeli de içerir; makale bir sonraki yayımlanışında birimler yeniden çıkarılır (bu, yeni sürümde zaten olur).
- Yeni vektör tablosu eklerken: tabloyu bu listeye ekle, metin ifadesini vektörü yazan kodla aynı tut, sorgularda `embedding_model` süzgecini koru.


### Kapsam süzgeci

`vectorstore.Scope` (org zorunlu; marka, departman):

- Markalı konuşma: o markanın bilgisi + markasız (organizasyon geneli) bilgi; markanın kendi bilgisi önce gelir, organizasyon geneli yedektir. Başka markanın bilgisi hiç gelmez.
- Markasız konuşma: sadece organizasyon geneli bilgi.
- `vectorstore.Organization(org)`: süzgeçsiz; sadece konuşmaya bağlı olmayan iç aramalar (yönetim araması, ekip sohbeti, yardım merkezi).
- Departman (CF-222): `Scope.DepartmentID` (`pgtype.Int8`) konuşmanın departmanıdır, o departmanın parçaları önce gelir. `Scope.Departments` görünür departmanlardır; organizasyon geneli parçalar (`department_id IS NULL`) her zaman yedektir. `vectorstore.Conversation(org, brand, department)` ikisini konuşmanın departmanıyla doldurur; departmansız konuşma yalnız organizasyon geneli bilgiyi görür. Süzgeç vektör sorgusunun içinde, parçanın `department_id`'sine uygulanır; böylece başka departmanın parçaları aday sınırını (40) doldurmaz. Kaynak kendi kuralını `Source.DepartmentVisible` ile verir; bilgi parçaları paylaşımlar için `department_visible(...)` kullanır, verilmezse `department_id IS NULL OR department_id = ANY(@scope_departments)`.
- RAG (`SearchScoped`) görünürlüğü bağlamdaki `deptscope`'tan alır (`Scope.WithDepartments`): yönetim araması üyenin departmanlarını, `read_all` sahibi tümünü görür. Bağlamda kapsam yoksa konuşma kapsamının departmanı kullanılır (yine kapalı: departman + organizasyon geneli). Makale düzeyindeki `department_visible` hidrasyonda ve lexical sorgularda görünürlük kuralı olarak kalır; parçadaki kopya eskimişse bile makalenin departmanı geçerlidir.
- Testler: `TestSearchScopedDepartmentFallbackAndTenantIsolation` (departman kapsamı, paylaşım, organizasyon geneli yedek, kiracı ayrımı, departman taşıma, bileşik anahtar).

`vectorstore.Search(ctx, pool, source, scope, Query{Vector, Model, MinSimilarity, Limit, Args})` kosinüs benzerliği eşiği geçen satırları döner (`Hit{ID, Similarity, Specific}`); çağıran satırları kendi sorgusuyla doldurur ve yeniden doğrular. HNSW `iterative_scan=relaxed_order` ile taranır, böylece kapsam süzgeci sonucu boş bırakmaz. Bilgi parçaları için eşik `KnowledgeMinSimilarity = 0.2`.

Kullananlar: müşteri AI cevabı ve ön getirme, AI aracı `knowledge_search`, temsilci asistanı (copilot), benzer konuşmalar (`AISimilarConversations`: aynı marka önce, sonra markasız; başka marka hiç).

### Marka personası

Konuşmanın markasının `ai_persona` metni müşteri AI'ının sistem istemine "güvenilmeyen ton ve kimlik tercihi" olarak girer (`AgentOptions.BrandPersona`, en fazla 4000 bayt). Organizasyonun genel AI personası (`StableData`) aynen kalır.

## 5. Bilgi birimleri (CF-216)

Bilgi birimi, aramanın döndürdüğü en küçük anlamlı parçadır. Kaynağı ne olursa olsun aynı tabloda durur:

| Alan | Not |
|---|---|
| `organization_id`, `brand_id`, `department_id`, `team_id` | bölüm 3 |
| `source_kind` | `article`, `case`, `approval`, `import` |
| `source_id` / `source_version` | kaynağa geri bağ; makale sürümü değişince eski birim pasif olur |
| `visibility` | `public` / `internal` |
| `content` (maskeli), `embedding`, `embedding_model` | bölüm 4 |
| `category_id`, etiketler | bölüm 9 |
| `evidence` | onay sayısı, kaynak güveni; aramada ağırlık |
| `status` | `active`, `superseded`, `rejected` |

Tablo adı: `knowledge_units`. Mevcut `knowledge_chunks` makale kaynağı olarak bu tabloya taşınır veya yanında durur; karar CF-216'nın.

## 6. Cevap önbelleği (CF-216)

- Anahtar: kapsam + normalleştirilmiş soru + soru embedding'i (eşik üstü benzerlik isabet sayılır).
- Değer: cevap metni, kullanılan birimler ve sürümleri.
- Geçersizlik: kullanılan birimlerden biri değişir, pasif olur veya reddedilirse kayıt düşer; ayrıca süre sınırı.
- İsabet de ölçülür (bölüm 10) ve kredi kuralları ölçümde açıkça yazılır.
- Müşteri AI'ı sadece `public` birimlerden kurulmuş önbellek kayıtlarını kullanır.

Tablo adı önerisi: `ai_answer_cache`.

## 7. Vaka hafızası (CF-216)

- Kapanan konuşmalardan maskeli bir **vaka özeti** (sorun, çözüm, kategori) çıkarılır ve `source_kind = case` bilgi birimi olur.
- Benzer açık konuşmada temsilci asistanı "benzer vakalar"ı gösterir; müşteri AI'ı sadece onaylanmış (`evidence` eşiği üstü) vakaları kanıt olarak kullanır.
- Aynı müşterinin geçmişi (kişi bağlamı) ayrı tutulur ve sadece o müşterinin konuşmasında kullanılır.

### 5–7 ayrıntı (CF-216)

Modül: `backend/internal/modules/aimemory`. Migration: `000228_knowledge_units`.

#### Bilgi birimi (`knowledge_units`, `knowledge_unit_questions`)

Bir birim; soru, en fazla beş soru varyantı, cevap, kaynak (makale ya da onaylı öneri), kategori, marka, departman ve vektör taşır. Soru ve her varyant ayrı ayrı vektörlenir; eşleşme en yakın ifadeye göre yapılır.

- **Makale:** `knowledge.article_published` olayında yayımlı sürümden üretilir. Görünürlük makaleninkidir (`public` ya da `internal`). Marka makalenin markasıdır.
- **Onaylı öneri:** `knowledge.suggestion_reviewed` olayında (AI ajanının öneri onayı bu olayı yazar) üretilir. Her zaman `internal`'dir; öneriden doğan taslak makale yayımlanınca kendi `public` birimlerini alır.
- **Yeniden üretim:** Kaynak özeti (`source_hash`: sürüm, metin, görünürlük, marka, embedding modeli) aynıysa AI çağrısı yapılmaz. Değiştiyse birimler silinip yeniden üretilir; `ON DELETE CASCADE` ile o birimlere bağlı önbellek kayıtları da silinir. Yayından kaldırılan ya da silinen makalenin birimleri silinir.
- **Çıkarım:** `reply` görevi kullanılır (özel iş bağlamı almaz), şema 20 birim ve birim başına 5 varyantla sınırlıdır. Enjeksiyon korumasından geçmeyen birim atılır.

#### Cevap önbelleği (`ai_answer_cache`, `ai_answer_cache_stats`)

Hem müşteri AI'ı (`audience=customer`) hem temsilci asistanı (`audience=agent`) kullanır. Arama sırası:

1. **Birebir tekrar:** Soru normalize edilir (kişisel veri sabit yer tutucuya çevrilir, küçük harf, noktalama ve fazla boşluk atılır) ve SHA-256 ile önbellekte aranır. İsabette **hiç AI çağrısı yoktur**.
2. **Anlamsal eşleşme:** Bir embedding çağrısı yapılır, birimler aranır. Eşik `0,90` kosinüstür. Eşik üstündeki adaylar arasında en dar kapsam seçilir (departman, marka, sonra organizasyon geneli). Sonuç önbelleğe yazılır. **Üretim çağrısı yoktur.**
3. **Iska:** Normal AI yolu çalışır. Varsa benzer vaka özetleri modele ipucu olarak eklenir.

Müşteri AI'ı yalnız `public` birimleri görür; temsilci asistanı `internal` birimleri de görür. Önbellek kaydı birimin görünürlüğünü okuma anında tekrar kontrol eder.

İsabet oranı organizasyon, gün ve kitle başına sayılır: `(exact + semantic) / (exact + semantic + miss)`. `GET /v1/tenant/ai/answer-cache/stats` ile okunur.

Müşteri akışında arama, bilgi tabanı ön getirmesinden **önce** yapılır. Birim isabet ederse ön getirmenin embedding çağrısı da yapılmaz. Konuşmada iş akışı (procedure), bağlı sistem verisi ya da ekran bağlamı varsa önbellekten cevap verilmez, çünkü cevap müşteriye özeldir.

#### Geçmiş vaka hafızası (`ai_case_memories`)

`conversations.resolved` olayında çözülmüş ya da kapanmış konuşmanın anonim problem/çözüm özeti çıkarılır ve vektörlenir. Konuşmada hem müşteri hem temsilci (ya da bot) mesajı yoksa özet çıkarılmaz.

**Güvenlik kuralı (kesin):** Müşteri AI'ı yalnız anonimleştirilmiş **ve** onaylı özetleri kullanır. Başka bir müşterinin konuşma metni, adı, iletişim bilgisi veya firma adı müşteriye asla dönmez. Bunu dört katman sağlar:

1. **Modele gitmeden önce:** Transkript, kişinin adı, e-postası, telefonu, firma adı ve alan adı ile kalıp tabanlı kişisel verilerden (e-posta, telefon, kart, IBAN, TCKN, URL) temizlenir. Model kimliği hiç görmez.
2. **Modelden sonra:** Özet aynı temizlikten tekrar geçer. Kimliğin herhangi bir parçası kalırsa `anonymized=false` olur. Bu özet onaylanamaz; veritabanı da `CHECK(status<>'approved' OR anonymized)` ile engeller.
3. **Onay:** Yeni özet `pending` başlar. Müşteri AI'ı yalnız `approved` olanı görür. Yeniden özetlenen konuşma tekrar `pending`'e döner.
4. **Okuma anında:** Müşteri yolunda özet, kaynak kişi ve firma kimliğine göre bir kez daha temizlenir. Hâlâ sızıntı varsa kayıt atlanır. Müşteriye dönen tip (`CustomerCase`) yalnız problem, çözüm, kategori ve skor taşır; konuşma, kişi ya da firma referansı içermez.

Temsilci asistanı `pending` ve `approved` özetleri görür. Kaynak konuşmanın bağlantısı yalnız temsilci o konuşmayı açabiliyorsa döner (inbox ekip görünürlüğü, `access.CanSee`). Soran konuşma kendi sonuçlarına girmez.

Sızıntı testi: `TestCustomerCaseMemoryNeverLeaksAnotherCustomer`. Model kimliği özete kopyalayacak şekilde kasıtlı olarak bozulur. Testin doğruladıkları: modele giden girdi, saklanan özet ve müşteri sonucu kimlik içermez; bekleyen özet müşteriye gitmez; anonim olmayan özet onaylanamaz; onaydan sonra özete elle eklenen ad da müşteriye ulaşmaz.

#### Kapsam süzgeci

Birim, önbellek, vaka ve bilgi açığı satırları `brand_id` ve `department_id` taşır. Bir arama yalnız aynı markanın ve departmanın satırlarını, ayrıca organizasyon geneli (`NULL`) satırları görür. Markası olmayan konuşma yalnız organizasyon geneli satırları görür. Departman CF-215 ile gelir; o zamana kadar `department_id` NULL'dır ve yabancı anahtarı yoktur.

#### Bilgi açığı (`knowledge_gap_questions`)

Eşleşen birim yoksa ve AI yolu da geçerli bir kaynağa dayanamazsa soru (kişisel veri maskelenmiş olarak) bilgi açığına yazılır. Müşteri tarafında bu, kaynaksız ya da devredilen cevaptır. Temsilci tarafında kaynaksız ve vakasız Copilot cevabıdır. Mevcut `AIKnowledgeGapTopics` raporu bu soruları da konu başına sayar. Ham sorular `GET /v1/tenant/ai/knowledge-gaps/questions` ile listelenir.

#### API

| Yol | Yetki | Ne yapar |
|-----|-------|----------|
| `POST /v1/tenant/ai/memory/search` | `tenant.conversations.reply` | Temsilci asistanı araması: birim cevabı ya da benzer vakalar |
| `GET /v1/tenant/ai/case-memories` | `tenant.settings.read` | Vaka özetleri (durum süzgeci) |
| `POST /v1/tenant/ai/case-memories/{uuid}/review` | `tenant.settings.write` | Onay veya ret; anonim olmayan özet `409` döner |
| `GET /v1/tenant/ai/knowledge-units` | `tenant.settings.read` | Bilgi birimleri |
| `GET /v1/tenant/ai/answer-cache/stats` | `tenant.settings.read` | İsabet oranı |
| `GET /v1/tenant/ai/knowledge-gaps/questions` | `tenant.settings.read` | Cevapsız sorular |

#### CF-214'e bağımlılık

`knowledge_unit_questions.embedding` ve `ai_case_memories.embedding` `vector(1536)` sütunlarıdır (CF-214). Benzerlik `KnowledgeUnitMatch` ve `CaseMemoryMatch` sorgularında kosinüs uzaklığıyla (`<=>`) hesaplanır.

## 8. Geçmiş kayıt hattı (CF-217)

Ayrıntılı şema ve akış: [docs/gecmis-kayitlar.md](../gecmis-kayitlar.md).

**Amaç.** Başka sistemlerden dışa aktarımlar ve kendi kapanan konuşmalarımız tek seferlik içe aktarma değil, sürekli çalışan bir hattan geçip bilgi önerisine dönüşür.

**Bileşenler** (`backend/internal/modules/history`):

| Parça | Görev |
|-------|-------|
| `usecase.Source` | Kaynak adaptörü. `FileSource` (yüklenen dosya), `ConversationSource` (gece işi). Bağlayıcılar `Service.Connectors` haritasına adıyla eklenir. |
| `usecase.BuildCase` | Sunucuda anonimleştirme; sağlayıcıya ve arşive giden metnin tek çıkış noktası. `ai.Anonymizer` kayıttaki yapılandırılmış alanları da siler. |
| `usecase.Taxonomy` | Kategori (CF-218 arayüzü). |
| Kümeleme | Kapsam başına (organizasyon, marka, departman, kategori, model) merkez vektörlü açgözlü kümeleme; kümeler işler arasında kalıcı. |
| Kanıt | Kümedeki çözüm embedding'lerinden birbirine benzeyen en büyük grup. |
| `usecase.Drafter` | Taslak yazımı; varsayılanı AI çekirdeğinden (`aicore` → `platform/ai`) geçer, bütçe ve politika uygulanır. |
| Karar hafızası | `history_decisions`: karar, gerekçe, düzeltme ve embedding. Red, aynı kümeyi ve benzer taslakları bastırır. |
| `usecase.Directory` | Departman doğrulama ve "departman lideri onaylar" kuralı (CF-215 arayüzü). |

**Veri.** `history_settings`, `history_imports` (iş, onay, ham dosya, ilerleme), `history_clusters`, `history_cases` (salt okunur arşiv), `history_decisions`; `knowledge_suggestions` tablosuna `source`, `cluster_id`, `import_id`, `evidence_count`, `conflict_article_id`, `conflict_score` eklendi (`department_id` CF-215'in 000226'sından gelir) ve `conversation_id` NULL olabilir oldu.

**Gizlilik.** Analiz onayı olmadan iş açılmaz (veritabanı kısıtı). Eğitim onayı ayrı alanda ve vaka başına `training_allowed` olarak saklanır. Ham dosya iş bitince silinir. Arşivde kaynak talep numarası yerine özeti tutulur. Kurallar `platform/ai` içinde olduğu için tüm AI çalıştırmalarında maskeleme genişledi (vergi no, firma, adres, isim).

**İş.** Worker'da dakikalık `app:history:sweep` (ham dosya ve arşiv saklama, sıradaki işler) ve her gece 01:17 UTC `app:history:nightly`. İşler kaldığı yerden devam eder: arşivlenmiş vakalar (aynı özet) atlanır.

### 8.1 Toplu AI işleri ve günlük sınır (CF-228)

**Sorun.** Prod'da 5531 kayıtlık bir yükleme `ai: daily calls exceeded` ile yarıda kaldı. Toplu iş günlük çağrı sınırının tamamını harcadığı için aynı organizasyonda canlı AI (müşteri AI'ı, temsilci asistanı) gün sonuna kadar durdu; ekranda yalnız genel "processing failed" görünüyordu.

**Şerit (lane).** Her AI çağrısı `live` ya da `batch` şeridindedir (`ai_runs.lane`, migration `000236`). Arka plan işleri bağlamı `ai.WithBatch(ctx)` ile işaretler: geçmiş hattı (`history.Process`), yeniden vektörleştirme (worker'daki `app:ai:vector_reembed` görevi) ve ölçüm (`aiagents.evaluate`, elle ve otomatik). İşaret, embedding ve üretim çağrılarının ikisinde de `aicore/repository.Store.Reserve` içinde okunur.

**Pay.** `ai_policies.batch_share_percent` (%10–90, varsayılan %50) toplu işlerin günlük çağrı ve bütçe sınırından kullanabileceği paydır. Sınırlar CF-229'daki gibi `Effective` = min(platform, kiracı) ile bulunur, pay bunun üstüne uygulanır (`BatchShare`):

- Toplu çağrı: toplam sınır **ve** `batch_calls < pay` (bütçe için de aynı).
- Canlı çağrı: yalnız toplam sınır. Canlı AI, toplu işlerin kullanmadığı payı da kullanabilir; toplu işler canlıya ayrılan kısmı (%100 − pay) hiçbir zaman tüketemez.
- Sınırsız platform limiti sınırsız kalır. Eşzamanlılık iki şeritte ortaktır.
- Hata türleri aynıdır (`ErrDaily`, `ErrBudget`); toplu payın dolması `"…: batch share used"` ile sarılır.
- Pay AI ayarları → Güvenlik sekmesinden değiştirilir; `GET /v1/tenant/ai/policy` yanıtı `batch_share_percent` ve `batch_limits` döndürür.

**Duraklat ve devam et.** `ErrDaily` ve `ErrBudget` işi bitirmez: iş `paused` olur, `error_code` (`daily_limit`/`budget`), `resume_at` (İstanbul'da bir sonraki gece yarısı + 1 dk; `AIUsageToday` günü bu sınırla sayar) ve `resume_offset` kaydedilir. `ErrConcurrency` iki dakikalık duraklamadır. Dakikalık sweep `resume_at` geçince işi yeniden alır. `resume_offset` önce kayıt dizinidir (yarıda kalan paket baştan tekrarlanır, sayaçları geri alınır), sonra `len(kayıtlar) + i` ile id sırasındaki i. küme değerlendirmesidir. Devam eden iş sayaçlarını ve dokunduğu kümeleri (`HistoryClustersOfImport`) yükler. Duraklayan işin ham dosyası saklama süresi boyunca kalır; süre dolarsa iş `raw_expired` ile başarısız olur. Duraklayan iş beklerken anında iptal edilebilir.

**Paket boyutu.** Paket 100 kayıttır (200 metin). Embedding çağrısı en çok 256 metin ve 250.000 bayt taşır (`model.MaxEmbedInputs`, `MaxEmbedBytes`). Bayt token sayısının üst sınırıdır; sağlayıcının istek başına 300.000 token sınırının altında kalır. Büyük paket gerekirse birden çok çağrıya bölünür (`embedChunks`). 32 kayıtlık eski pakete göre çağrı sayısı yaklaşık üçte birine iner.

**Görünür neden.** `history_imports.error_code` sınıflandırılır: `daily_limit`, `budget`, `concurrency`, `credits`, `quota`, `ai_disabled`, `not_configured`, `provider`, `invalid`, `unavailable`, `unsafe_record`, `raw_expired`, `unknown`. Panel kodu TR/EN gösterir, duraklayan işte "sınır yenilenince devam edecek" ve tahmini saat yazar. `error` İngilizce mesaj olarak kalır. Başarısız iş Sentry'ye `warning` düzeyinde gider (`history_error_code` etiketi, kod başına parmak izi). Duraklama beklenen durumdur, yalnız INFO loglanır.

**Ön tahmin.** `POST /v1/tenant/history/imports/estimate` dosyayı saklamadan ayrıştırır. Yanıt şunları verir: embedding çağrı sayısı (dosya için kesin), öneri başına ek çağrı (2: taslak ve karşılaştırma embedding'i), tahmini token (bayt/4) ve maliyet, bugün kalan toplu pay, gereken gün sayısı ve ham dosya saklama süresinin yetip yetmediği. Panel dosya seçilince tahmini gösterir; bugünkü payı veya saklama süresini aşacaksa yüklemeden önce uyarır.

**Sonraki adımlar.** Vektörler `vector(1536)` ve `<=>` sorgularıyla çalışır (birleştirmede taşındı). Geçmiş tablolarındaki `department_id` için departman FK'si; CF-218 ile `Taxonomy`; CF-215 ile `Directory` (departman lideri) bağlanır.

## 9. Kategorizasyon (CF-218)

### Taksonomi

- `categories`: organizasyon başına tek ağaç. Kök düğümler organizasyon geneli (`department_id` NULL) ya da bir departmana ait olabilir. Departman düğümünün altı aynı departmanda kalır. Organizasyon geneli bir düğümün altında departman dalı açılabilir.
- En fazla 5 seviye (`depth` 0..4). `slug` organizasyon içinde tekildir.
- Kullanılan bir düğüm (alt düğüm, kayıt ya da etiket) silinmez, arşivlenir. Arşiv alt ağaca yayılır. Arşivli düğüm sınıflandırmada aday olmaz, etiketlerini korur.
- Görünürlük: `department` ile listelemede organizasyon geneli düğümler ve o departmanın dalları gelir. Departmanı olmayan bir kayıt tüm ağaçtan kategori alabilir. D departmanındaki kayıt organizasyon geneli düğümlerden ve D dalından alır.
- CF-215 birleşene kadar `department_id` sütununun yabancı anahtarı yoktur. Departman uuid'leri `usecase.Departments` arayüzüyle çözülür. Varsayılan `NoDepartments` departman kapsamını `400 department unavailable` ile reddeder, sessizce düşürmez. CF-215 kendi uygulamasını verir ve `departments(organization_id,id)` yabancı anahtarını ekler.

### Her kayıtta kategori

- `category_assignments`: (organizasyon, `subject_type`, `subject_id`) başına bir satır. Satırda kategori, güven puanı (0..1), kaynak (`embedding`, `ai`, `human`, ileride `model`), sınıflandırıcı adı ve sürümü (`embedding/v1`, `ai/v1`, `human`) ile `needs_review` bulunur.
- `subject_type`: `conversation`, `archive_record` (CF-217), `knowledge_unit` (CF-216), `suggestion` (`knowledge_suggestions`). Arşiv ve birim tabloları başka dallarda geldiği için `subject_id` yabancı anahtarsızdır. Her modül kendi kaydını silerken atamasını da siler.
- Kayıt türleri `Service.RegisterSubject(kind, SubjectSource)` ile takılır. `SubjectSource.Load(ctx, org, actor, uuid)`, `actor > 0` olduğunda görünürlüğü kontrol eder: konuşmalarda gelen kutusu kuralları, önerilerde `tenant.settings.read`. `actor = 0` worker yoludur.
- Otomatik sınıflandırma insan kararının üstüne yazmaz. `force=true` yalnız makine kararını yeniler.

### Sınıflandırıcı (faz 1) ve değiştirilebilir arayüz

`backend/internal/modules/categories/classify` tek sözleşmedir:

```go
type Classifier interface {
    Classify(ctx, Input, []Candidate) (Result, error)
}
type Stage interface {
    Name() string
    Predict(ctx, Input, []Candidate) (Prediction, error) // Decided=false: çekimser / düşük güven
}
```

- `Chain` aşamaları sırayla çalıştırır ve ilk kararlı tahminde durur. Aşama hatası (sağlayıcı kapalı, AI politikası kapalı, kredi yok) ölümcül değildir; zincir sonraki aşamaya geçer. Hiçbir aşama karar vermezse en iyi tahmin öneri olarak saklanır ve kayıt `needs_review` ile temsilciye düşer.
- **Embedding aşaması:** kayıt metni, kategori örnekleriyle (ad + açıklama + `examples`) ve organizasyonun etiketli kayıtlarıyla karşılaştırılır; kategori başına en yüksek kosinüs alınır. Karar için skor ≥ 0,82 ve ikinciye fark ≥ 0,04 gerekir. Örnek vektörleri tembel üretilir (`category_examples`); metin ya da model değişince yeniden üretilir.
- **AI aşaması:** `aicore.RunAgent` (task `triage`, şema zorunlu). Maskeleme, guard, bütçe ve kredi aynı yoldan geçer; sağlayıcıya doğrudan çağrı yoktur. Karar için öz bildirimli güven ≥ 0,7 gerekir.
- **İnsan:** son güven < 0,6 ise `needs_review`. Kayıtlar inceleme kuyruğunda bekler (`GET /v1/tenant/categories/review`).
- **CF-219:** organizasyon modeli bir `Stage` olarak `Chain.Prepend` ile öne takılır. Emin değilse `Decided=false` döner ve faz 1'e düşülür. Kaynak `model` olarak kaydedilir.
- **CF-217:** arşiv kayıtları `Service.ClassifySubject(ctx, org, "archive_record", Subject{...}, force)` ile aynı yoldan sınıflandırılır. Doğrudan sınıflandırıcıya erişim için `Service.Classifier()` kullanılır.
- Gelen kutusu: `message.received` tüketicisi (`categories.classify`), ilk gelen müşteri mesajında kategorisi olmayan konuşmayı bir kez sınıflandırır.

### Etiket deposu ve dışa aktarma

- `category_labels`: temsilcinin her onayı (`confirm`) ve düzeltmesi (`correct`) için bir satır. Satır maskelenmiş metni, vektörünü ve embedding modelini, eski kategoriyi, eski kaynağı ve güveni, yeni kategoriyi, kimin yaptığını ve zamanı tutar. Satırlar yalnız eklenir; tek istisna, vektörü sonradan eklemektir.
- Etiketler embedding aşamasında da örnek olarak kullanılır, yani her düzeltme bir sonraki sınıflandırmayı hemen etkiler.
- `GET /v1/tenant/categories/labels/export` (`tenant.categories.export`) organizasyonun etiketlerini JSON Lines olarak verir; `since` ile artımlı alınabilir. Satır biçimi `CategoryLabelExport` şemasındadır. Başka organizasyonun verisi hiçbir zaman dönmez.
- Vektör sütunları `vector(1536)` (CF-214); benzerlik kosinüs uzaklığıyla (`<=>`) hesaplanır.

### İzinler

| Slug | Ne | Varsayılan roller |
|---|---|---|
| `tenant.categories.read` | Ağacı ve kayıt kategorisini görmek | owner, admin, supervisor, member, agent |
| `tenant.categories.manage` | Ağacı yönetmek | owner, admin |
| `tenant.categories.label` | Onaylamak, düzeltmek, sınıflandırmayı çalıştırmak, inceleme kuyruğu | owner, admin, supervisor, member, agent |
| `tenant.categories.export` | Etiketleri dışa aktarmak | owner, admin, supervisor |

## 10. Ölçüm (CF-220)

Bilgi katmanının işe yarayıp yaramadığını ölçer ve gösterir.

### Ölçüm seti

- Kayıt: `ai_evaluation_cases`. Bu iş şu alanları ekler: `department_id` (NULL = organizasyon geneli, CF-215'e kadar yabancı anahtarsız), `expected_answer`, `expected_article_uuids`, `active`. Kategori (`category`) zaten vardı.
- Soru kısa biçimde girilebilir (`question`); `Customer: <soru>` olarak transcript'e yazılır. Bir vakada beklenen cevap, beklenen kaynak veya `expected` kontrollerinden (içerir, içermez, devir, en düşük güven) en az biri olmalıdır.
- Vaka güncellenince revizyonu artar; eski sonuçlarla karşılaştırma yalnızca aynı revizyonda yapılır.

### Puanlama (LLM yargıcı yok, deterministik)

| Ölçü | Tanım |
|------|-------|
| Doğru cevap oranı | Tamamlanan vakalarda: `expected` kontrollerinin hepsi geçer **ve** cevap, beklenen cevabın anlamlı kelimelerinin en az %60'ını içerir ve devir yoktur. Sadece kaynak verilmiş vakada: devirsiz, boş olmayan ve beklenen kaynağa atıf yapan cevap. |
| Kaynak isabeti | Beklenen kaynağı olan vakalarda, cevabın atıf yaptığı makalelerden biri beklenenlerden biri mi. |
| Önbellek isabeti | `AnswerCache.Lookup` (CF-216) soruyu cevaplayabilir mi: müşteri kitlesi için tam önbellek kaydı ya da eşiği geçen bilgi birimi, vakanın departmanı kapsamında. Uyarlayıcı (`MemoryCache`) `aimemory` `Probe`'u kullanır; sayaç artmaz, önbelleğe yazılmaz, bu yüzden ölçüm çalıştırmaları paneldeki isabet oranını oynatmaz (en çok bir embedding çağrısı). Embedding modeli yoksa önbellek bağlanmaz ve ölçü null kalır. |
| Ortalama gecikme | Tamamlanan vakalarda simülasyon süresi (ms). |
| Soru başına maliyet | Çalıştırmanın toplam sağlayıcı maliyeti / vaka sayısı (mikro USD). |

Ölçülemeyen ölçü `null` kalır; sıfır olarak gösterilmez.

### Önce / sonra

- Her çalıştırma (`ai_evaluations`) ölçüleri sütun olarak saklar (`answer_accuracy`, `source_hit_rate`, `cache_hit_rate`, `avg_latency_ms`, `cost_per_case_micros`), raporda (`report.metrics`) departman kırılımıyla tutar.
- Açık bir `baseline_uuid` verilmezse aynı kapsamın (organizasyon ya da departman) önceki tamamlanmış çalıştırması "önce"dir; rapordaki `before` alanı onun ölçüleridir, `regressions` puanı düşen vakalardır.
- `trigger_kind`: `manual`, `initial`, `config_change`, `model_change`, `knowledge_change`. `fingerprint`: ölçümün yapıldığı AI ajanı ayar revizyonu, son cevap modeli ve yayınlanmış bilginin durumu.

### Otomatik çalıştırma

- `ai_evaluation_watches` organizasyon başına isteğe bağlıdır (`PUT /v1/tenant/ai/evaluations/watch`). Açan üye kaydedilir; otomatik çalıştırmalar onun adına ve AI kredisi harcayarak yapılır.
- `app:ai:evaluation_sweep` 5 dakikada bir en çok 3 organizasyonu kontrol eder (son kontrolden 10 dakika geçmiş olanlar, kira ile). Parmak izi değiştiyse ve aktif vaka varsa set çalışır; sonraki karşılaştırma için çalıştırma sonrası parmak izi saklanır (çalıştırmanın kendi cevap modeli değişiklik sayılmaz).

### Panel

`GET /v1/tenant/analytics/ai-knowledge?from&until&department_uuid` (varsayılan son 30 gün). Bölümler: önbellek isabeti, AI çağrıları ve maliyet (göreve göre), bilgi açıkları, öneri hattı (bekleyen, onaylanan, reddedilen), kategori dağılımı ve sınıflandırma güveni (5 eşit aralık, <0,6 düşük güven), devir oranı, son ölçüm ve öncekisi.

Her bölüm `available` ve gerekirse `reason` taşır:

- `source_not_connected`: veriyi üreten katman bağlı değil (sağlayıcı alanı nil).
- `department_scope_unavailable`: kaynak departman ayrımı yapamıyor; organizasyon verisi departman verisi gibi gösterilmez. CF-225'ten sonra yerleşik kaynakların hepsi departmana göre süzülür, bu neden yalnız departmansız yeni bir kaynak için kalır.

Departman seçilince süzgeç (CF-225):

| Bölüm | Departman nereden gelir |
|-------|-------------------------|
| AI çağrıları ve maliyet (`ai_runs`) | Çalıştırmanın konuşmasının **şu anki** departmanı. Konuşmasız çalıştırmalar (embedding, içe aktarma) yalnız organizasyon toplamında sayılır. |
| Devir oranı, bilgi açıkları, triage kategorileri (`ai_agent_jobs`) | İşin konuşmasının şu anki departmanı. |
| Öneri hattı (`knowledge_suggestions`) | Önerinin kendi `department_id`'si (konuşmasından devralınır). |
| Kategori dağılımı (`category_assignments`) | Kaydın konuşmasının şu anki departmanı. |
| Önbellek isabeti (`ai_answer_cache_stats`) | Aramanın kapsamındaki departman (aşağıya bakın). |
| Son ölçüm (`ai_evaluations`) | Çalıştırmanın `department_id`'si. |

Önbellek sayaçları (migration `000237`): `ai_answer_cache_stats` her gün, kitle ve departman için bir satır tutar (`department_id` NULL = departmansız arama, `UNIQUE NULLS NOT DISTINCT`). Organizasyon toplamı tüm satırların toplamıdır. Departman silinince sayaçları da silinir (`ON DELETE CASCADE`; `SET NULL` aynı günün organizasyon satırıyla çakışırdı). Sayaçlar günlük olduğu için panelde pencere tam günlere genişler.

Kategori bölümü: kategori deposu (CF-218) pencerede konuşma sınıflandırmışsa ondan okunur; hiç sınıflandırma yoksa (kategori ağacı kurulmamış) son tamamlanan triage sonucu kullanılır.

Sağlayıcı arayüzleri (`analytics/usecase/knowledge.go`, `aiagents/usecase/measurement.go`):

| Arayüz | Kim bağlar | Yokken |
|--------|------------|--------|
| Arayüz | Uyarlayıcı (CF-225) | Yokken |
|--------|---------------------|--------|
| `DepartmentSource` (panel) | `analytics/repository.DepartmentStore` (`departments` tablosu) | Departman listesi boş; `department_uuid` 400 `DEPARTMENTS_UNAVAILABLE` |
| `DepartmentDirectory` (ölçüm), kategoriler `Departments` | `departments/usecase.Directory` | Departmanlı vaka ve çalıştırma 400 `DEPARTMENTS_UNAVAILABLE`; kategori ağacında departman kapsamı reddedilir |
| `CacheSource` (panel) | `analytics/repository.CacheStore` (`ai_answer_cache_stats`) | Önbellek bölümü "veri yok" |
| `AnswerCache` (ölçüm) | `aiagents/usecase.MemoryCache` (`aimemory` `Probe`) | Ölçümde önbellek isabeti null |
| `ClassificationSource` | `analytics/repository.CategoryStore` (`category_assignments`) | Kategori ve güven son tamamlanan triage sonucundan okunur |
| `KnowledgeStore` (yerleşik) | `analytics/repository.KnowledgeStore` | Departman kapsamında `ErrDepartmentScope` |

Bağlantı: API sunucusu (`httpserver/server.go`) paneli, ölçüm setini ve kategori servisini; worker (`cmd/worker`) otomatik ölçüm taramasının (`app:ai:evaluation_sweep`) kullandığı AI ajanı servisini ve kategori servisini aynı uyarlayıcılarla kurar (`agentusecase.WireMeasurement`).

Birleştirme notu: CF-215 ve CF-216 `cf-toplu`'ya girdiğinde bu arayüzler bağlanmamıştı (alanlar nil); CF-225 bağladı. `aiagents` içinde CF-220 alanı, CF-215'in `Departments` (yönlendirici) alanıyla çakışmasın diye `DepartmentSets` adını aldı.

### Yetki

- `tenant.ai_insights.read`: panel, ölçüm seti, çalıştırma geçmişi ve otomatik çalıştırma ayarını görme.
- `tenant.ai_insights.manage`: vaka ekleme ve düzenleme, çalıştırma, otomatik çalıştırmayı açıp kapama.
- Migration, `tenant.settings.read` / `tenant.settings.write` tutan her role (sistem veya özel) karşılık gelen yeni izni verir; ölçüm uçları önceden bu izinlerle korunuyordu. `owner`, `admin` ve `supervisor` sistem rolleri ayrıca `read` alır.

## 11. Ortak kurallar

- Sağlayıcıya doğrudan HTTP çağrısı yazılmaz; her çağrı `platform/ai` arayüzünden ve ölçümlü sarmalayıcılardan (`aicore.MeteredEmbedder`, `MeteredTranscriber`, AI çalıştırma kayıtları) geçer.
- Embedding'e giden metin maskelenir; çıktı guard'dan geçer.
- Paralel dallarda migration numaraları çakışabilir; PR açmadan önce `cf-toplu`'daki son numaraya göre yeniden numaralanır.
- PR hedefi `cf-toplu`.
