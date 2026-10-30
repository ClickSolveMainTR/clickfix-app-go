# Platform ticareti: sektör karşılaştırması ve ClickFix eksikleri

> Araştırma notu (2026-10-29). Sektör bilgileri resmi dokümanlardan doğrulandı, linkler [Kaynaklar](#5-kaynaklar) bölümünde. Doğrulanamayan iddialar "(doğrulanmadı)" ile işaretlendi. ClickFix bölümleri kodun bu tarihteki halinden (`main`) okundu.
>
> Bölüm 3 ve 4, CF-143 öncesi `main` durumunu anlatır. CF-143 ile kapatılan maddeler [Bölüm 6](#6-cf-143-sonucu-simülasyon-ve-durum)'da.

## 1. Özet

- ClickFix'in faturalama çekirdeği güçlü: değişmez plan sürümleri, append-only kredi defteri (FIFO lot + son kullanma), proratalı plan/eklenti değişikliği, kupon, iade, iade notu (credit note), e-fatura ve dunning (1/3/5/7 gün + 14 gün grace) var.
- Kredi tarafında AI kredisi ve video dakikası aynı cüzdan (`billing_credit_ledger`) üzerinden yürüyor. Plan hakkı her dönem sonunda yanıyor, satın alınan paketler varsayılan olarak hiç yanmıyor.
- En büyük boşluklar ürün yüzeyinde: tenant tarafında cüzdan/kredi satın alma ekranı yalnızca video sayfasında var. Otomatik yükleme (auto-recharge) için API var ama arayüz yok ve kayıtlı kart şart. AI kredisi için düşük bakiye uyarısı yok.
- Admin kredi düzeltmesi var ama gerekçe serbest metin, defter satırında işlemi yapanın kim olduğu tutulmuyor, aktivite kaydına tutar ve gerekçe yazılmıyor.
- İadede kredi geri alma (claw-back) `ai_credits` birimine sabitlenmiş. Video dakikası paketi iadesi kod okumasına göre çakışma hatası veriyor.
- Plan geçişi (migrate subscribers) yalnızca API'de var: en fazla 100 aboneliği tek tek seçmek gerekiyor, arayüzü ve müşteriye bildirimi yok.
- CF-124: yayınlanmış sürüme yeni özellik eklemek için migration'lar değişmezlik trigger'ını kapatıyor. Bu iş için resmi bir mekanizma gerekiyor.
- Kayıt modu iki yere bölünmüş (`TENANCY_SELF_SERVE` env + `auth_settings.registration_enabled`). Davetle kayıt, bekleme listesi ve alan adı kısıtı yok.

## 2. Kavramlar ve sektör karşılaştırması

Kısaltmalar: **Stripe** = Stripe Billing, **CB** = Chargebee, **KB** = Kill Bill, **OM** = OpenMeter.

### 2.1 Planlar, plan sürümleri, grandfathering, plan geçişi

| Sistem | Değişmezlik | Mevcut aboneler | Geçiş aracı |
|---|---|---|---|
| Stripe | Price tutarı API'den değiştirilemez. Yeni Price açılır, eskisi arşivlenir. | Arşivlenen Price'taki abonelikler iptal edilene kadar devam eder. | Abonelik item'ını yeni Price'a güncellemek; toplu geçiş için subscription schedule (dönem sonunda yeni faz). |
| Chargebee | Plan fiyatı güncellenince yalnızca yeni aboneliklere uygulanır (grandfathering). | Eski fiyatla yenilenmeye devam eder. | Tekil "Edit Subscription" veya toplu "Update Subscription for Items". İkisi de price override açık olmasını gerektirir. |
| Kill Bill | Katalog, `effectiveDate` ile sürümlenen XML dosyalarıdır. Her yeni sürüm bir öncekinin yerini alır. | Mevcut abonelikler eski fiyatla devam eder. `effectiveDateForExistingSubscriptions` ile mevcut abonelere hangi tarihte geçileceği belirlenir. | Plan değişim kuralları: `changePolicy` = `END_OF_TERM` / `IMMEDIATE` / `ILLEGAL`. Kural yoksa varsayılan kullanılır. |
| Lago | Plan + abonelik başına `plan_overrides`. | Override edilmiş aboneliklere ana plandaki değişiklik yansımaz. | Abonelik güncelleme API'si. |
| Paddle | (doğrulanmadı) | - | - |

### 2.2 Deneme süresi (trial)

| Sistem | Kartsız deneme | Deneme sonu |
|---|---|---|
| Stripe | Var. `trial_settings.end_behavior.missing_payment_method` = `create_invoice` (varsayılan, ödenmezse `past_due`) / `cancel` / `pause`. | `pause` seçilirse ödeme yöntemi eklenince abonelik devam ettirilir. |
| Chargebee | Var. Varsayılan: kart yoksa veya ödeme başarısızsa `canceled`. | "End of trial action" ayarı (site/plan/abonelik düzeyinde): Activate veya Cancel. Beta, destek ekibinden açtırılıyor. |
| Paddle | `requires_payment_method=false` olan price ile, yalnızca API üzerinden. | Kart yoksa otomatik iptal. Kartsız deneme duraklatılamaz. |
| Lago | Plan override ile deneme süresi abonelik başına değiştirilebilir. | (doğrulanmadı) |

### 2.3 Eklentiler (add-on)

- **Stripe/Chargebee**: aboneliğe ikinci bir item veya addon eklenir, proratasyon varsayılan olarak açıktır (Stripe).
- **Zendesk**: Copilot ve benzeri eklentiler ajan başına satılır. Otomatik çözüm (automated resolution) ek alımı aboneliğe tekrarlayan sipariş olarak eklenir.
- **Freshdesk**: Freddy AI Agent oturum paketleri (aşağıda) eklenti olarak satılır.

### 2.4 Ön ödemeli kredi paketleri / kullanım kredileri

| Sistem | Model | Son kullanma / devir (rollover) | Öncelik |
|---|---|---|---|
| Stripe Billing Credits | Credit grant: `category` = `paid` / `promotional` (verilmezse `paid`). `applicability_config` ile yalnızca metered price'lara uygulanır. | `expires_at` verilmezse yanmaz. Grant hemen expire veya void edilebilir. | `priority` 0-100 (varsayılan 50, preview). |
| Lago | Cüzdan (wallet) + ödenmiş/hediye kredi. | Son kullanma tarihi yoksa bakiye sıfırlanana kadar dönemden döneme devreder. Tarih gelince kalan bakiye void edilir. | (doğrulanmadı) |
| OpenMeter | Entitlement'a bağlı grant'ler. | Her grant'in expiration'ı zorunludur. `minRolloverAmount` / `maxRolloverAmount` ile reset anında devir hesaplanır. | `priority` 0-255. Eşitlikte önce süresi en yakın olan, sonra en eski grant tüketilir. |
| Chargebee | Adjustment, refundable ve promotional kredi. Promotional kredi nakde çevrilmez. | (doğrulanmadı) | - |
| Intercom Fin | Sonuç başına ücret: çözüm başına $0,99, konuşma başına en fazla bir kez. Müşterinin istediği insan devri ücretsiz, "assumed resolution" ücretli. | - | - |
| Zendesk | Plana ve koltuk sayısına göre dahil gelen çözüm hakkı. Admin Center'dan bakiye eklenir. | Eski modelde otomatik çözümler sonraki döneme devretmez. Hak iade edilemez, nakit değeri yoktur. | - |
| Freshdesk | Freddy AI Agent oturum paketi: eski modelde 1.000 oturum $100. 13 Kasım 2025'ten sonraki ilk alımlarda 100 oturum $49. | Paket mevcut döneme eklenir, sonraki dönemde yenilenir. Devir yok iddiası (doğrulanmadı). | - |

### 2.5 Cüzdan, yükleme (top-up), otomatik yükleme, düşük bakiye uyarısı

| Sistem | Otomatik yükleme | Düşük bakiye uyarısı |
|---|---|---|
| Lago | Tekrarlayan yükleme: fixed/target × interval/threshold. Eşik altına düşünce yükleme başlar. Premium özellik. İade sonrası eşik kuralının yeniden tetiklenip tekrar tahsilat yapabileceği uyarısı var. | Cüzdan uyarıları: `wallet_balance_amount`, `wallet_ongoing_balance_amount/credits`. 20'ye kadar azalan eşik, `alert.triggered/resolved` olayları. |
| Stripe | Billing'de yerleşik otomatik yükleme (doğrulanmadı). Metronome entegrasyonu eşik altı auto-recharge sunuyor. | Billing alert tipi yalnızca `usage_threshold` (metre üstü). Kredi bakiyesi için hazır uyarı bulunamadı. `billing.credit_balance_transaction.created` webhook'u ile uygulama tarafında kurulabilir. |
| OpenMeter | - | Entitlement bildirimleri yüzde eşikleriyle çalışır (ör. %200 soft limit aşımı). |

### 2.6 Aşım (overage): hard / soft limit, kullandıkça öde

- **OpenMeter**: `isSoftLimit=false` olduğunda bakiye 0'da erişim kesilir. Soft limitte erişim devam eder, `overage` raporlanır. `preserveOverageAtReset` ile aşım sonraki dönemden düşülür.
- **Zendesk**: dahil hak bitince çözüm başına ücretlendirilir. Admin "AI ajanını durdur" ya da "devam et ve aşım öde" seçebilir.
- **Lago**: "ongoing balance" eksiye düşebilir, yani kullanım faturalanana kadar bakiye negatif görünür.

### 2.7 Faturalar, iade notları, iadeler ve kredi geri alma

- **Chargebee**: refundable kredi ödenmiş faturadan doğar. Saklanabilir ya da ödeme yöntemine iade edilir. Credit note'larda yapılandırılabilir reason code zorunludur.
- **Lago**: ön ödemeli kredi faturasında ödeme kaydı varsa kullanılabilir cüzdan bakiyesi iade edilir ve krediler anında void edilir. Credit note "credit" ve "refund" kısımlarına ayrılır.
- **Paddle**: refund ve credit "adjustment" kaydıdır, oluşturulduktan sonra değiştirilemez. Credit onay gerektirmez, refund çoğunlukla Paddle incelemesinden geçer.

### 2.8 Kupon ve indirimler

- **Stripe**: coupon + promotion code (ayrıntılar resmi sayfada).
- **Chargebee**: kuponlar plan veya addon'a sınırlandırılabilir (doğrulanmadı).

### 2.9 Manuel kredi düzeltmesi, gerekçe ve denetim izi

- **Chargebee**: credit note oluştururken listeden seçilen reason code zorunlu.
- **Paddle**: adjustment kayıtları değiştirilemez ve raporlanabilir.
- **Stripe**: müşteri bakiyesi değiştirilemez bir defterdir (customer balance transactions).
- Ortak desen: değiştirilemez kayıt + kategori (reason code) + işlemi yapan + serbest not.

### 2.10 Organizasyon açma: satış odaklı (admin) ve self-serve; kayıt modları

- Stripe, Chargebee ve Lago'da abonelik API'den, panelden veya quote/checkout akışından açılır. Satış odaklı akışta özel fiyat için override (Lago `plan_overrides`, Chargebee price override) kullanılır.
- Kayıt modu (açık / davetle / kapalı / bekleme listesi) faturalama ürünlerinin değil, uygulamanın sorumluluğundadır. Karşılaştırılan sistemlerde bu konuda bir standart yok.

### 2.11 Askıya alma, yeniden etkinleştirme, dunning

- **Stripe**: varsayılan deneme sonu davranışında ödenmeyen fatura aboneliği `past_due` durumuna geçirir. `pause` sonrası resume edilir.
- **Paddle**: `past_due` aboneliklerde ödeme kurtarma ayarlarına göre yeniden denenir. Denemeler tükenince abonelik `canceled` olur.
- **Chargebee**: otomatik tahsilat açıkken başarısız ödeme dunning sürecini başlatır. Tahsil edilemeyen tutar adjustment credit note'a döner.

## 3. ClickFix'te bugün olan

Kod yolları `backend/internal/` altındadır.

| Konu | Uygulama (dosya / fonksiyon / tablo) |
|---|---|
| Plan ve sürüm | `billing_plans`, `billing_plan_versions`, `billing_prices`, `billing_price_tiers`, `billing_plan_entitlements` (000031). `billing_guard_version()` trigger'ı yayınlanmış sürümde UPDATE/DELETE ve alt tablolarda INSERT/UPDATE/DELETE işlemlerini `BILLING_VERSION_IMMUTABLE` ile reddeder. `usecase/catalog.go`: `SaveVersion` yalnızca draft'ı düzenler. `Publish` monotondur: eski bir draft, daha yeni yayınlanmış sürümün önüne geçemez. |
| Grandfathering | Abonelik `plan_version_id` + `price_id` tutar. Yeni sürüm yayınlamak mevcut aboneleri etkilemez. |
| Plan geçişi | `catalog.go` `ScheduleMigration` → `POST /v1/platform/plan-migrations`. 1-100 abonelik/org UUID çifti alır, gerekçe zorunludur. Yalnızca aynı plan içinde, eşleşen interval/para birimiyle çalışır. `pending_plan_version_id` yazar. Geçiş bir sonraki yenilemede `applyTransition` içinde uygulanır. Frontend'de çağıran yok, müşteriye bildirim yok (`SubscriptionPlanScheduled` bildirim kayıtlarında değil). |
| CF-124 olayı | `000194_multibrand` ve `000198_appointments`, yeni özelliğin hakkını tüm sürümlere INSERT ediyordu. Trigger reddetti ve canlıda migrate kirli kaldı. Düzeltme (a005d5c): INSERT süresince `ALTER TABLE billing_plan_entitlements DISABLE TRIGGER trg_billing_entitlement_guard`. |
| Plan içi müşteriye özel hak | `billing_entitlement_overrides` + `PUT /v1/platform/organizations/{org}/entitlements/{feature}` (step-up). Fiyat override'ı yok. |
| Deneme | `usecase/trials.go` `startTrial`: org başına tek deneme (`BillingTrialEnrolled`), kart gerekmez. Fiyat seçilmezse `gateway_settings.trial_price_uuid` ya da varsayılan deneme fiyatı kullanılır. Denemesi olmayan plan seçilirse 72 saatlik `pending_payment` penceresi açılır (CF-42). Hatırlatmalar 7/3/1 gün önce (`billing.trial_reminder` → e-posta + uygulama içi). `ExtendTrial` admin + gerekçe ister. |
| Deneme → ücretli | Deneme sonunda `AutoPay` + kayıtlı kart varsa `ChargeTrial` çalışır, başarılıysa `active`. Yoksa `Advance` ile `expired` (terminal). Deneme sırasında `POST /v1/tenant/billing/subscriptions/{id}/trial-invoice` ile ödenebilir. |
| Eklentiler | `usecase/addons.go`: `billing_addons` (özellik anahtarı + birim + fiyat). Artış dönem sonuna kadar proratalı faturalanır. Azaltma `pending_quantity` ile dönem sonuna planlanır. Eklentinin para birimi abonelik para birimiyle aynı olmalı. |
| Kredi birimleri | `billing_credit_ledger.unit` ∈ `TRY`, `ai_credits`, `video_minutes`. Defter append-only (`billing_guard_append_only`). Tüketim `billing_credit_allocations` ile lot bazında FIFO (önce son kullanma tarihi en yakın olan). |
| Plan hakkı | `usecase/plan_credits.go` `grantPlanCredits`: `ai.credits` ve `video.minutes` limitleri her dönem için `PeriodEnd`'de yanan lot olarak verilir. Yükseltmede fark tamamlanır, harcanan kredi geri gelmez. `recordTransition` her geçişte çağırır. Geriye dönük doldurma için `cmd/backfill-plan-credits` var. |
| Kredi paketleri | `billing_credit_packages` (000046, `unit` 000199'da eklendi): kod, ad, kredi, `amount_minor`, `expires_days` (boş olabilir, yani hiç yanmaz), `active`. API'de yalnızca oluşturma ve listeleme var, pasifleştirme/düzenleme yok. Paketler global: plan bazlı görünürlük ve para birimi yok, `PurchaseCredits` faturayı `TRY` keser. Ödeme yapılınca `settleCommerce` (`plan_changes.go`) lotu `purchase:<invoice>` anahtarıyla yazar. |
| AI kredisi tüketimi | `aicore/repository/repository.go`: `ai_wallet_reserve` ile çalıştırmadan önce rezervasyon, `Finish` içinde kullanılmayan kısım `ai_wallet_release` ile geri bırakılır. Bakiye yetmezse `ErrCredits` döner, `WalletSummary.AIAction = stop_and_handoff` (insana devir). |
| Video dakikası | `videorooms/metering.go` `bill`: görüşme kapanınca `MeterUsage` ile en fazla mevcut bakiye kadar düşülür, kalan kısım `unbilled` olarak kaydedilir. `outOfMinutes` bakiye bitince canlı odaları kapatır. `checkLowBalance` org eşiğinin altında günde bir kez sahiplere bildirim gönderir (`video.balance_low`). |
| Onboarding hakkı | `onboarding/service.go` `CreateOrganization`: self-serve org'a 30 gün geçerli tek seferlik AI kredisi verir (`OnboardingAICredits`). |
| Otomatik yükleme | `wallet.go` `SaveWalletSettings` → `PUT /v1/tenant/billing/wallet/settings`, tablo `billing_wallet_settings`. Açık rıza zorunlu (`consented_at`), `sub.AutoPay` + `PaymentMethodID` zorunlu. `WalletSweep` eşik altındaki org'lar için paket faturası açar ve kayıtlı karttan çeker (`auto:<tarih>` anahtarı, yani günde en fazla bir kez). Frontend'de bu uç noktayı çağıran yok. |
| Düşük bakiye | Video için var (yukarıda). AI kredisi için cüzdan bakiyesine bağlı uyarı yok. Var olan `EntitlementsQuotaWarning` dönem kullanımını plan limitiyle karşılaştırır, satın alınan paketleri hesaba katmaz. `billing.wallet_changed` olayına bildirim aboneliği yok. |
| Aşım | Entitlement'larda `enforcement` = `hard`/`soft` + `warn_percent` (000031). Kademeli kullanım fiyatları dönem sonunda faturalanır (`invoicing.go` `periodInvoice`). Cüzdan birimleri (AI, video) her zaman hard stop: borç yok, kullandıkça öde yok. |
| Faturalar | Numaralı fatura ve satırlar değiştirilemez (000046 trigger'ları), PDF/UBL e-fatura (`ubl/`, `einvoice_provider.go`), TCMB kuru (`profiles_fx.go`). |
| İade ve credit note | `credit_notes.go` `localRefund`: cüzdana (TRY) veya manuel banka iadesi (kanıt referansı zorunlu). Credit note orijinal snapshot ve KDV ile kesilir. Admin uç noktası step-up ister. |
| Kredi geri alma | `refund_credits.go` `reserveRefundCredits`: iade oranında, yalnızca ilgili satın almanın kullanılmamış lotlarından düşer. Yetmezse `ErrConflict` döner (harcanmış kredi otomatik iade edilmez). Sağlayıcı reddederse `releaseRefundCredits` geri yükler. Lot sorgusu ve defter kaydı `"ai_credits"` sabitine yazılmış. |
| Kuponlar | `coupons.go`: yüzde veya tutar, geçerlilik aralığı, `max_redemptions`, `per_org_limit`, `duration_months`, `recurring`, plan ve interval kısıtı. Tenant `POST /v1/tenant/billing/coupon` ile uygular. Referans (referral) programı da var (`referrals.go`). |
| Manuel kredi düzeltmesi | `POST /v1/platform/organizations/{org}/billing/wallet/entries` (`WalletCommand`, step-up). Arayüz: `organization-wallet-tab.tsx`. Birim + işaretli tutar + serbest metin gerekçe (≤500) + opsiyonel son kullanma. Defter satırında aktör kolonu yok. `CommerceAudit` aktivite kaydına yalnızca `settings.updated` + route + org UUID yazar, tutar/birim/gerekçe yazmaz. `billing.wallet_changed` olayında `WithActor` yok. |
| Admin ile org açma | `organizations/handler/handler.go`: `owner_email` + `trial_price_uuid` → `CreateOrganizationWithOwnerInvitation` (sahip daveti). Deneme `TrialHooks.OnCreatedWithTrial` ile aynı transaction'da başlar. Ayrıca `POST /v1/platform/subscriptions` (`lifecycle.go` `CreateSubscription`) ile `pending_payment`/`trialing`/`active` ve serbest dönem sınırlarıyla abonelik açılabilir. |
| Self-serve | `onboarding/service.go`: `TENANCY_SELF_SERVE` (env, varsayılan `false`) kapalıysa `SELF_SERVE_DISABLED`. Kullanıcı başına sahip olunabilecek org sınırı ve fiyat beyaz listesi (`SelfServeSignupPriceAllowed`) var. |
| Hesap kaydı | `authsettings` `registration_enabled` (+ `password_register_enabled`): e-posta, OAuth ve native OAuth kayıtlarında kontrol edilir. Davet akışı ayrı (`InviteAccounts`). Bekleme listesi, alan adı kısıtı veya "yalnızca davetle" modu yok. |
| Yaşam döngüsü | `model/lifecycle.go` `LegalTransitions`: `pending_payment`, `trialing`, `active`, `past_due`, `grace`, `suspended`, `paused`, `canceled`/`expired` (terminal). `CanWrite` yalnızca active/trialing/past_due/grace durumlarında yazmaya izin verir. Org düzeyinde ayrıca `SetStatus` ve erişim penceresi var. |
| Dunning | `renewals.go`: ilk başarısızlıktan sonra 1/3/5/7. günlerde tekrar deneme (`RetryAt`), `past_due` → `grace` (14 gün) → `suspended`. Havalede günlük hatırlatma. Bildirimler: `billing.payment_required`, `subscription.suspended`. Yenileme hatırlatması 30/7 gün önce. `suspended` → `active` ödeme ile döner. |

## 4. Eksikler ve kararlar

Öncelik: **P0** = para/veri doğruluğu veya canlı riski, **P1** = satış/kullanım için gerekli, **P2** = iyileştirme.

**G1 - Video dakikası iadesinde kredi geri alma hatalı (P0)**
- Eksik: `reserveRefundCredits` ve `releaseRefundCredits` lot sorgusunda ve defter kaydında `"ai_credits"` sabitini kullanıyor. `video_minutes` paketi iade edilince `available` 0 kalıyor ve `ErrConflict` dönüyor. Kod okumasına göre iade hiç yapılamıyor. Eşleşme olsaydı bile yanlış birimden düşülürdü.
- Neden önemli: para iadesi ile kredi tutarlılığı bozulur, destek ekibi iadeyi yapamaz.
- Öneri: birimi `purchase.Unit`'ten al. `billing_refund_credit_reservations` tablosuna `unit` kolonu ekle (migration). Video paketi için entegrasyon testi yaz (`plan_credits_integration_test.go` örüntüsüyle).

**G2 - Yayınlanmış sürümlere yeni özellik ekleme mekanizması yok (P0)**
- Eksik: CF-124'te 000194 ve 000198 değişmezlik trigger'ını `DISABLE TRIGGER` ile kapattı. Her yeni özellik migration'ı aynı kalıbı kopyalamak zorunda. Unutulursa canlıda migrate kirli kalır ve backend açılmaz.
- Neden önemli: hem operasyonel risk hem de "yayınlanan sürüm değişmez" garantisinin sessizce delinmesi.
- Öneri: `billing_features` tablosuna `default_for_existing_versions` (değer + kind) ekle ve çözümlemede (`entitlements.Resolve`) sürümde satır yoksa bu varsayılanı kullan. Böylece eski sürümlere satır yazılmaz. Alternatif: tek bir `SECURITY DEFINER` fonksiyon `billing_backfill_feature(key, value)`, trigger'ı yalnızca bu fonksiyonun bypass ettiği bir session değişkeniyle (`SET LOCAL billing.backfill = on`) çalışır. `db-migration` skill'ine kural olarak yazılmalı.

**G3 - Tenant tarafında cüzdan ve kredi satın alma yüzeyi eksik (P1)**
- Eksik: `GET /v1/tenant/billing/wallet` frontend'de kullanılmıyor ve yanıtta `video_minutes` alanı da yok (`handler/commerce.go`). Kredi paketi satın alma yalnızca video ayarları sayfasında var, AI kredi paketi satın alma ekranı yok.
- Neden önemli: Fin, Zendesk ve Freshdesk'te AI hakkı faturalama sayfasında görünür ve satın alınabilir. ClickFix'te AI bitince `stop_and_handoff` oluyor ama müşteri nereden kredi alacağını göremiyor.
- Öneri: Faturalama portalına "Krediler" bölümü ekle: birim başına bakiye, lot listesi (kaynak, son kullanma), paket satın alma, otomatik yükleme ayarı. Wallet yanıtına `video_minutes` ekle (OpenAPI ve `pnpm api:generate`).

**G4 - Otomatik yükleme kayıtlı kart şartına bağlı, arayüzü yok (P1)**
- Eksik: `SaveWalletSettings` `sub.AutoPay && PaymentMethodID` istiyor. Yalnızca havaleyle ödeyen tenant'lar otomatik yüklemeyi açamıyor. Ayarın arayüzü de yok.
- Neden önemli: Türkiye'de B2B müşterilerin önemli kısmı havale/EFT ile ödüyor.
- Öneri: iki mod: (a) kart: bugünkü davranış; (b) havale: eşik altına düşünce `wallet` faturası aç + `billing.payment_required` bildirimi gönder (tahsilat yok, açık fatura varken yeni fatura açma, `BillingUnsettledWalletInvoice` zaten var). Rıza metnini ve tarihini UI'da göster. Lago'nun "iade sonrası eşik yeniden tetiklenir" uyarısı nedeniyle iade sırasında otomatik yüklemeyi atla.

**G5 - AI kredisi için düşük bakiye uyarısı yok (P1)**
- Eksik: Video için `checkLowBalance` var. AI kredisinde cüzdan bakiyesine bağlı uyarı yok. `EntitlementsQuotaWarning` dönem kullanımını plan limitiyle kıyaslıyor. Paket alan müşteri bakiyesi dururken %100 ("quota_exhausted") bildirimi alabilir (kod okumasına göre, testle doğrulanmalı).
- Neden önemli: AI bitince insana devir başlıyor ve müşteri bunu bildirim olmadan fark ediyor.
- Öneri: birimden bağımsız tek mekanizma: `billing_wallet_settings` içine birim başına `low_balance_threshold`, `billing.wallet_changed` tüketicisinde eşik geçişi (bir kez, bakiye eşiğin üstüne çıkınca sıfırlanır, video'daki `VideoMarkLowBalanceNotified` örüntüsü). Video'nun kendi kontrolü buna taşınabilir. `ai.credits` için kota uyarısı cüzdan bakiyesine göre hesaplanmalı ya da kapatılmalı.

**G6 - Manuel kredi düzeltmesinde gerekçe kategorisi ve aktör izi yok (P1)**
- Eksik: Gerekçe serbest metin. `billing_credit_ledger` satırında `actor_id` yok. Aktivite kaydı tutar, birim ve gerekçe içermiyor. Olayda `WithActor` yok.
- Neden önemli: denetim ve mali müşavir sorusu "bu krediyi kim, neden verdi" tek sorguda cevaplanamıyor. Chargebee reason code zorunlu tutuyor.
- Öneri: migration ile `actor_id` + `reason_code` kolonları ekle. Kategoriler: `goodwill`, `sla_credit`, `migration`, `correction`, `promo`, `sales_deal`, `clawback`. UI'da kod seçimi + not. Aktivite payload'ına unit/delta/reason_code ekle, olaya `WithActor` koy. Belirli bir eşiğin üstü için ikinci onay seçeneği (P2).

**G7 - Kredi paketlerinin yönetimi zayıf: plan görünürlüğü, pasifleştirme, para birimi (P1)**
- Eksik: Paketler global. Plana veya segmente göre gösterme yok. `active` kolonu var ama API'de pasifleştirme/düzenleme yok. Para birimi yok, fatura hep `TRY` kesiliyor (USD/EUR aboneler için tutarsız).
- Neden önemli: kurumsal plana özel indirimli paket ve fiyat güncellemesi yapılamıyor. Hatalı bir paket kaldırılamıyor.
- Öneri: `billing_credit_package_plans` (paket ↔ plan, boşsa herkese) ekle, `currency` kolonu ekle, `PATCH .../credit-packages/{uuid}` ile `active` değiştir. Paketler de sürüm mantığıyla ele alınmalı: satın alma `credits/expires_days` değerlerini zaten kopyalıyor, o yüzden düzenleme yerine "pasifleştir + yenisini aç" yeterli.

**G8 - Kredi paketlerinde son kullanma varsayılanı yok (P2)**
- Eksik: `expires_days` boşsa satın alınan kredi hiç yanmıyor. Plan hakkı dönem sonunda, onboarding hakkı 30 günde yanıyor.
- Neden önemli: süresiz yükümlülük (muhasebede ertelenmiş gelir) ve fiyat değişikliklerinde eski ucuz kredilerin birikmesi. Stripe ve Lago'da da varsayılan süresiz, OpenMeter ise süre zorunlu tutuyor.
- Öneri: oluşturma formunda varsayılanı 365 gün yap, boş bırakmak bilinçli bir seçim olsun. Satış koşullarına "kullanılmayan kredi iade edilmez/süresi dolar" maddesi ekle (hukuk onayı).

**G9 - Plan geçişi için kullanılabilir bir akış yok (P1)**
- Eksik: `ScheduleMigration` en fazla 100 UUID çiftini elle istiyor. "X sürümündeki tüm aboneleri Y'ye taşı" seçimi, önizleme, arayüz ve müşteriye bildirim yok. Yalnızca yenilemede uygulanıyor, anında veya belirli tarihte geçiş yok.
- Neden önemli: fiyat artışı ya da yeni sürüme geçiş operasyonu yapılamıyor. Kill Bill `effectiveDateForExistingSubscriptions` ile, Stripe schedule ile bunu çözüyor.
- Öneri: `POST /v1/platform/plans/{uuid}/versions/{version}/migrate` (kaynak sürüm filtresi, hedef sürüm, `at` = `renewal` | tarih, dry-run sayımı). İş `bulkengine` job'ı olarak sayfa sayfa yürütülsün. Müşteriye en az 30 gün önce bildirim gitsin (`billing.plan_change_scheduled` şablonu). Plan editöründe "Aboneleri taşı" düğmesi olsun.

**G10 - Kayıt modu iki yere bölünmüş, davet/bekleme listesi yok (P1)**
- Eksik: Org açma `TENANCY_SELF_SERVE` env değişkeninde (yeniden deploy gerektirir), hesap kaydı `auth_settings.registration_enabled` içinde (panelden). "Yalnızca davetle", bekleme listesi ve alan adı beyaz listesi yok.
- Neden önemli: lansman öncesi ve kapalı beta dönemlerinde env değiştirip deploy etmek gerekiyor. İki bayrağın kombinasyonu kafa karıştırıyor (kayıt açık + self-serve kapalı = hesap açılıyor ama org açılamıyor).
- Öneri: `auth_settings` içinde tek `signup_mode` = `open` | `invite_only` | `waitlist` | `closed` + opsiyonel `allowed_email_domains`. Env yalnızca üst sınır olarak kalsın (`TENANCY_SELF_SERVE=false` ise panelden açılamasın). `waitlist` modunda başvuru tablosu + platform onayı → onaylanınca `CreateOrganizationWithOwnerInvitation`. Kayıt kapalıyken davetlinin hesap açıp açamadığı ayrıca doğrulanmalı (doğrulanmadı).

**G11 - Satış odaklı müşteriye özel fiyat yok (P2)**
- Eksik: Org bazında yalnızca entitlement override var. Fiyat override'ı (Lago `plan_overrides`, Chargebee price override) yok. Pazarlıklı fiyat ancak kupon veya ayrı gizli plan ile yapılabiliyor.
- Öneri: kısa vadede org'a özel "private" plan sürümü (katalogda `public=false`) + `CreateSubscription`. Uzun vadede abonelik düzeyinde `price_override_minor`.

**G12 - Deneme sonu davranışı yapılandırılamıyor (P2)**
- Eksik: Kart yoksa deneme `expired` (terminal) oluyor. Havaleyle ödeyecek müşteri için `pending_payment` penceresi ya da `paused` seçeneği yok. Stripe (`pause`/`create_invoice`) ve Chargebee (Activate/Cancel) bunu ayar olarak sunuyor.
- Öneri: plan sürümünde `trial_end_action` = `expire` | `invoice` (deneme sonunda fatura + N günlük `pending_payment`/grace). Varsayılan bugünkü davranış kalsın.

**G13 - Cüzdan birimlerinde kullandıkça öde (aşım) seçeneği yok (P2)**
- Eksik: AI ve video bakiye bitince sert duruyor. Zendesk'te "devam et ve aşımı faturala" seçeneği var.
- Öneri: ürün kararı. İstenirse org ayarı `overage_mode` = `stop` | `postpaid` (aylık tavanlı), aşım dönem faturasına satır olarak eklenir. Bu, "no debt" kuralının bilinçli olarak gevşetilmesi demek.

> Kontrol edildi, eksik değil: kupon (plan/interval kısıtı, kullanım limitleri), dunning (retry + grace + suspend + bildirim), iade notu ve e-fatura, deneme hatırlatmaları, admin ile org açma + sahip daveti + deneme fiyatı, video için düşük bakiye uyarısı.

## 5. Kaynaklar

Not: bu oturumda resmi doküman alan adlarına doğrudan erişim (WebFetch) kapalıydı. Bilgiler aşağıdaki resmi sayfaların arama özetlerinden doğrulandı.

**Stripe**
- Billing credits: https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits
- Credit Grant API: https://docs.stripe.com/api/billing/credit-grant
- Credit grant category opsiyonel: https://docs.stripe.com/changelog/clover/2025-10-29/credit-grant-optional-category
- Billing alerts: https://docs.stripe.com/api/billing/alert, https://docs.stripe.com/billing/subscriptions/usage-based/alerts
- Billing credit webhooks: https://docs.stripe.com/changelog/basil/2025-03-31/billing-credit-webhooks
- Customer balance: https://docs.stripe.com/invoicing/customer/balance
- Trials: https://docs.stripe.com/billing/subscriptions/trials/free-trials
- Price değişikliği: https://docs.stripe.com/billing/subscriptions/change-price, https://docs.stripe.com/products-prices/manage-prices
- Kuponlar: https://docs.stripe.com/billing/subscriptions/coupons

**Chargebee**
- Grandfathering: https://www.chargebee.com/docs/billing/2.0/kb/product-catalog/will-a-new-plan-price-affect-the-existing-subscriptions
- Trial ve deneme sonu aksiyonu: https://www.chargebee.com/docs/billing/2.0/subscriptions/trial_periods_hidden, https://www.chargebee.com/docs/billing/2.0/kb/billing/how-do-i-configure-end-of-trial-action
- Krediler: https://www.chargebee.com/docs/account-credits.html
- Credit notes API (reason code): https://apidocs.chargebee.com/docs/api/credit_notes
- Dunning: https://www.chargebee.com/docs/payments/2.0/dunning/dunning-v2

**Lago**
- Cüzdan, yükleme ve void: https://docs.getlago.com/guide/wallet-and-prepaid-credits/wallet-top-up-and-void
- Wallet ve prepaid credits: https://docs.getlago.com/guide/wallet-and-prepaid-credits
- Cüzdan uyarıları: https://getlago.com/docs/guide/alerts/credits-consumption-alerts, https://getlago.com/docs/api-reference/wallets/create-alert
- Plan override: https://getlago.com/docs/guide/subscriptions/assign-plan
- Credit notes: https://getlago.com/docs/guide/invoicing/credit-notes/overview
- İade ve eşikli yükleme uyarısı: https://knowledge.getlago.com/articles/1054289714-refunding-prepaid-credits-triggers-repeated-charges-when-a-threshold-top-up-rule-is-active

**Kill Bill**
- Subscription guide (katalog sürümleri, `changePolicy`): https://docs.killbill.io/latest/userguide_subscription
- Catalog examples: https://docs.killbill.io/latest/catalog-examples

**OpenMeter**
- Grants: https://openmeter.io/docs/billing/entitlements/grant
- Entitlement: https://openmeter.io/docs/billing/entitlements/entitlement
- Bildirimler: https://openmeter.io/docs/integrations/notifications/entitlements

**Paddle**
- Kartsız deneme: https://developer.paddle.com/build/trials/cardless-trials/
- past_due: https://developer.paddle.com/webhooks/subscriptions/subscription-past-due/
- Adjustments: https://developer.paddle.com/api-reference/adjustments/overview

**Intercom**
- Fin outcomes: https://www.intercom.com/help/en/articles/8205718-fin-ai-agent-outcomes
- Fin fiyatlandırma: https://www.intercom.com/help/en/articles/7837512-how-fin-pricing-works

**Zendesk**
- Resolution allowance: https://support.zendesk.com/hc/en-us/articles/10479587390106-Managing-resolution-allowances
- Otomatik çözümler: https://support.zendesk.com/hc/en-us/articles/8357756668186-About-automated-resolutions-for-AI-agents
- Önceki model: https://support.zendesk.com/hc/en-us/articles/6958358659226

**Freshdesk / Freshworks**
- Oturum SSS: https://crmsupport.freshworks.com/support/solutions/articles/50000004664-ai-agent-and-chatbot-sessions-faqs
- Freddy AI: https://support.freshdesk.com/support/solutions/articles/50000002692-all-things-freddy-ai-for-cx

**ClickFix içi**
- `docs/billing/ARCHITECTURE.md`, `docs/billing/PLAN-AI-CREDITS.md`, `docs/billing/METERING-TRIALS-GATEWAYS.md`
- Commit a005d5c (CF-124)

## 6. CF-143 sonucu: simülasyon ve durum

### 6.1 Simülasyon testleri

`backend/internal/modules/billing/scenario_integration_test.go`, gerçek Postgres üzerinde (`TEST_DATABASE_URL`) bir organizasyonun ticari yaşam döngüsünü baştan sona yürütür:

| Test | Adımlar |
|---|---|
| `TestCommerceScenarioPlatformAdminLifecycle` | Platform yöneticisi org açar (sahip + deneme planı) → deneme uzatılır → ödeme sağlayıcısı yokken ilk dönem havale ile ödenir ve onaylanır → sandbox sağlayıcıyla Pro'ya yükseltme (fark faturası, ödemeden önce plan değişmez) → dönem sonuna düşürme → AI paketi havale ile, dakika paketi sandbox ile alınır → AI rezervasyonu ve dakika ölçümü → gerekçeli manuel düzeltme (eksi bakiyeye izin yok) → AI paketi banka iadesi ve dakika paketi cüzdana iadesi (yalnızca harcanmamış kısım geri alınır) → org askıya alma/geri açma → dunning ile abonelik askıya alma/geri açma. |
| `TestCommerceScenarioNewFeatureAfterPublish` | Plan yayınlandıktan sonra yeni özellik eklenir: eski aboneler katalog varsayılanını alır, yayınlanmış sürüme satır eklenemez, yeni sürüm yayınlamak mevcut aboneyi değiştirmez. |
| `TestCommerceScenarioWalletAutomation` | Düşük AI bakiyesi uyarısı günde bir kez, plana kapalı paket listelenmez/alınamaz, paket arşivlenir, kart yokken otomatik yükleme reddedilir, kartla otomatik yükleme paketi alıp tahsil eder ve tekrar etmez. |
| `TestCommerceScenarioAdminPlanChange` | Satış odaklı plan değişikliği: gerekçesiz reddedilir, "hemen" faturasız geçirir ve hakkı tamamlar, "dönem sonu" yalnızca planlar, geçmişe yazılır. |
| `TestClosedSignupAdmitsOnlyPlatformInvitations` (organizations) | Kayıt kapalıyken platformun gönderdiği sahip daveti hesap açar, ekip daveti açamaz; "davetle" modunda ekip daveti çalışır. |

### 6.2 Simülasyonda bulunan ve düzeltilen hatalar

1. **Deneme uzatılınca deneme kredileri eski bitişte yanıyordu.** Plan lotları `period_end`'de yanıyor, `ExtendTrial` ise lotları taşımıyordu. Artık kalan deneme kredisi ve dakikası ters kayıt + yeni lot ile uzatılmış bitişe taşınıyor (`trials.go` `extendPlanLots`).
2. **Dakika paketi iadesi AI kredisinden düşmeye çalışıyordu (G1).** Geri alma artık satın alınan paketin birimini kullanıyor (`refund_credits.go`).
3. **Otomatik yükleme, aboneliğin yenilemesi kapalıysa ya da iptal planlanmışsa sessizce çalışmıyordu.** Cüzdan faturaları yenileme durumundan bağımsız tahsil ediliyor; ödeme altyapısı yoksa hiç fatura açılmıyor (`renewals.go`, `wallet.go`).
4. **Cüzdan yanıtı `video_minutes` alanını vermiyordu** (OpenAPI'de zorunlu yazılı olmasına rağmen). Eklendi, `settings` da döner.

### 6.3 Eksiklerin durumu

| Madde | Durum |
|---|---|
| G1 | Kapandı (yukarıda 2). |
| G2 | Kapandı: `billing_features.default_bool/default_limit/default_unlimited`. Yeni özellik migration'ı yayınlanmış sürümlere satır yazmaz, varsayılanı katalogda tanımlar (`db-migration` skill'ine kural olarak eklendi). Taslak editörü eksik özellikleri bu varsayılanla doldurur. |
| G3 | Kapandı: müşteri faturalama sayfasında bakiyeler, paket satın alma (kart veya havale; ödeme altyapısı yoksa açıklama), otomatik yükleme ve uyarı ayarı, kredi hareketleri. |
| G4 | Kısmen: otomatik yükleme bilinçli olarak kayıtlı kart ister (havalede tahsilat olmaz); arayüz bunu açıklar ve `auto_topup_available` ile düğmeyi kapatır. Havale modu (eşik altında fatura aç + hatırlat) açık iş. |
| G5 | Kapandı: `billing.wallet_low` (e-posta + uygulama içi), eşik `alert_threshold`, boşsa plan hakkının beşte biri, günde en fazla bir kez. |
| G6 | Kapandı: düzeltmelerde `category` (goodwill, correction, promotion, sales, migration, other) ve `actor_user_id` defter satırında; aktivite kaydı `billing.wallet_adjusted` tutar, birim, gerekçe ve kategoriyi yazar. |
| G7 | Kapandı (para birimi hariç): açıklama, plan görünürlüğü (`plan_ids`), sıra, düzenleme ve arşiv (`PUT /v1/platform/billing/credit-packages/{package}`); müşteri yalnızca planına açık paketleri görür ve alabilir. Paket faturası TRY kalır. |
| G8 | Arayüzde yeni paket için varsayılan 365 gün; süresiz seçim bilinçli yapılır. |
| G9 | Kısmen: plan editöründe "Aboneleri taşı" (mevcut `plan-migrations`, yenilemede uygulanır). Ayrıca platform yöneticisi tek bir organizasyonun planını hemen ya da dönem sonunda değiştirebilir (`POST /v1/platform/organizations/{organization}/subscription/change-plan`). Toplu iş ve müşteriye önceden bildirim açık iş. |
| G10 | Kapandı (bekleme listesi ve alan adı kısıtı hariç): `auth_settings.signup_mode` = `open` / `invite` / `closed`, `TENANCY_SELF_SERVE` kaldırıldı. Kapalı modda yalnızca platformun gönderdiği davetler hesap açar (`organization_invitations.platform_issued`). Platform yöneticisi her modda org açabilir. |
| G11, G12, G13 | Açık (ürün kararı gerekiyor). |

### 6.4 CMS kullanıcısı rolü

`cms_user` platform rolü kaldırıldı (migration 000207): atamaları ve varsayılan kayıt rolü temizlendi. Çalışma alanı erişimi yalnızca organizasyon üyeliğinden gelir; yeni bir organizasyonu açan hesap o organizasyonun sahibi (`owner`) olur. Rolün verdiği iki izinden yalnızca QR girişi onayı bir uç noktada kullanılıyordu; QR onayı artık yalnızca oturum açmış olmayı ister.


## Kanonik web adresleri (CF-149, PR #160 entegrasyonu)

Kendi kendine kayıt `/register` adresindedir: hesap ve şirket → e-posta kodu
→ seçilen planda organizasyon ve deneme → `/getting-started` AI kurulumu.
Denemesiz veya hemen ödemeli planda sıradaki adres `/billing?pay=trial` olur.
Yarım kalan kayıt ve OAuth sonrası organizasyon oluşturma `/setup` üzerinden devam eder.
`/v1/app/config` içindeki `signup_mode=open` kayıt formunu, `invite` ve `closed`
demo talebini ve giriş bağlantısını gösterir. Landing CTA'ları aynı ayarı kullanır.
Eski kayıt ve kurulum adresleri `frontend/legacy-redirects.json` üzerinden query
parametreleri korunarak 308 yönlendirilir.
