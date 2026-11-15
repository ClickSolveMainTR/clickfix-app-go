# Bilgi katmanı

AI bilgi katmanının (CF-213 ile CF-220) ortak tasarımı. Her iş kendi bölümünü ekler.

## CF-214: pgvector, tek embedding modeli, kapsam süzgeci

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
