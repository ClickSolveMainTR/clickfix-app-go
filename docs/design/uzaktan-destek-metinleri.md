# Uzaktan destek: aydınlatma ve açık rıza metinleri (taslak, CF-237)

> **Hukuki onay notu:** Bu metinler ürün ekibinin taslağıdır, hukuki görüş değildir. KVKK danışmanının ve şirket hukukçusunun onayından geçmeden ClickFix Destek uygulamasına (CF-239) ve web arayüzüne konmaz. Onaydan sonra metnin sürümü ve onay tarihi bu dosyaya yazılır; müşteri uygulaması rızayı hangi sürümle aldığını kayda geçirir.

Bağlam: `docs/design/masaustu.md` 5. bölüm (onaylar ayrı ayrı alınır) ve 7. bölüm (KVKK: aydınlatma ve açık rıza ayrıdır, kayıt ayrı bir rızadır). Ekran içeriği kişisel veri, hatta özel nitelikli kişisel veri içerebilir; bu yüzden izleme için açık rıza istenir.

Köşeli parantezli alanlar (`[Organizasyon]` gibi) oturumdan doldurulur.

## 1. Aydınlatma metni (her oturumda, rızadan ayrı ekranda)

**Türkçe**

> **Uzaktan destek hakkında bilgilendirme**
>
> Veri sorumlusu **[Organizasyon]**'dur. ClickFix, hizmeti [Organizasyon] adına veri işleyen olarak sunar.
>
> **Hangi veriler işlenir:** Oturum süresince ekranınızın görüntüsü; onay verirseniz temsilcinin fare ve klavye olayları; onay verirseniz oturum kaydı; oturumun zamanı, süresi, verdiğiniz ve geri aldığınız onaylar, cihazınızın işletim sistemi ve uygulama sürümü, IP adresiniz.
>
> **Amaç:** [Organizasyon] ile süren destek talebinizi çözmek, oturumun güvenliğini sağlamak ve kötüye kullanımı önlemek.
>
> **Hukuki sebep:** Ekran görüntüsü ve kayıt için açık rızanız (KVKK md. 5/1, özel nitelikli veri görünürse md. 6). Oturum kayıtları (zaman, onaylar, IP) için veri sorumlusunun meşru menfaati ve hukuki yükümlülükleri (md. 5/2-ç, 5/2-f).
>
> **Aktarım:** Görüntü ClickFix'in Türkiye'deki kendi medya ve aktarma sunucularından geçer; yurt dışına aktarılmaz. Kayıt yalnız onay verirseniz alınır ve [Organizasyon]'un belirlediği **[saklama süresi] gün** sonunda silinir.
>
> **Haklarınız:** KVKK md. 11 kapsamındaki haklarınızı (bilgi isteme, düzeltme, silme, itiraz) [Organizasyon]'a [başvuru adresi] üzerinden kullanabilirsiniz. Oturumu istediğiniz an **Durdur** düğmesiyle ya da `Ctrl+Alt+Shift+S` kısayoluyla bitirebilirsiniz.
>
> **Uyarı:** ClickFix çalışanları ve işletmeler sizden asla para transferi, banka girişi veya kart bilgisi istemez. Böyle bir istek gelirse oturumu durdurun ve **Dolandırıcılık bildir** düğmesine basın.

**English**

> **About remote support**
>
> The data controller is **[Organization]**. ClickFix provides the service as a data processor on behalf of [Organization].
>
> **Data processed:** the image of your screen during the session; if you approve, the agent's mouse and keyboard events; if you approve, a recording of the session; the session's time and length, the consents you give and withdraw, your device's operating system and app version, your IP address.
>
> **Purpose:** resolving your ongoing support request with [Organization], keeping the session secure and preventing abuse.
>
> **Legal basis:** your explicit consent for the screen image and the recording; the controller's legitimate interest and legal obligations for the session log (time, consents, IP).
>
> **Transfers:** the image passes through ClickFix's own media and relay servers in Türkiye and is not transferred abroad. A recording is made only with your consent and is deleted after **[retention] days** set by [Organization].
>
> **Your rights:** you can exercise your rights under KVKK art. 11 (information, correction, erasure, objection) with [Organization] at [contact]. You can end the session at any time with the **Stop** button or the `Ctrl+Alt+Shift+S` shortcut.
>
> **Warning:** ClickFix staff and businesses will never ask you for a money transfer, bank login or card details. If someone does, stop the session and press **Report a scam**.

## 2. Açık rıza metinleri (ayrı kutular, önceden işaretli değil)

| Rıza | Türkçe | English |
|---|---|---|
| İzleme (zorunlu) | [Organizasyon] temsilcisi [Temsilci]'nin bu oturum süresince ekranımı görmesine açık rıza veriyorum. | I explicitly consent to [Agent] from [Organization] seeing my screen during this session. |
| Kontrol (temsilci istediyse) | [Temsilci]'nin bu oturum süresince fare ve klavyemi kullanmasına izin veriyorum. İzni istediğim an geri alabilirim. | I allow [Agent] to use my mouse and keyboard during this session. I can withdraw this at any time. |
| Kayıt (organizasyon kayıt açtıysa) | Bu oturumun kaydedilmesine ve [saklama süresi] gün saklanmasına açık rıza veriyorum. Hayır dersem oturum kayıtsız sürer. | I explicitly consent to this session being recorded and kept for [retention] days. If I say no, the session continues unrecorded. |

## 3. Temsilci tarafı (bilgi amaçlı)

Temsilci konsolu bu metinleri göstermez; yalnız hangi rızanın verildiğini, ne zaman verildiğini ve geri alındığını gösterir. Bütün adımlar organizasyonun denetim kaydındadır.

## 4. Açık sorular (hukuki görüşe)

1. Ekranda üçüncü kişilerin verisi (ör. açık bir e-posta) göründüğünde veri sorumlusunun yükümlülüğü ve uyarı metni.
2. Kayıt saklama süresinin üst sınırı (şu an teknik sınır 365 gün).
3. Rızanın sürümlenmesi: metin değiştiğinde süren oturumların durumu.
