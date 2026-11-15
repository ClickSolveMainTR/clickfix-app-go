# Bilgi katmanı (CF-213 – CF-220)

Bu belge bilgi katmanı işlerinin ortak tasarım kaynağıdır; her iş kendi bölümünü ekler.

Ortak kurallar: sağlayıcı çağrıları yalnız `backend/internal/platform/ai` arayüzünden geçer; kapsam sırası organizasyon → marka → departman → ekip (`department_id` NULL = organizasyon geneli, CF-215'e kadar); vektörler pgvector'da (CF-214), tek embedding modeli ve sabit boyut.

## CF-217: Geçmiş kayıt hattı

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

**Veri.** `history_settings`, `history_imports` (iş, onay, ham dosya, ilerleme), `history_clusters`, `history_cases` (salt okunur arşiv), `history_decisions`; `knowledge_suggestions` tablosuna `source`, `cluster_id`, `import_id`, `department_id`, `evidence_count`, `conflict_article_id`, `conflict_score` eklendi ve `conversation_id` NULL olabilir oldu.

**Gizlilik.** Analiz onayı olmadan iş açılmaz (veritabanı kısıtı). Eğitim onayı ayrı alanda ve vaka başına `training_allowed` olarak saklanır. Ham dosya iş bitince silinir. Arşivde kaynak talep numarası yerine özeti tutulur. Kurallar `platform/ai` içinde olduğu için tüm AI çalıştırmalarında maskeleme genişledi (vergi no, firma, adres, isim).

**İş.** Worker'da dakikalık `app:history:sweep` (ham dosya ve arşiv saklama, sıradaki işler) ve her gece 01:17 UTC `app:history:nightly`. İşler kaldığı yerden devam eder: arşivlenmiş vakalar (aynı özet) atlanır.

**Sonraki adımlar.** CF-214 ile `REAL[]` → `vector(n)` ve `<=>` sorguları; CF-218 ile `Taxonomy`; CF-215 ile `Directory` (departman lideri) bağlanır.
