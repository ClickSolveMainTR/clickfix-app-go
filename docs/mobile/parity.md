# Mobil ↔ web panel özellik eşitliği (CF-79)

Bu belge, web panelin (`frontend/src/app/(cms)`, `frontend/src/features`) ve `/v1/tenant/*` API'sinin sunduğu her özelliğin mobil uygulamadaki (`mobile/`) durumunu ve önceliğini tutar. Rakip mobil uygulamaların analizi en alttadır.

- **Durum**: `yok` (mobilde hiç yok), `kısmi` (bir kısmı var), `tamam`.
- **Öncelik**: `P0` temsilcinin her gün yaptığı iş; `P1` takım lideri / gün içinde sık; `P2` yönetici, ara sıra; `—` mobile taşınmaz (masaüstü işi).
- **Kim**: T = temsilci (agent), L = takım lideri, Y = organizasyon yöneticisi.
- "Web" sütunu: `ekran` panelde sayfa/bileşen var; `API` yalnızca API var, web arayüzü henüz yok (mobil ilk istemci olur).

Durumlar bu PR'ın (`cf-79-mobile-parity`) sonundaki hâli gösterir; "önce" sütunu PR #128 sonrası başlangıçtır. "Web" sütunu `main`'deki hâldir (#131 raporlar ve nöbet ekranlarını ekledi).

## 1. Gezinme ve kabuk

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Alt sekmeler: Gelen kutusu, Kişiler, Ajanda, Bildirimler, Daha fazla | ekran (yan menü) | T | P0 | kısmi (Gelen kutusu, Ajanda, Ayarlar) | tamam | En fazla 5 sekme (iOS kendi "More"una katlamasın). Ajanda yalnızca `tenant.appointments.read` olanlara görünür. "Daha fazla" merkezi her ikincil alana gider |
| Genel arama (Cmd+K): konuşma, kişi, şirket, makale | ekran (`search-engine`) | T | P0 | yok | tamam | `GET /v1/tenant/search`, tür çipli sonuçlar. Gelen kutusu ve Kişiler başlığındaki büyüteçten ve Daha fazla → "Ara ve bilgi bankası"ndan açılan ekran |
| Organizasyon değiştirici | ekran | T | P0 | kısmi (ayarlar içinde) | tamam | Daha fazla başlığında, tek dokunuş |
| iPad bölünmüş görünüm (liste + ayrıntı) | — | T | P1 | yok | tamam | ≥ 768 pt genişlikte gelen kutusu iki sütun |
| Karanlık mod, dinamik yazı, VoiceOver | ekran | T | P0 | kısmi | tamam | tüm yeni ekranlar token sınıfları + etiketler |

## 1b. Takvim, randevular ve görüntülü görüşme (CF-138, CF-139)

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Ajanda: yaklaşan randevular (benim / organizasyon), canlı güncelleme | ekran | T | P0 | tamam (#135) | tamam | Ajanda sekmesi + Daha fazla → "Takvim ve randevular" |
| Randevu ayrıntısı, durum değiştirme | ekran | T | P0 | tamam (#135) | tamam | `appointment/[uuid]` |
| Konuşmadan görüntülü görüşme başlat | ekran | T | P0 | tamam (#136) | tamam | konuşma başlığındaki kamera düğmesi; tablet bölmesinde de |
| Görüntülü görüşme odaları: canlı / bekleyen / biten, anında oda | ekran | T | P1 | yok | tamam | Daha fazla → "Görüntülü görüşme" (`GET/POST /v1/tenant/video/rooms`), bekleyen sayısı |
| Görüşmeye katılma, bekleme odası (içeri al / reddet), misafir bağlantısı | ekran | T | P0 | tamam (#136) | tamam | `video/[uuid]`, `r/[code]` |
| Kayıt başlat / durdur, kayıt listesi | ekran | L | P2 | yok | yok | sonraki iş |
| AI kurulum kartı (onboarding) | ekran | Y | P1 | tamam (#138) | tamam | Ayarlar'da ve Daha fazla merkezinde |

## 1c. Yardım: rehberler, ClickFix Asistan, geri bildirim (CF-141, CF-142)

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Ekran turları (coach mark), "Bu ekran ne işe yarar?" | ekran (CF-141) | T | P1 | yok | tamam | her başlıkta "?" menüsü; hedef kimlikleri `docs/mobile/guides.md` |
| Kurulum turu (onboarding listesi) | ekran (CF-141) | Y | P1 | yok | tamam | girişten sonra bir kez; Daha fazla → Yardım → Kurulum rehberi |
| ClickFix Asistan (sohbet, önerilen işlemler, desteğe iletme) | ekran (CF-141) | T | P1 | yok | tamam | `/assistant`; bağlam olarak ekran rotası |
| Sallayarak geri bildirim (ekran görüntüsüyle) | düğme (CF-141) | T | P2 | yok | tamam | Ayarlar'dan kapatılır |

## 2. Gelen kutusu

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Görünümler (Tümü, Bana atanan, Atanmamış, Bahsedilenler) | ekran | T | P0 | tamam | tamam | |
| Kayıtlı görünümler (`conversation-views`) + sayaçlar | ekran | T | P0 | yok | tamam | görünüm seçicide, sayaçlarla |
| Durum sekmeleri | ekran | T | P0 | tamam | tamam | |
| Filtre: takım, temsilci, öncelik, etiket, kanal, okunmamış, SLA ihlali, sıralama | ekran (`filter-bar`) | T | P0 | kısmi (öncelik, okunmamış, SLA) | tamam | tek filtre sayfası, etkin filtre çipleri, organizasyon başına saklanır |
| Arama | ekran | T | P0 | tamam | tamam | |
| Toplu seçim: ata, durum, öncelik, etiket, geri al | ekran (`bulk-bar`) | L | P1 | yok | tamam | `POST /conversations/bulk` + `bulk/undo` |
| Kaydırma işlemleri (üstlen / ertele / çöz) | — | T | P0 | tamam | tamam | |
| Erteleme hazır süreleri | ekran | T | P0 | tamam | tamam | |
| SLA rozetleri / sayaçları | ekran | T | P0 | tamam | tamam | |
| Okunmamış, çekerek yenileme, sonsuz kaydırma, canlı güncelleme | ekran | T | P0 | tamam | tamam | |

## 3. Konuşma

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Zaman tüneli: sistem olayları, iç notlar, ekler, teslim/okundu | ekran | T | P0 | tamam | tamam | |
| @bahsetmeler (yazma) | ekran | T | P0 | yok | tamam | not modunda `@` temsilci seçici, `mentions` alanı |
| Yanıt / not geçişi, ekler (kamera, foto, dosya) | ekran | T | P0 | tamam | tamam | |
| Hazır yanıtlar | ekran | T | P0 | tamam | tamam | |
| Makrolar (çalıştır) | ekran | T | P0 | yok | tamam | `POST /conversations/{uuid}/macros/{macro}` |
| Bilgi bankası makalesi ekle | ekran | T | P0 | yok | tamam | `knowledge/search` → bağlantı ekleme |
| AI copilot (öneri, özet, çeviri, ton) | ekran | T | P0 | tamam | tamam | |
| Başlıktan ata / takım / durum / öncelik / etiket / ertele | ekran | T | P0 | kısmi (ayrıntı sayfasında) | tamam | başlık menüsü + ayrıntı sayfası |
| Etiket ekle/kaldır | ekran | T | P0 | yok | tamam | `conversations/{uuid}/tags` |
| Birleştir / böl / ilişkili konuşmalar | ekran | L | P1 | yok | tamam | `merge`, `split`, `links` |
| Yan konuşmalar | ekran | T | P1 | yok | tamam | liste, yeni (e-posta), okuma, yanıt, kapatma |
| Hatırlatıcılar | ekran (`reminder-menu`) | T | P1 | yok | tamam | `conversations/{uuid}/reminders` |
| Çakışma: "şu an görüntülüyor" | ekran (`viewers-bar`) | T | P0 | yok | tamam | `conversations/{uuid}/presence` |
| CSAT sonucu | API | L | P1 | yok | kısmi | konuşma bazlı CSAT ucu yok; anket olayı zaman tünelinde, ortalama Raporlar'da ve şirket sayfasında |
| Kanal rozeti | ekran | T | P0 | kısmi (metin) | tamam | |
| Mühendislik kayıtları (görüntüle / oluştur / bağla) | API | T | P1 | yok | tamam | ayrıntılarda öneriler + eskalasyonlar, yeni kayıt (AI özetli), mevcut kayda bağla; mesajdan kayıt |
| Kişi kartı, SLA saatleri | ekran | T | P0 | tamam | tamam | |
| Talep formu yanıtı | ekran | T | P2 | yok | yok | sonraki iş |
| Yazıyor göstergesi (müşteri tarafı) | ekran | T | P2 | yok | yok | `conversation:{uuid}` kanalına abonelik sonraki iş; temsilciler arası "yazıyor" görüntüleyenler çubuğunda |

## 4. Kişiler ve şirketler

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Kişi listesi + arama | ekran | T | P0 | yok | tamam | |
| Kişi ayrıntısı: kimlikler, özel alanlar, konuşma geçmişi | ekran | T | P0 | kısmi (konuşma içinde kart) | tamam | |
| Özel alan düzenleme | ekran | T | P1 | yok | tamam | `PATCH /contacts/{uuid}` `custom_fields` |
| Şirket listesi + arama | ekran | T | P0 | yok | tamam | |
| Şirket ayrıntısı: genel bakış, zaman tüneli (CF-85), sağlık skoru, notlar | ekran | T/L | P1 | yok | tamam | `overview`, `timeline`, `health`, `notes` |
| İçe / dışa aktarma, birleştirme | ekran | Y | — | yok | yok | masaüstü işi |

## 5. Bilgi bankası

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Koleksiyonlara göre gezinme, arama | ekran | T | P0 | yok | tamam | |
| Makale görüntüleme, bağlantı paylaşma | ekran | T | P0 | yok | tamam | paylaşım sayfası (`Share`) |
| Taslakta hızlı düzenleme (başlık / metin), yayımlama | ekran | Y | P2 | yok | tamam | izin `tenant.knowledge.write` |
| Kaynaklar, incelemeler, çelişki kontrolü | ekran | Y | — | yok | yok | masaüstü işi |

## 6. Bildirimler

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Bildirim merkezi: liste, okundu, tümünü okundu | ekran | T | P0 | yok | tamam | sekme rozeti `unread-count` |
| Push → her varlığa derin bağlantı | — | T | P0 | kısmi (konuşma, QR) | tamam | kişi, şirket, makale, yan konuşma, ekip sohbeti, nöbet uyarısı, mühendislik kaydı |
| Bildirim tercihleri | ekran | T | P0 | tamam | tamam | |

## 7. Ekip

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Durum / müsaitlik | ekran | T | P0 | tamam | tamam | |
| Ekip durumu (kim çevrimiçi) | ekran (`team-status`) | L | P1 | yok | tamam | `GET /tenant/presence` |
| Ekip sohbeti (CF-106): odalar, mesajlar, okundu | API | T | P1 | yok | tamam | `team-chat` |
| Vardiyalar (okuma) | API | T | P1 | kısmi (kendi vardiyam) | kısmi | `schedules/shifts` için GET ucu yok; takım çalışma saatlerinden "Vardiyam" kartı ekip durumu ekranında |
| Nöbet: kim nöbette, uyarıyı onayla | ekran (`/oncall`, #131) | L | P1 | yok | tamam | `oncall/rotas/{uuid}/current`, `oncall/alerts/{uuid}/acknowledge` |

## 8. Raporlar

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Kişisel + takım KPI'ları: hacim, ilk yanıt, çözüm, CSAT, SLA | ekran (`/reports`, #131) | L | P1 | yok | tamam | `analytics/dashboard`, önceki döneme göre değişim, çubuk grafik (react-native-svg) |
| Özel raporlar, zamanlanmış raporlar | API | Y | — | yok | yok | masaüstü işi |

## 9. Olaylar ve mühendislik

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Açık olay bandı (durum sayfası) | API | T | P1 | yok | tamam | `status/incidents` |
| Mühendislik kayıtları listesi / ayrıntısı | API | T | P1 | yok | tamam | |

## 10. Yönetici (hafif)

| Özellik | Web | Kim | Öncelik | Önce | Sonra | Not |
|---|---|---|---|---|---|---|
| Kanal sağlığı listesi (+ kontrol et) | ekran | Y | P1 | yok | tamam | `channels`, `channels/{uuid}/check` |
| Üye listesi, davet gönder | ekran | Y | P1 | yok | tamam | `members`, `invitations` |
| AI aç/kapat, kredi bakiyesi | ekran | Y | P1 | yok | tamam | `ai/policy`, `billing/wallet` |
| Fatura özeti (abonelik, son faturalar) | ekran | Y | P2 | yok | tamam | salt okunur |
| Roller, SLA, yönlendirme, özel alan tanımları, widget, denetim kaydı | ekran | Y | — | yok | yok | masaüstü işi; "Daha fazla"da panelde aç bağlantısı |

## 11. Ayarlar / profil

| Özellik | Web | Kim | Öncelik | Önce | Sonra |
|---|---|---|---|---|---|
| Dil, tema | ekran | T | P0 | tamam | tamam |
| Biyometrik kilit | — | T | P0 | tamam | tamam |
| Oturumlar | ekran | T | P0 | tamam | tamam |
| Hesap silme | ekran | T | P0 | tamam | tamam |

## Rakip analizi

Kaynaklar: mağaza sayfaları ve yardım merkezleri (Ekim 2026'da tarandı; bağlantılar aşağıda), Crisp / Plain / Missive / HubSpot için ürün belgelerinden bilinen davranış (uygulamalar elle denenmedi).

| Uygulama | Mobilde sundukları | Güçlü kalıp | Biz alıyoruz |
|---|---|---|---|
| Intercom (Conversations) | Gelen kutusu, atama, bahsedilenler, iç not, kayıtlı yanıtlar, kişi/lead arama ve profil | Bahsedilenler görünümü, profil kartı | Bahsedilenler görünümü (vardı), genel kişi arama |
| Zendesk Support | Görünümler, makrolar, bilet oluşturma, Konuşma / Ayrıntılar sekmeleri, SLA, iPad'de raporlar | Konuşma ↔ Ayrıntılar ayrımı, makro uygulama, iPad'de pano | Makrolar, ayrıntı sayfası, iPad'de bölünmüş görünüm, Raporlar |
| Front | Ayarlanabilir kaydırma, çoklu seçim (arşiv, ertele), Gönder ve ertele, iç yorumlarda @bahsetme, kimin okuduğu | Çoklu seçim + ertele, yorumda @ | Toplu seçim, notta @bahsetme, görüntüleyenler |
| Help Scout | Atama, durum, etiket, arama, kayıtlı yanıtlar, notlar, zamanlanmış gönderim | Sade yazma alanı, etiketler | Etiket düzenleme, hazır yanıt |
| Freshdesk | Bilet ve kişi yönetimi, hazır yanıtlar, çözüm makalesi ekleme, özel/genel not, görünümler, kaydırma (kapat, ata) | Yanıta makale ekleme | Bilgi bankası makalesi ekleme |
| HubSpot Service | Biletler, gelen kutusu, kişi/şirket kaydı, görevler, bildirimler | CRM kaydı mobilde tam | Şirket sayfası (genel bakış, zaman tüneli, sağlık) |
| Gorgias | Görünümler, çok kanallı geçmiş tek görünümde, değişkenli makrolar, müşteri/sipariş bilgisi | Değişkenli makro, tek müşteri geçmişi | Kişi sayfasında konuşma geçmişi |
| Crisp | Sohbet, kişi profili, ekip içi mesajlaşma, bildirimler | Ekip sohbeti | Ekip sohbeti (CF-106) |
| Plain | Kuyruk / iş akışı, müşteri zaman tüneli, Linear/GitHub bağlantıları | Destek ↔ mühendislik bağı | Mühendislik kayıtları ekranı |
| Missive | Paylaşılan gelen kutusu, konuşma içi ekip sohbeti, atama | Konuşmaya bağlı ekip sohbeti | Yan konuşmalar + ekip sohbeti |

Kaynaklar:
[Intercom Conversations, App Store](https://apps.apple.com/app/intercom-conversations/id739927601) ·
[Zendesk mobile](https://www.zendesk.com/mobile) · [Zendesk yardım](https://support.zendesk.com/hc/en-us/articles/360026458794) ·
[Front mobil uygulama](https://help.front.com/en/articles/4907072) ·
[Help Scout mobil](https://docs.helpscout.com/article/1601-use-the-mobile-app) ·
[Gorgias mobil bilgi sayfası](https://docs.gorgias.com/faq/mobile-app-fact-sheet) ·
[Freshdesk mobil](https://freshworks.com/freshdesk/mobile)

### Ortak dersler

1. Rakiplerin çoğu mobilde **ayarları değil işi** taşır (Front bunu açıkça yazar). Biz de yönetimi "hafif" tutuyoruz: görüntüle, aç/kapat, davet et; tanımları panele bırakıyoruz.
2. **Ayrıntılar ayrı bir yüzeydir** (Zendesk). Konuşma ekranı yazmaya odaklıdır; atama/etiket/SLA/kişi ayrıntı sayfasındadır, sık işler başlık menüsündedir.
3. **Toplu seçim + ertele** (Front) ve **makro** (Zendesk, Gorgias) temsilcinin en sık mobil işleridir.
4. **Ekip içi konuşma** (Crisp, Missive) mobilde e-postadan önce gelir: yan konuşma + ekip sohbeti.
5. Mağaza yorumlarında en sık şikâyet eksik özelliklerdir (Intercom topluluğu, 2024). Eşitlik matrisi bu yüzden tutuluyor.

## Bilinçli olarak mobile taşınmayanlar

Roller ve izinler, SLA politikaları, yönlendirme kuralları, özel alan / nesne tanımları, talep formları, widget, kanal bağlama (OAuth akışları), içe / dışa aktarma, denetim kaydı, destek erişimi, AI prosedürleri / değerlendirmeleri, bilgi bankası kaynakları ve incelemeleri, özel rapor oluşturucu ve platform yönetimi. Bunlar tanım işidir; "Daha fazla → Web paneli aç" panele gider (Front ve Zendesk de mobilde bu ayrımı yapar).
