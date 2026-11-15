# Bilgi katmanı

Bu dosya CF-213 ile CF-220 arasındaki işlerin ortak tasarım kaynağıdır; her iş kendi bölümünü ekler. Aşağıdaki bölüm CF-215'tir ve iskeletteki "3. Kapsam modeli" bölümünün departman kısmını doldurur.

## CF-215: Departmanlar, görünürlük, widget seçimi, AI tahmini ve devir

Departman bir iş birimidir (teknik destek, satış, faturalama); ekip değildir. Ekipler bir departmana bağlanır, insanlar ekiplere. Kapsam sırası **organizasyon → marka → departman → ekip**.

### Tablolar (migration `000225_departments`)

| Tablo / sütun | Anlamı |
|---|---|
| `departments` | Organizasyon altında, isteğe bağlı `brand_id` (marka). `name` organizasyonda tekil. `widget_visible`, `sort_order`, `sla_policy_id` (UUID, `sla_policies(organization_id, uuid)`), `timezone` + `working_hours` (NULL = organizasyonun saatleri), `ai_enabled`, `ai_instructions`, `routing_hint`. |
| `teams.department_id` | Ekibin departmanı; departman silinince NULL. |
| `knowledge_articles.department_id`, `knowledge_sources.department_id`, `business_context_sections.department_id`, `knowledge_suggestions.department_id` | NULL = organizasyon geneli. Departman bilgi sahibiyken silinemez (yabancı anahtar NO ACTION); kullanım durumu bilgiyi önce organizasyon geneline ya da başka departmana taşır. Öneri, konuşmanın departmanını tetikleyiciyle devralır. |
| `conversations.department_id`, `department_source` (`customer`, `ai`, `agent`, `rule`), `department_confidence`, `department_routed_at` | Konuşmanın departmanı, kimin belirlediği, AI güven puanı ve yönlendirmenin bir kez yapıldığı an. |
| `department_knowledge_shares` | Bir departmanın tek bir bilgisinin (`article`, `source`, `context`) başka bir departmanla paylaşımı. |
| `conversation_department_transfers` | Departman değişiklikleri: kimden, kime, kim (`customer`, `ai`, `agent`), güven puanı, neden ve devralan ekibe bağlam özeti. |

**Sütun tipi.** `department_id` her tabloda `BIGINT` ve `(organization_id, department_id) → departments(organization_id, id)` bileşik yabancı anahtarıdır (ekipler ve konuşma ekibiyle aynı kalıp). Paralel dallarda NULL olarak eklenen `department_id` sütunları bu tipe uyar ve birleşmeden sonra aynı yabancı anahtarı alır; `UUID` tipiyle eklenmiş olanlar (`000225_pgvector`) `BIGINT`'e çevrilir.

### Görünürlük kuralı

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

pgvector deposu (CF-214) ve bilgi birimleri (CF-216) aynı kuralı kullanır: paylaşımı olmayan tablolar için `department_id IS NULL OR department_id = ANY(viewer_departments)`, paylaşılan türler için `department_visible(...)`. Kapsam `deptscope.Effective(ctx).Args()` ile okunur.

### Departman belirleme

1. **Widget.** Bootstrap `departments` listesini döner (widget'ta görünür, markası widget'ın markası ya da boş). Müşteri yeni konuşmada seçerse ilk mesaj `department_uuid` taşır; konuşma `customer` kaynağıyla o departmana girer. Seçim isteğe bağlıdır ("Emin değilim").
2. **AI tahmini.** Müşteri seçmezse AI ajanı ilk müşteri mesajında bir kez (`department_routed_at`) departmanı tahmin eder: adaylar departman adı, açıklaması ve `routing_hint` (güvenilmeyen etiket olarak) ile verilir, çıktı yapılandırılmış şemadır (`department_uuid` ya da null, `confidence`, `reason`), çağrı `triage` görevi olarak ölçülür. Güven **0,6** (`PredictThreshold`) ve üstüyse konuşma `ai` kaynağıyla departmana girer; altındaysa organizasyon geneli kalır ve temsilci devreder. Bilinmeyen bir uuid yok sayılır.
3. **Devir.** Temsilci (`tenant.conversations.assign`) `POST /v1/tenant/conversations/{uuid}/department-transfer` ile, AI ise cevap sırasında `transfer_department` aracıyla konuşmayı başka departmana devreder. Neden ve bağlam özeti zaman çizelgesine (`department_transferred` olayı) ve devir geçmişine yazılır; başka departmanın ekibi atanmışsa ekip ataması kalkar, istenirse temsilci de bırakılır. SLA saatleri hemen yeniden hesaplanır.

### Departman ayarlarının etkisi

- **SLA:** konuşmanın departmanının politikası marka ve şirket politikasından önce gelir.
- **Çalışma saatleri:** kendi saati olmayan ekip departmanın saatini devralır; ekipsiz konuşma departmanın saatini kullanır.
- **AI:** `ai_enabled = false` departmanda AI cevap yazmaz, konuşmayı temsilciye devreder; `ai_instructions` cevap talimatlarına eklenir.

### API ve izinler

`/v1/tenant/departments` (liste, oluştur, getir, güncelle, sil `?move_to=none|<uuid>`), `/v1/tenant/departments/{uuid}/shares`, `/v1/tenant/conversations/{uuid}/department`, `/v1/tenant/conversations/{uuid}/department-transfer`. İzinler: `tenant.departments.read` (tüm roller), `tenant.departments.write` ve `tenant.departments.read_all` (sahip, yönetici).
