# Bilgi katmanı

AI bilgi katmanının tasarım kaynağı (CF-213 – CF-220). Her iş kendi bölümünü ekler.

## CF-220: Ölçüm ve panel

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
| Önbellek isabeti | `AnswerCache.Lookup` (CF-216) soruyu cevaplayabilir mi. Önbellek bağlı değilse ölçülmez (null). |
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

- `source_not_connected`: veriyi üreten katman henüz yok (önbellek, CF-216).
- `department_scope_unavailable`: kaynak departman ayrımı yapamıyor. Konuşmalar, AI çalıştırmaları, triage ve öneriler henüz departman taşımadığı için yerleşik kaynaklar departman seçilince veri göstermez; organizasyon verisi departman verisi gibi gösterilmez.

Sağlayıcı arayüzleri (`analytics/usecase/knowledge.go`, `aiagents/usecase/measurement.go`):

| Arayüz | Kim bağlar | Yokken |
|--------|------------|--------|
| `DepartmentSource`, `DepartmentDirectory` | CF-215 | Departman listesi boş; `department_uuid` 400 `DEPARTMENTS_UNAVAILABLE` |
| `CacheSource`, `AnswerCache` | CF-216 | Önbellek bölümü "veri yok", ölçümde önbellek isabeti null |
| `ClassificationSource` | CF-218 | Kategori ve güven son tamamlanan triage sonucundan okunur |
| `KnowledgeStore` (yerleşik) | CF-215 sonrası departman süzgeci | Departman kapsamında `ErrDepartmentScope` |

### Yetki

- `tenant.ai_insights.read`: panel, ölçüm seti, çalıştırma geçmişi ve otomatik çalıştırma ayarını görme.
- `tenant.ai_insights.manage`: vaka ekleme ve düzenleme, çalıştırma, otomatik çalıştırmayı açıp kapama.
- Migration, `tenant.settings.read` / `tenant.settings.write` tutan her role (sistem veya özel) karşılık gelen yeni izni verir; ölçüm uçları önceden bu izinlerle korunuyordu. `owner`, `admin` ve `supervisor` sistem rolleri ayrıca `read` alır.
