# Netgsm SMS ve Netsantral

Netgsm SMS ve ses, ayrı ClickFix kanallarıdır. Her kanal ve çağrı organizasyon kapsamındadır. API kullanıcı bilgileri kanal vault'unda; temsilci SIP bilgileri organizasyon üyeliğinde, organizasyon ve kullanıcıya bağlı AES-GCM zarfında saklanır. Gerçek değerleri repo, ekran görüntüsü veya loglara yazmayın.

## Kurulum

1. Netgsm portalında API kullanıcısı oluşturun ve gerekli SMS, bakiye ve Netsantral rapor yetkilerini açın. API IP kısıtı varsa ClickFix worker'ın çıkış IP adresini izin listesine alın.
2. ClickFix Kanallar'dan Netgsm SMS ekleyin: `usercode`, `password`, `msgheader`, `webhook_token`. Token en az 32 karakter olmalıdır. Ticari SMS için mevcut İYS yapılandırması ve izin kontrolü geçerlidir.
3. Ses kanalı ekleyin: sağlayıcı `netgsm`, `usercode`, `password`, `webhook_token`. Parolalar salt yazılır; GET yanıtlarında dönmez. Kanal sağlık kontrolü `POST /balance` üzerinden API erişimini doğrular; kanal detayındaki sağlık açıklaması bakiyeyi gösterir. Bu kontrol SIP kaydının veya ses medyasının çalıştığını tek başına doğrulamaz.
4. Netsantral **API Talep Ayarları** bölümüne kanalın mutlak webhook URL'sini `?token=<webhook_token>` ile girin: `/v1/webhooks/channels/voice/<instance_uuid>?token=<webhook_token>`. Inbound_call, Outbound_call, Local_call, Answer, Hangup, Queue, InboundtoPBX, Context, DTMF, QueueLeave ve cdr olaylarını açın. JSON ve URL-encoded form kabul edilir. Hatalı token 401; doğrulanmış hatalı/işlenemeyen gövde dahil diğer bildirimler 200 alır. Worker CDR sorgusu eksik çağrıları tamamlar.
5. SMS portalında **SMS Hizmeti → İnteraktif SMS → URL Adresine Yönlendir** alanına SMS kanalının `/v1/webhooks/channels/sms/<instance_uuid>?token=<webhook_token>` URL'sini girin. `messageId`, `subscriberNumber`, `sourceNumber`, `messageDateTime`, `content` alanları işlenir. İsteğe bağlı `from_number` hedef numara kontrolü sağlar.
6. Ses kanalı ayarlarında `ws_url` (Netsantral'in sağladığı `wss://` adresi), `domain` (SIP domain) ve `ice_servers` JSON dizisini ayarlayın. ICE örneği biçimi: `[{"urls":"stun:stun.example.org:3478"}]`. TURN kimlik bilgilerini genel kanal ayarlarına yazmayın; yalnız erişimi kısıtlı, süreli TURN servisi kullanın.
7. Netsantral'da her temsilci için ayrı **WSS** dahili açın. Üyeler → erişim düzenleme → Netsantral SIP bölümünde yönetici/sahip `sip_extension`, `sip_username`, `sip_password` alanlarını kaydeder. Dahili aynı organizasyonda tek üyeye ait olabilir. Şifreyi boş bırakmak kayıtlı değeri korur. `PUT /v1/tenant/webphone/members/{user}` üzerindeki `clear_password` alanı şifreyi siler.
8. Temsilci tarayıcısında mikrofon iznini verin. Telefon paneli hazır olduğunda kişi ve konuşma başlığındaki **Ara** kullanılabilir. Gelen çağrı panelinde Kabul et/Reddet; etkin görüşmede Sessiz/Beklet/Aktar ve tuş takımı vardır. Aktarım için santralde SIP REFER yetkisi açık olmalıdır.

## Worker ve kayıtlar

- Redis ve asynq worker çalışmalıdır. Dakikalık iş her canlı Netgsm kanalına ayrı sorgu işi açar. CDR hesabın `usercode` özetine bağlı Redis anahtarıyla en az 31 saniye aralıklı sorgulanır. Bugün ve dün, İstanbul saatinde 00:00–23:59 pencereleriyle dönüşümlü taranır. İki günden uzun kesintiler için geçmiş günlerin yeniden çalıştırılması gerekir.
- SMS teslimat raporları gönderilmiş mesajların sağlayıcı iş kimlikleriyle sorgulanır. Gelen SMS `GET /sms/rest/v2/inbox` üzerinden son 24 saat için açık tarih aralığıyla alınır; sağlayıcının bir kez okuma/silme davranışı kullanılmaz. Tekrarlı mesajlar sağlayıcı kimliğiyle tekilleştirilir.
- Kayıt arşivi yalnız HTTPS `netgsm.com.tr` ve alt alan adlarından indirilir. Kullanıcı bilgisi/özel port ve yönlendirme reddedilir; süre 60 saniye, üst sınır 100 MB. Kuruluşun ek/plan kotası daha düşük olabilir. İndirme veya saklama başarısızlığı asynq tarafından yeniden denenir.
- Dosya mevcut `InboundAttachment`/mesaj eki akışına girer. Başarılı saklama sonrasında `channel_voice_calls.recording_message_id` bağlanır; migration 000202'nin CF-122 transkripsiyon tetikleyicisi devreye girer. Transkripsiyon servisinin mevcut yapılandırması ve ücretlendirmesi geçerlidir.
- Browser çağrı özeti aynı temsilci, yön, son 10 haneli telefon ve ±120 saniyelik en yakın tek kayıt ile birleştirilir. İki ayrı sağlayıcı kimliği telefon benzerliği yüzünden birleştirilmez. Sağlayıcı sonucu istemci özetinden önceliklidir.
- CDR hata kodları: 30 API kimliği/yetki, 40 kayıt yok, 50 tarih formatı, 60 pencere >24 saat, 70 parametre, 80 sorgu sınırı, 100 sistem hatası, 101 dakikalık sorgu sınırı.

## Ağ ve sorun giderme

Bu sürüm tarayıcıdan doğrudan SIP over WSS/WebRTC kullanır; ClickFix Go API medya taşımaz. WSS TLS sertifikası tarayıcı tarafından güvenilir olmalı, proxy WebSocket yükseltmesini desteklemeli, santralin giden arama ve bakiye yetkileri açık olmalıdır. Ses yok/tek yönlü ses durumunda ICE/TURN, NAT ve sağlayıcının medya erişimini kontrol edin. Tarayıcı HTTPS üzerinde çalışmalıdır.

Referans `docs/operations/VOICE_AGENT_RUNBOOK.md` uyarıları: Netsantral bazı hatlarda `0` ile başlayan yerel numara bekler. 401/407 kimlik/dahili bağlantı tipi, 403 giden arama izni, 404/484 numara biçimi, 486/600 meşgul veya PBX reddi olabilir. Düşük bakiye nedeniyle erken 486 + Q.850 cause=17 oluşabilir.

Sunucu tarafı SIP köprüsü ileride kurulursa 5060 TCP/UDP ve RTP 10000–10100 UDP yalnız Netgsm'in destekten alınan güncel IP listesine açılmalıdır. Docker host portları UFW'yi atlayabilir; `DOCKER-USER` veya bulut güvenlik grubu ile sınırlandırın. Bu portlar tarayıcı softphone'u için ClickFix sunucusunda açılmaz.

## Ortam ve kapsam

Mevcut altyapı adları: `APP_ENCRYPTION_KEY`, `PUBLIC_API_URL`, `REDIS_*`, `ATTACHMENT_MAX_FILE_BYTES`, `ATTACHMENT_ORG_QUOTA_BYTES`, `S3_*`. Yeni global Netgsm parolası/ortam değişkeni yoktur; kimlik bilgileri her organizasyonda kanal/üye üzerinden girilir.

Takip işleri: outbound IVR/sesli SMS; LiveKit SIP AI ajanı ve sunucu medya köprüsü. Bu sürümün kapsamında değildir.

Resmi gelen SMS sözleşmesi: https://www.netgsm.com.tr/dokuman/ — Gelen SMS / JSON GET, `GET /sms/rest/v2/inbox`; webhook yönlendirme alanları aynı sayfada. Garanti referansında gelen SMS bulunmadığından bu bölüm resmi sözleşmeye göre yeni yazılmıştır.
