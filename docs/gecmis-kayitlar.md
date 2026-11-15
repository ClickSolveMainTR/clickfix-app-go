# Geçmiş kayıtlar: dosya şeması ve hat (CF-217)

Geçmiş destek kayıtlarını bilgiye dönüştüren kalıcı ve otomatik özellik. Kaynaklar:

- **Dosya yükleme:** başka bir sistemden alınmış dışa aktarım (`.json` veya `.zip`), aşağıdaki şemada.
- **Gece işi:** her gece, organizasyonun kendi kapanan konuşmaları aynı hattan geçer (Ayarlar'dan açılır; organizasyonun analiz onayı gerekir).
- **Bağlayıcılar:** Zendesk, Freshdesk gibi sistemler ileride `usecase.Source` arayüzünü uygulayarak eklenir (`history_imports.source = 'connector'`, `connector` = adaptör adı).

Panel: **Ayarlar → Bilgi bankası → Geçmiş kayıtlar**. Örnek dosya: [`docs/examples/gecmis-ornek.json`](./examples/gecmis-ornek.json). Panelde "Örnek dosyayı indir" ile de alınır. Dosyadaki tüm isim, numara ve adresler uydurmadır.

## Dosya şeması `clickfix.history/v1`

Tek bir JSON nesnesi:

```json
{
  "schema": "clickfix.history/v1",
  "source_system": "ornek-yardim-masasi",
  "locale": "tr",
  "agents":   [{ "id": "a-1", "name": "..." }],
  "contacts": [{ "id": "c-1", "name": "...", "email": "...", "phone": "...", "company": "...", "tax_number": "...", "address": "..." }],
  "tickets":  [{ "id": "t-1", "contact_id": "c-1", "subject": "...", "status": "solved", "category": "Fatura", "resolution": "...", "locale": "tr", "created_at": "2026-01-05T09:12:00Z", "closed_at": "2026-01-05T10:02:00Z" }],
  "messages": [{ "ticket_id": "t-1", "author": "customer", "agent_id": "", "body": "...", "created_at": "2026-01-05T09:12:00Z" }]
}
```

| Alan | Zorunlu | Açıklama |
|------|---------|----------|
| `schema` | hayır | Verilirse `clickfix.history/v1` olmalı. |
| `source_system`, `locale` | hayır | Bilgi amaçlı; `locale` `tr` veya `en` (varsayılan `tr`). |
| `agents[]` | hayır | Temsilci adları yalnız mesajlardan silinmek için okunur. |
| `contacts[]` | hayır | Kişinin alanları (isim, e-posta, telefon, firma, vergi no, adres) mesaj metinlerinden de silinir. Hiçbiri saklanmaz. |
| `tickets[].id` | evet | Kaynak sistemdeki talep numarası (en çok 200 karakter, dosyada tekil). Yalnız SHA-256 özeti saklanır; aynı dosya tekrar yüklenirse aynı talep iki kez sayılmaz. |
| `tickets[].status` | hayır | Yalnız `solved`, `closed`, `resolved` (veya `çözüldü`, `kapandı`) işlenir; diğerleri atlanır. |
| `tickets[].category` | hayır | Kategori taksonomisi (CF-218) gelene kadar gruplamada kullanılır. |
| `tickets[].resolution` | hayır | Açık çözüm özeti. Yoksa son temsilci mesajları çözüm sayılır. |
| `messages[].ticket_id` | evet | Dosyadaki bir talebe bağlanmalı. |
| `messages[].author` | hayır | `customer` (varsayılan), `agent` veya `system`. |
| `messages[].body` | evet | Mesaj metni (en çok 20 000 karakter, fazlası kesilir). |

**Zip:** kökte veya bir klasörde ya `history.json` (yukarıdaki nesne) ya da ayrı ayrı `tickets.json`, `messages.json`, `contacts.json`, `agents.json` (her biri dizi) bulunur.

**Sınırlar:** dosya en çok 50 MB, açılmış zip en çok 200 MB, en çok 20 000 talep ve 200 000 mesaj. Bilinmeyen alanlar yok sayılır.

## Hat

1. **Yükleme** (`POST /v1/tenant/history/imports`, `tenant.history.manage`): dosya doğrulanır ve özel depoya konur. Veri sahibinin **analiz onayı zorunludur**; **eğitim onayı** isteğe bağlıdır ve ayrı kaydedilir (`consent_analysis`, `consent_training`, `consent_by`, `consent_at`, `consent_version`). İşleme worker'da yapılır.
2. **Okuma ve anonimleştirme** (sunucuda): `platform/ai` maskeleme kuralları (e-posta, telefon, TCKN, IBAN, kart; CF-217 ile vergi no, firma, adres ve isim kalıpları) ve kaydın yapılandırılmış alanlarından gelen değerler geri döndürülemez biçimde `[PII_*]` ile değiştirilir. Arşive ve sağlayıcıya yalnız bu metin gider.
3. **Arşiv:** vakalar `history_cases` tablosunda organizasyon, marka ve departman kimliğiyle **salt okunur** saklanır (metin değiştirilemez; tetikleyici engeller). Gelen kutusunda konuşma olarak görünmez.
4. **Kategorize et ve grupla:** kategori `Taxonomy` arayüzünden gelir (CF-218 bağlanana kadar dosyanın kendi kategorisi). Vakalar embedding benzerliğiyle (`cluster_similarity`) aynı kapsamdaki (organizasyon → marka → departman → kategori) kümeye eklenir; kümeler işler arasında kalıcıdır, kanıt birikir.
5. **Kanıt eşiği:** kümedeki çözümler karşılaştırılır; aynı çözümü (`resolution_similarity`) paylaşan en büyük vaka grubu kümenin kanıtıdır. Kanıt `evidence_threshold` değerine ulaşınca grup vakalarından bir makale taslağı yazılır ve mevcut `knowledge_suggestions` akışına `source = 'history'` olarak eklenir. Yayındaki bir makaleye çok benzeyen öneri `conflict_article_*` ile işaretlenir.
6. **Onay ve karar hafızası** (`POST /v1/tenant/history/suggestions/{uuid}/decision`, `tenant.history.review`): onay, red veya düzeltme gerekçesiyle `history_decisions` tablosunda embedding ile saklanır. Onay ve düzeltme iç makale taslağı oluşturur (yayınlamak ayrı karardır). Reddedilen kümeden, ya da taslağı reddedilen bir öneriye `rejection_similarity` kadar benzeyen kümeden, tekrar öneri çıkmaz.
7. **AI sınırları (CF-228):** hat toplu iştir; günlük AI çağrı ve bütçe sınırının yalnız toplu iş payını kullanır (varsayılan %50, AI ayarları → Güvenlik). Böylece canlı AI çalışmaya devam eder. Pay dolunca iş başarısız olmaz, **duraklar** (`paused`). Gün dönümünde (İstanbul) kaldığı paketten devam eder. Yüklemeden önce `POST /v1/tenant/history/imports/estimate` tahmini çağrı sayısını, maliyeti ve kaç gün süreceğini verir. Panel, bugünkü payı aşacaksa uyarır. Ayrıntı: [bilgi-katmani.md 8.1](./design/bilgi-katmani.md#81-toplu-ai-işleri-ve-günlük-sınır-cf-228).
8. **İş takibi:** her iş organizasyona aittir; panelde aşama ve sayaçlar görünür. Bekleyen iş hemen, çalışan iş bir sonraki partide **iptal** edilir. Bitmiş, başarısız veya iptal edilmiş iş **geri alınır**: arşivlenen vakaları, yalnız onlara dayanan bekleyen öneriler ve boş kalan kümeler silinir; onaylanmış öneriler ve karar hafızası kalır.

## Saklama

- **Ham dosya** iş nasıl biterse bitsin hemen silinir. `raw_retention_hours` içinde bitmeyen (sırada bekleyen, duraklayan veya takılan) iş başarısız sayılır (`raw_expired`) ve dosyası dakikalık süpürmede silinir; dosya yeniden yüklenir. Birkaç gün sürecek bir yükleme için saklama süresini uzatın; ön tahmin bunu uyarır.
- **Arşiv** `archive_retention_days` sonra silinir (`0` = silinmez).
- Gerçek müşteri verisi repoya, issue'ya veya test verisine girmez; testler sahte veriyle çalışır.

## Ayarlar (`GET/PUT /v1/tenant/history/settings`)

| Alan | Varsayılan | Aralık |
|------|-----------|--------|
| `evidence_threshold` | 3 | 2–100 |
| `cluster_similarity` | 0.80 | 0.5–0.99 |
| `resolution_similarity` | 0.75 | 0.5–0.99 |
| `rejection_similarity` | 0.85 | 0.5–0.99 |
| `raw_retention_hours` | 24 | 1–720 |
| `archive_retention_days` | 0 | 0–3650 |
| `nightly_enabled` | false | `nightly_consent_analysis` gerekir |

## Bağımlılıklar

- **pgvector (CF-214):** vektörler `vector(1536)` sütunlarındadır ve kosinüs uzaklığıyla (`<=>`) karşılaştırılır.
- **Kategori taksonomisi (CF-218):** `usecase.Taxonomy` arayüzü.
- **Departman (CF-215):** `department_id` yabancı anahtarsız ve NULL olabilir (NULL = organizasyon geneli). `usecase.Directory` arayüzü departman doğrulamasını ve "departman lideri onaylar" kuralını taşır; CF-215 gelene kadar `tenant.history.review` yetkisi olan herkes (sahip, yönetici, süpervizör) onaylar.
