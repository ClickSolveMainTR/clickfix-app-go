# Bilgi katmanı

AI her soruda sıfırdan arama yapıp cevap yazmasın; önceden işlenmiş bilgiden cevap versin. Bu doküman CF-213 ile CF-220 arasındaki işlerin ortak tasarım kaynağıdır. Her iş kendi bölümünü ekler.

Ortak kurallar:

- Sağlayıcı çağrıları yalnız `backend/internal/platform/ai` arayüzünden geçer (sohbet için aicore `RunAgent`, embedding için ölçülü embedder).
- Kapsam sırası organizasyon → marka → departman → ekip. `NULL` o seviyede "organizasyon geneli" demektir.
- Vektörler pgvector'da tutulur (CF-214). Tek embedding modeli kullanılır; satırlar `embedding_model` taşır, model değişince eski satırlar eşleşmez ve yeniden üretilir.

## CF-216: Bilgi birimleri, cevap önbelleği ve geçmiş vaka hafızası

Modül: `backend/internal/modules/aimemory`. Migration: `000225_knowledge_units`.

### Bilgi birimi (`knowledge_units`, `knowledge_unit_questions`)

Bir birim; soru, en fazla beş soru varyantı, cevap, kaynak (makale ya da onaylı öneri), kategori, marka, departman ve vektör taşır. Soru ve her varyant ayrı ayrı vektörlenir; eşleşme en yakın ifadeye göre yapılır.

- **Makale:** `knowledge.article_published` olayında yayımlı sürümden üretilir. Görünürlük makaleninkidir (`public` ya da `internal`). Marka makalenin markasıdır.
- **Onaylı öneri:** `knowledge.suggestion_reviewed` olayında (AI ajanının öneri onayı bu olayı yazar) üretilir. Her zaman `internal`'dir; öneriden doğan taslak makale yayımlanınca kendi `public` birimlerini alır.
- **Yeniden üretim:** Kaynak özeti (`source_hash`: sürüm, metin, görünürlük, marka, embedding modeli) aynıysa AI çağrısı yapılmaz. Değiştiyse birimler silinip yeniden üretilir; `ON DELETE CASCADE` ile o birimlere bağlı önbellek kayıtları da silinir. Yayından kaldırılan ya da silinen makalenin birimleri silinir.
- **Çıkarım:** `reply` görevi kullanılır (özel iş bağlamı almaz), şema 20 birim ve birim başına 5 varyantla sınırlıdır. Enjeksiyon korumasından geçmeyen birim atılır.

### Cevap önbelleği (`ai_answer_cache`, `ai_answer_cache_stats`)

Hem müşteri AI'ı (`audience=customer`) hem temsilci asistanı (`audience=agent`) kullanır. Arama sırası:

1. **Birebir tekrar:** Soru normalize edilir (kişisel veri sabit yer tutucuya çevrilir, küçük harf, noktalama ve fazla boşluk atılır) ve SHA-256 ile önbellekte aranır. İsabette **hiç AI çağrısı yoktur**.
2. **Anlamsal eşleşme:** Bir embedding çağrısı yapılır, birimler aranır. Eşik `0,90` kosinüstür. Eşik üstündeki adaylar arasında en dar kapsam seçilir (departman, marka, sonra organizasyon geneli). Sonuç önbelleğe yazılır. **Üretim çağrısı yoktur.**
3. **Iska:** Normal AI yolu çalışır. Varsa benzer vaka özetleri modele ipucu olarak eklenir.

Müşteri AI'ı yalnız `public` birimleri görür; temsilci asistanı `internal` birimleri de görür. Önbellek kaydı birimin görünürlüğünü okuma anında tekrar kontrol eder.

İsabet oranı organizasyon, gün ve kitle başına sayılır: `(exact + semantic) / (exact + semantic + miss)`. `GET /v1/tenant/ai/answer-cache/stats` ile okunur.

Müşteri akışında arama, bilgi tabanı ön getirmesinden **önce** yapılır. Birim isabet ederse ön getirmenin embedding çağrısı da yapılmaz. Konuşmada iş akışı (procedure), bağlı sistem verisi ya da ekran bağlamı varsa önbellekten cevap verilmez, çünkü cevap müşteriye özeldir.

### Geçmiş vaka hafızası (`ai_case_memories`)

`conversations.resolved` olayında çözülmüş ya da kapanmış konuşmanın anonim problem/çözüm özeti çıkarılır ve vektörlenir. Konuşmada hem müşteri hem temsilci (ya da bot) mesajı yoksa özet çıkarılmaz.

**Güvenlik kuralı (kesin):** Müşteri AI'ı yalnız anonimleştirilmiş **ve** onaylı özetleri kullanır. Başka bir müşterinin konuşma metni, adı, iletişim bilgisi veya firma adı müşteriye asla dönmez. Bunu dört katman sağlar:

1. **Modele gitmeden önce:** Transkript, kişinin adı, e-postası, telefonu, firma adı ve alan adı ile kalıp tabanlı kişisel verilerden (e-posta, telefon, kart, IBAN, TCKN, URL) temizlenir. Model kimliği hiç görmez.
2. **Modelden sonra:** Özet aynı temizlikten tekrar geçer. Kimliğin herhangi bir parçası kalırsa `anonymized=false` olur. Bu özet onaylanamaz; veritabanı da `CHECK(status<>'approved' OR anonymized)` ile engeller.
3. **Onay:** Yeni özet `pending` başlar. Müşteri AI'ı yalnız `approved` olanı görür. Yeniden özetlenen konuşma tekrar `pending`'e döner.
4. **Okuma anında:** Müşteri yolunda özet, kaynak kişi ve firma kimliğine göre bir kez daha temizlenir. Hâlâ sızıntı varsa kayıt atlanır. Müşteriye dönen tip (`CustomerCase`) yalnız problem, çözüm, kategori ve skor taşır; konuşma, kişi ya da firma referansı içermez.

Temsilci asistanı `pending` ve `approved` özetleri görür. Kaynak konuşmanın bağlantısı yalnız temsilci o konuşmayı açabiliyorsa döner (inbox ekip görünürlüğü, `access.CanSee`). Soran konuşma kendi sonuçlarına girmez.

Sızıntı testi: `TestCustomerCaseMemoryNeverLeaksAnotherCustomer`. Model kimliği özete kopyalayacak şekilde kasıtlı olarak bozulur. Testin doğruladıkları: modele giden girdi, saklanan özet ve müşteri sonucu kimlik içermez; bekleyen özet müşteriye gitmez; anonim olmayan özet onaylanamaz; onaydan sonra özete elle eklenen ad da müşteriye ulaşmaz.

### Kapsam süzgeci

Birim, önbellek, vaka ve bilgi açığı satırları `brand_id` ve `department_id` taşır. Bir arama yalnız aynı markanın ve departmanın satırlarını, ayrıca organizasyon geneli (`NULL`) satırları görür. Markası olmayan konuşma yalnız organizasyon geneli satırları görür. Departman CF-215 ile gelir; o zamana kadar `department_id` NULL'dır ve yabancı anahtarı yoktur.

### Bilgi açığı (`knowledge_gap_questions`)

Eşleşen birim yoksa ve AI yolu da geçerli bir kaynağa dayanamazsa soru (kişisel veri maskelenmiş olarak) bilgi açığına yazılır. Müşteri tarafında bu, kaynaksız ya da devredilen cevaptır. Temsilci tarafında kaynaksız ve vakasız Copilot cevabıdır. Mevcut `AIKnowledgeGapTopics` raporu bu soruları da konu başına sayar. Ham sorular `GET /v1/tenant/ai/knowledge-gaps/questions` ile listelenir.

### API

| Yol | Yetki | Ne yapar |
|-----|-------|----------|
| `POST /v1/tenant/ai/memory/search` | `tenant.conversations.reply` | Temsilci asistanı araması: birim cevabı ya da benzer vakalar |
| `GET /v1/tenant/ai/case-memories` | `tenant.settings.read` | Vaka özetleri (durum süzgeci) |
| `POST /v1/tenant/ai/case-memories/{uuid}/review` | `tenant.settings.write` | Onay veya ret; anonim olmayan özet `409` döner |
| `GET /v1/tenant/ai/knowledge-units` | `tenant.settings.read` | Bilgi birimleri |
| `GET /v1/tenant/ai/answer-cache/stats` | `tenant.settings.read` | İsabet oranı |
| `GET /v1/tenant/ai/knowledge-gaps/questions` | `tenant.settings.read` | Cevapsız sorular |

### CF-214'e bağımlılık

CF-214 (pgvector ve ortak vektör yardımcısı) bu iş yazılırken birleşmemişti. Stok `postgres:18` imajı ve CI çalışmaya devam etsin diye vektör sütunları 000060'taki taşınabilir `REAL[]` ve `ai_cosine` yardımcısını kullanır. CF-214 birleşince `knowledge_unit_questions.embedding` ve `ai_case_memories.embedding` sütunları `vector(n)` tipine ve ortak yardımcıya taşınır. Sorgular yalnız `KnowledgeUnitMatch` ve `CaseMemoryMatch` içinde olduğu için değişiklik bu iki sorguyla sınırlı kalır.
