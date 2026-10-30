# Lansman kontrol listesi (CF-135)

Bu liste 30 Ekim 2026 lansmanı için yazıldı. Sıra önemli: altyapı → ortam değişkenleri → deploy → canlı kontroller →
dış hesaplar → duyuru. Kim yapacak sütununda **Sahip** hesap/kimlik gerektiren adımları, **Ekip** repo ve sunucu
adımlarını gösterir.

## 1. Altyapı (sunucu: `ssh clickfix-server`, Dokploy: dokploy.getclickfix.com)

| # | Adım | Kim | Not |
|---|---|---|---|
| 1.1 | DNS A kayıtları: `@`, `www`, `api`, `widget`, `rt` → `134.199.188.121` | Sahip | Tamamlandı (29 Eki) |
| 1.2 | `video` (ve TURN kullanılacaksa `turn`) A kaydı | Sahip | [video-rooms.md](./video-rooms.md) |
| 1.3 | DigitalOcean güvenlik duvarı: TCP 80/443, TCP 7881, UDP 3478, UDP 50000–50100, UDP 50200–50300 | Sahip | LiveKit için |
| 1.4 | 4 GB swap (`/swapfile`, swappiness 10) | Ekip | Tamamlandı (29 Eki) |
| 1.5 | SMTP çıkışı: DigitalOcean 25/465/587 portlarını kapatıyor | Sahip | Sistem e-postası için `SMTP_PORT=2587`. Müşterilerin kendi SMTP'leri için DigitalOcean'a destek bileti |
| 1.6 | Yedekleme: Postgres yedeği ve SeaweedFS'teki saklama kuralı | Ekip | [DISASTER_RECOVERY.md](./DISASTER_RECOVERY.md); geri yüklemeyi bir kez deneyin |

## 2. Ortam değişkenleri (Dokploy)

Tam liste ve hangi ekrandan alınacağı: [ortam-degiskenleri.md](./ortam-degiskenleri.md). Lansman için asgari:

- [ ] Kurulumda üretilen sırlar (JWT, şifreleme anahtarı, DB/Redis/S3/Meili/Centrifugo, VAPID). [deploy-dokploy.md](./deploy-dokploy.md)
- [ ] E-posta: `SMTP_PASSWORD` (Resend), `SMTP_PORT=2587`, `RESEND_API_KEY`, `RESEND_WEBHOOK_SECRET`, `EMAIL_INBOUND_DOMAIN`
- [ ] Yapay zekâ: `ANTHROPIC_API_KEY`, altı `AI_*_PRICE_MICROS`, `OPENAI_API_KEY`, `AI_EMBEDDING_MODEL`, `AI_EMBEDDING_INPUT_PRICE_MICROS`
- [ ] Faturalama: `BILLING_SELLER_JSON`; ödeme alınacaksa `PAYMENTS_PAYTR_*` veya `PAYMENTS_IYZICO_*` (önce `MODE=sandbox`); webhook adresleri ve `PUBLIC_API_URL` için [odeme.md](./odeme.md)
- [ ] Meta: `META_*`, `META_CHANNELS_ENABLED=true`, `META_GRAPH_VERSION=v26.0`
- [ ] Slack: `SLACK_CHANNEL_*`
- [ ] Görüntülü görüşme (isteğe bağlı): `LIVEKIT_URL`, `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`
- [ ] Hata izleme (isteğe bağlı): `BACKEND_SENTRY_DSN`, `FRONTEND_SENTRY_DSN`

CI'daki `scripts/check-compose-env.sh` kontrolü, kodun okuduğu her değişkenin compose'a geçtiğini garanti eder.

## 3. Deploy ve tek seferlik komutlar

| # | Adım | Komut / yer |
|---|---|---|
| 3.1 | Son `v0.N` etiketini Dokploy'da deploy edin, tüm konteynerlerin sağlıklı olduğunu görün (`worker-sync`, `livekit`, `livekit-egress` dahil) | Dokploy |
| 3.2 | `https://api.getclickfix.com/readyz` → `ready` | tarayıcı / curl |
| 3.3 | Mevcut aboneliklere dönem AI kredisini yükleyin | backend konteyneri: `/app/backfill-plan-credits` |
| 3.4 | Süper admin | backend konteyneri: `/app/create-super-admin` |
| 3.5 | (İsteğe bağlı) Demo organizasyonu | backend konteyneri: `/app/seed-demo` ([demo-org.md](./demo-org.md)) |
| 3.6 | Arama dizinlerini doldurun | backend konteyneri: `/app/search-reindex` |

## 4. Canlı duman testleri (deploy sonrası, sırayla)

- [ ] Kayıt → yapay zekâ destekli kurulum (şablon ile) → ekip daveti e-postası geliyor
- [ ] Widget'tan mesaj → gelen kutusu → ata → yanıt → çöz → CSAT
- [ ] E-posta kanalı: yönlendirme adresine gelen posta konuşma oluyor, yanıt müşteriye gidiyor
- [ ] Yapay zekâ: triage etiketi geliyor, copilot yanıt önerisi üretiyor; Ayarlar → Yapay zekâ → Çalışma kaydı'nda hata yok
- [ ] Bilgi bankası makalesi yayınla → yardım merkezinde görünüyor, aramada çıkıyor
- [ ] Meta: test sayfası/IG hesabı/WhatsApp test numarasıyla mesaj gelip gidiyor
- [ ] Slack: kanal bağla (uygulamayı bir kez yeniden onaylayın), mesaj → konuşma, gönderen adı görünüyor
- [ ] Randevu: herkese açık sayfadan randevu al → takvimde görünüyor → hatırlatma geliyor
- [ ] Görüntülü oda (LiveKit açıksa): misafir bağlantısıyla PIN'li odaya gir, ekran paylaş, dakika kredisi düşüyor
- [ ] Mobil: giriş, gelen kutusu, mesaj gönderme (canlı API'ye karşı)
- [ ] KVKK: bir kişi için dışa aktarma talebi ZIP üretiyor; hesap silme talebi oturumu kapatıyor
- [ ] Ödeme (CF-132): sandbox modda bir plan satın alma, fatura oluşuyor; sonra canlı modda küçük tutarla tekrar

## 5. Dış hesaplar ve başvurular (Sahip)

| Konu | Durum | Not |
|---|---|---|
| Meta: callback'leri kaydet, işletme doğrulaması, Tech Provider, App Review | Webhook'lar bağlı | Review onayı günler sürebilir; o zamana kadar kanallar geliştirme modunda (test hesapları) |
| e-Fatura entegratörü seçimi ve test hesabı (CF-31, CF-38, CF-16) | Bekliyor | Seçilince entegratör bağlantısı yazılacak |
| PayTR / iyzico canlı hesap (CF-16, CF-132) | Bekliyor | Önce sandbox anahtarları |
| Apple Developer (ücretli) ve Google Play hesapları, EAS (CF-17, CF-83) | Bekliyor | Mağaza gönderimi bunlara bağlı |
| Microsoft 365 + Azure Bot (Teams) | Bekliyor | [slack-teams-kurulum.md](./slack-teams-kurulum.md) |
| DigitalOcean SMTP port bileti | Bekliyor | Madde 1.5 |
| Yasal metinler (KVKK aydınlatma, gizlilik, koşullar, çerez, DPA) hukuk onayı | Taslak yayında | `/legal/*` |

## 6. Gözlem ve geri dönüş

- Uyarılar: [OBSERVABILITY.md](./OBSERVABILITY.md) (Prometheus kuralları, `sync` kuyruğu birikmesi dahil).
- Sunucu belleği: `free -m`. Swap sürekli doluysa egress/worker sınırlarını düşürün veya sunucuyu büyütün.
- Geri dönüş: Dokploy'da önceki etiketi yeniden deploy edin. Migration'lar geriye dönük uyumlu yazıldı; veritabanını
  geri almak gerekirse [DISASTER_RECOVERY.md](./DISASTER_RECOVERY.md).

## 7. Lansman sonrası (bilinçli olarak ertelenenler)

Gmail/Microsoft OAuth ile e-posta bağlama, yapay zekâ ses ajanı ve transkripsiyon, harici takvim senkronu (Google/
Microsoft), forumda müşteri girişi, SDK paketlerinin npm/Go yayını, mobil widget SDK'sı.
