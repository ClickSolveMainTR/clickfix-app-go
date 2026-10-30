# Google Chat kurulumu

CF-137. Google Chat kanalı **organizasyon başınadır**: her müşteri kendi Google Cloud projesinde bir Google Chat
uygulaması (HTTP endpoint modu) ve bir hizmet hesabı (service account) açar, ClickFix'e hizmet hesabı anahtarını
(JSON) ve proje numarasını girer. Sunucu tarafında Google Chat için **ortam değişkeni yoktur**; anahtar kanalın
kimlik bilgilerinde `APP_ENCRYPTION_KEY` ile şifreli saklanır ve bir daha gösterilmez.

Kod: `backend/internal/platform/channels/googlechat`. Alan adları `docs/meta-kurulum.md` ile aynı: panel
`https://getclickfix.com`, API `https://api.getclickfix.com`.

> Google Chat uygulamaları yalnızca **Google Workspace** hesaplarında çalışır (kişisel Gmail hesabıyla Chat
> uygulaması yayınlanamaz). Aşağıdaki adımları Workspace yöneticisi ya da projede yetkili bir kullanıcı yapar.

## Özet

| Ne                     | Nerede                                                                                  | ClickFix alanı                                 |
| ---------------------- | --------------------------------------------------------------------------------------- | ---------------------------------------------- |
| Proje numarası         | Google Cloud Console → proje seçici → **Project number** (ya da IAM & Admin → Settings) | `project_number`                               |
| Hizmet hesabı anahtarı | IAM & Admin → Service Accounts → hesap → Keys → Add key → JSON (indirilen dosya)        | `service_account_json`                         |
| HTTP endpoint URL      | ClickFix'te kanal oluşturulunca gösterilir                                              | Google Chat API → Configuration'a yapıştırılır |
| Odalar                 | Kanal ayarları → Yönlendirilen Google Chat odaları                                      | `settings.space_ids`                           |

## 1. Google Cloud projesi

1. [console.cloud.google.com](https://console.cloud.google.com) → proje seçici → **New project** (ya da var olan
   bir projeyi kullanın). Proje, müşterinin Workspace organizasyonu altında olmalıdır.
2. Proje panosundaki **Project number** değerini not edin (yalnızca rakam, ör. `123456789012`). Bu, proje
   kimliği (`my-project-id`) **değildir**.

## 2. Google Chat API'yi etkinleştirin

APIs & Services → Library → **Google Chat API** → **Enable**.

## 3. Hizmet hesabı ve anahtar

1. IAM & Admin → **Service Accounts** → **Create service account**. Ad örneği: `clickfix-chat`. Rol vermeye gerek
   yoktur: uygulama Chat API'yi `https://www.googleapis.com/auth/chat.bot` kapsamıyla, uygulama kimliğiyle
   çağırır.
2. Oluşan hesap → **Keys** → **Add key** → **Create new key** → **JSON**. Dosya bilgisayarınıza iner.
   - Organizasyonda `iam.disableServiceAccountKeyCreation` politikası açıksa anahtar oluşturulamaz; Workspace /
     Cloud yöneticisinin bu proje için istisna tanıması gerekir.
   - Dosyayı e-postayla ya da sohbetle paylaşmayın; yalnızca ClickFix'e yapıştırın ve ardından silin.

## 4. ClickFix'te kanalı oluşturun

Panel → **Kanallar** → **Kanal ekle** → **Google Chat**:

- **Ad**: gelen kutusunda görünecek ad (ör. "Google Chat – Destek").
- **Google Cloud proje numarası**: 1. adımdaki rakamlar.
- **Hizmet hesabı anahtarı (JSON)**: 3. adımda inen dosyanın içeriğini yapıştırın ya da **Dosyadan yükle** ile
  seçin. Alan yalnızca yazılabilir; kaydedildikten sonra panelde maskeli görünür.

**Bağla** dediğinizde ClickFix anahtarla Google'dan bir erişim token'ı alarak bilgileri doğrular (hatalı anahtar
ya da proje numarası `422` döner). Ardından ekranda kopyalama düğmeli **HTTP endpoint URL** görünür:

```
https://api.getclickfix.com/v1/webhooks/channels/google_chat/{kanal-uuid}
```

Aynı adres daha sonra kanal ayarlarında da görünür.

## 5. Chat uygulamasını yapılandırın

APIs & Services → Google Chat API → **Configuration**:

- **Build this Chat app as a Google Workspace add-on**: **işaretlemeyin** (ClickFix klasik Chat uygulaması
  olay biçimini bekler).
- **App name**: ör. `ClickFix`. **Avatar URL**: herkese açık bir PNG adresi. **Description**: ör. "Destek
  ekibimize yazın".
- **Interactive features**: açık.
  - **Functionality**: _Receive 1:1 messages_ ve _Join spaces and group conversations_ işaretli.
- **Connection settings**: **HTTP endpoint URL** seçin ve 4. adımdaki adresi yapıştırın.
  - **Authentication Audience**: **Project Number**. ClickFix gelen her isteğin `Authorization: Bearer` JWT'sini
    bu ayara göre doğrular (aşağıda).
- **Visibility**: uygulamayı kullanabilecek kişi ve grupları seçin (alan adınızdaki belirli kişiler/gruplar ya
  da tüm alan adı).
- **Logs**: _Log errors to Logging_ açık kalabilir.

**Save**. Birkaç dakika içinde uygulama Google Chat'te aranabilir olur.

## 6. Kullanım ve odaların seçilmesi

- **Doğrudan mesaj**: Google Chat'te **Yeni sohbet** → uygulama adını arayın → mesaj yazın. Doğrudan mesajlar her
  zaman gelen kutusuna düşer; kişi, Google kullanıcısıdır (ad ve e-posta Google'dan gelir).
- **Odalar (spaces, grup sohbetleri)**: odaya uygulamayı ekleyin (oda adı → **Apps & integrations** → **Add
  apps**). Ardından ClickFix → Kanallar → Google Chat kanalı → **Ayarlar** → **Yönlendirilen Google Chat
  odaları** listesinden odayı işaretleyin (liste Chat API `spaces.list` ile uygulamanın üye olduğu odaları
  getirir; gerekirse `spaces/AAAA…` kimliği elle eklenebilir). **Seçilmeyen odaların mesajları alınmaz.**
  Odalarda Google, uygulamaya yalnızca uygulamanın **@etiketlendiği** mesajları gönderir.

## Çalışma şekli

- **Doğrulama**: Google her olayı `Authorization: Bearer <JWT>` ile gönderir. ClickFix şunları denetler: RS256
  imza, Google Chat'in yayınladığı sertifikalardan biriyle
  (`https://www.googleapis.com/service_accounts/v1/metadata/x509/chat@system.gserviceaccount.com`, bir saat
  önbellekte; bilinmeyen `kid` için en fazla dakikada bir yenilenir), `iss = chat@system.gserviceaccount.com`,
  `aud =` kanalın proje numarası, süresi dolmamış token. Biri tutmazsa `401` döner ve olay kaydedilmez.
- **Olaylar**: `MESSAGE` gelen mesaj olur. `ADDED_TO_SPACE` oda adını önbelleğe alır; uygulama bir mesajda
  @etiketlenerek eklendiyse o mesaj da alınır. `REMOVED_FROM_SPACE` ve diğer olaylar bir şey üretmez. Botların
  mesajları yok sayılır. Yanıt gövdesi `{}`'dir; yanıtlar Chat API ile ayrıca gönderilir.
- **Konuşmalar**: doğrudan mesaj alanı (DM) tek konuşmadır, kişi kullanıcıdır. Odalarda her **thread** ayrı bir
  konuşmadır, kişi odadır; mesajı yazan `author_name` / `author_email` olarak mesajda görünür (Slack'teki gibi).
- **Adlar**: kullanıcı ve oda adları olaydan alınır ve 24 saat önbellekte tutulur (Redis varsa süreçler arasında
  paylaşılır; anahtar organizasyon + proje kapsamlı). Olayda ad yoksa önbellekten, oda adı için gerekirse
  `spaces.get` ile okunur; okunamazsa mesaj yine alınır ve aynı kimlik 5 dakika sorulmaz.
- **Yanıtlar**: temsilci yanıtı `POST https://chat.googleapis.com/v1/{space}/messages` ile uygulama kimliğiyle
  gönderilir. Odalarda yanıt aynı thread'e gider (`thread.name`,
  `messageReplyOption=REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD`). `messageId=client-<mesaj-uuid>` yeniden denemeyi
  tekrarsız yapar. Erişim token'ı hizmet hesabıyla OAuth 2.0 JWT bearer akışıyla
  (`https://oauth2.googleapis.com/token`) alınır ve süresi dolana kadar önbellekte tutulur. JSON dosyasındaki
  `token_uri` dikkate alınmaz.
- **Sınırlar**: yalnızca metin (en fazla 4096 karakter). Dosya, kart ve tepki bu dilimde yok; dosya içeren bir
  mesaj metni yoksa "desteklenmeyen" olarak düşer. Seçimden çıkarılan bir odaya yanıt gönderilmez.
- **Sağlık kontrolü**: kanal kontrolü anahtarla yeniden token alır; anahtar silinmiş ya da hesap kapatılmışsa
  kanal hatalı görünür.

## Anahtar yenileme

1. Aynı hizmet hesabında yeni bir JSON anahtarı oluşturun.
2. ClickFix → kanal ayarları → **Hizmet hesabı anahtarı** alanına yeni dosyayı yükleyip kaydedin (boş bırakılan
   alan mevcut değeri korur).
3. Google Cloud'da eski anahtarı silin.

Proje numarası değişirse (başka bir projeye geçiş) bu yeni bir uygulamadır: yeni bir Google Chat kanalı
oluşturun.

## Sorun giderme

| Belirti                           | Neden / çözüm                                                                                                                          |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Chat'te "ClickFix yanıt vermiyor" | HTTP endpoint URL yanlış ya da Authentication Audience "Project Number" değil; ClickFix'teki proje numarası Cloud'dakiyle aynı olmalı. |
| Bağlarken `422`                   | JSON bir hizmet hesabı anahtarı değil, anahtar silinmiş ya da proje numarası rakam değil.                                              |
| Oda mesajları gelmiyor            | Oda kanal ayarlarında seçili değil ya da mesajda uygulama @etiketlenmedi.                                                              |
| Oda listesi yüklenmiyor           | Chat API etkin değil ya da anahtar geçersiz; oda kimliği elle eklenebilir.                                                             |
| Yanıt gitmiyor ("thread" hatası)  | Oda seçimden çıkarılmış; yeniden seçin.                                                                                                |
