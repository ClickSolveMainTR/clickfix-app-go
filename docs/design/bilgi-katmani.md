# Bilgi katmanı

Bilgi katmanı işlerinin (CF-213 ile CF-220) ortak tasarım belgesi. İskeleti CF-213 yazar; her iş kendi bölümünü ekler.

## CF-218: Kategorizasyon

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
- Vektör sütunları bugün `REAL[]` + `ai_cosine` (knowledge_chunks ile aynı). CF-214 bu sütunları `vector(N)` ve HNSW indeksine taşır.

### İzinler

| Slug | Ne | Varsayılan roller |
|---|---|---|
| `tenant.categories.read` | Ağacı ve kayıt kategorisini görmek | owner, admin, supervisor, member, agent |
| `tenant.categories.manage` | Ağacı yönetmek | owner, admin |
| `tenant.categories.label` | Onaylamak, düzeltmek, sınıflandırmayı çalıştırmak, inceleme kuyruğu | owner, admin, supervisor, member, agent |
| `tenant.categories.export` | Etiketleri dışa aktarmak | owner, admin, supervisor |
