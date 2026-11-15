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

## 4. Vektör deposu: pgvector (CF-214)

### Veritabanı

- Postgres imajı `pgvector/pgvector:0.8.7-pg18-trixie` (PostgreSQL 18.7 + pgvector 0.8.7). Yerel, prod ve CI aynı imajı kullanır.
- Migration `000225_pgvector`: `vector` eklentisi; `knowledge_chunks.embedding` artık `vector(1536)` ve HNSW (`vector_cosine_ops`) indeksli. Eski `REAL[]` vektörler 1536 boyutluysa olduğu gibi taşınır; başka boyuttakiler `NULL` kalır ve arka plan işi saklı (maskelenmiş) metinden yeniden üretir. `ai_cosine` kaldırıldı.
- `knowledge_chunks` kapsam sütunları: `brand_id` (makaleden tetikleyiciyle kopyalanır, makalenin markası değişince parçalar da değişir), `department_id` (CF-215 gelene kadar `NULL`).

### Vektör tablosu sözleşmesi

Vektör tutan her tablo aynı sütunları taşır; böylece arama, yerleştirme ve yeniden vektörleme tek kodla yapılır (`backend/internal/platform/vectorstore`):

| Sütun | Anlamı |
|-------|--------|
| `id BIGINT`, `organization_id BIGINT` | Satır ve kiracı |
| `brand_id UUID NULL`, `department_id UUID NULL` | Kapsam; `NULL` = organizasyon geneli |
| `embedding vector(1536) NULL`, `embedding_model TEXT NOT NULL` | Aranan vektör ve modeli |
| `pending_embedding vector(1536) NULL`, `pending_model TEXT NULL` | Model değişiminde hazırlanan yeni vektör |
| metin sütunu | Vektörün üretildiği maskelenmiş metin (`Source.TextColumn`) |

Yeni tablo için: migration'da bu sütunlar + `USING hnsw (embedding vector_cosine_ops)` indeksi; kodda bir `vectorstore.Source` tanımı ve `aisystem` içindeki `Reembedder.Sources` listesine ekleme.

### Tek model

- Model ve boyut tek yerde: `platform/ai/embedding.go` (`DefaultEmbeddingModel = text-embedding-3-small`, `EmbeddingDimensions = 1536`). `AI_EMBEDDING_MODEL` başka bir 1536 boyutlu modeli seçebilir; `text-embedding-3-*` modellerinden 1536 boyut istenir.
- Sağlayıcı çağrıları `platform/ai` arayüzünden geçer (`Embedder`, ek olarak `ModelEmbedder.EmbedModel`).
- Model değişince `app:ai:vector_reembed` (5 dakikada bir, maintenance kuyruğu) eski modeldeki satırları organizasyon organizasyon `pending_*` sütunlarına yeniden vektörler. Bu sırada arama eski vektörlerle sürer: sorgu vektörü de eski modelle üretilir (`vectorstore.SearchModel`). Organizasyonun bütün satırları hazır olunca tek `UPDATE` ile yeni modele geçilir. Geçiş sırasında yayımlanan makalelerin vektörleri de `pending_*`'e yazılır (`vectorstore.Place`), geçişle birlikte aranır hale gelir. Embedding çağrıları ölçülü embedder'dan geçer (organizasyonun AI kredisi ve politikası); AI'ı kapalı organizasyon bekletilir.

### Kapsam süzgeci

`vectorstore.Scope` (org zorunlu; marka, departman):

- Markalı konuşma: o markanın bilgisi + markasız (organizasyon geneli) bilgi; markanın kendi bilgisi önce gelir, organizasyon geneli yedektir. Başka markanın bilgisi hiç gelmez.
- Markasız konuşma: sadece organizasyon geneli bilgi.
- `vectorstore.Organization(org)`: süzgeçsiz; sadece konuşmaya bağlı olmayan iç aramalar (yönetim araması, ekip sohbeti, yardım merkezi).
- Departman aynı kuralla çalışır (CF-215 doldurur).

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

## 8. Geçmiş kayıt hattı (CF-217)

Kaynaklar: dosya yükleme (CSV/JSON dökümler), bağlayıcılar (dış yardım masası), gece işi (kendi kapanmış konuşmalarımız).

Aşamalar: içe al → anonimleştir (PII maskeleme, `ai.RedactPII`) → böl → kategorize et (bölüm 9) → kanıt eşiği → bilgi birimi. Eşiğin altındaki adaylar temsilci onayına düşer; onaylar pgvector'da saklanır (**onay hafızası**) ve aynı soru bir daha geldiğinde kanıt sayılır.

Her aşama idempotent bir iş (`asynq`) ve organizasyon kapsamında; maliyet her embedding/sohbet çağrısında ölçülür.

## 9. Kategorizasyon (CF-218)

### 9.1 Taksonomi

Departman başına kategori ağacı (CF-215 departmanları). Departman yokken organizasyon geneli tek ağaç.

### 9.2 Her kayıtta kategori ve etiket

Konuşma, bilgi birimi ve vaka özeti bir kategori ve etiketler taşır. Etiketler bir etiket deposunda (kim, ne zaman, insan mı AI mı) tutulur; bu depo CF-219'un eğitim verisidir.

### 9.3 Sınıflandırma modeli (CF-219, planlandı)

Biriken etiketlerle organizasyona özel sınıflandırıcı. Kapsam ve veri sözleşmesi CF-219'da.

## 10. Ölçüm (CF-220)

- Departman başına soru setleri (soru, beklenen kaynak, beklenen cevap özeti).
- Bir değişiklikten (yeni embedding modeli, yeni birimler, önbellek eşiği) önce ve sonra aynı set koşulur; isabet, kaynak doğruluğu, maliyet ve gecikme karşılaştırılır.
- Analiz paneli: önbellek isabet oranı, cevapsız sorular (bilgi açıkları), kategori dağılımı, maliyet.

## 11. Ortak kurallar

- Sağlayıcıya doğrudan HTTP çağrısı yazılmaz; her çağrı `platform/ai` arayüzünden ve ölçümlü sarmalayıcılardan (`aicore.MeteredEmbedder`, `MeteredTranscriber`, AI çalıştırma kayıtları) geçer.
- Embedding'e giden metin maskelenir; çıktı guard'dan geçer.
- Paralel dallarda migration numaraları çakışabilir; PR açmadan önce `cf-toplu`'daki son numaraya göre yeniden numaralanır.
- PR hedefi `cf-toplu`.
