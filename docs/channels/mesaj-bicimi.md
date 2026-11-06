# Mesaj biçimi ve kanallar (CF-177)

Ajan ve AI mesajları tek bir **kaynak biçimde** yazılır ve veritabanında bu hâliyle durur. Mesaj bir kanala gönderilirken o kanalın anladığı biçime çevrilir. Çeviri yalnız gönderim anında, adaptörün `Send` işlevinde yapılır; saklanan içerik değişmez. Bu yüzden yeniden denenen bir gönderim iki kez çevrilmez.

Kod: `backend/internal/platform/channels/format` (saf paket, bağımlılığı yok).

## Kaynak biçim

Web widget'ının Markdown alt kümesidir (`frontend/widget-sdk/src/shared/markdown.ts`):

| Yazılan | Anlamı |
|---|---|
| `**metin**` | Kalın |
| `*metin*` veya `_metin_` | İtalik |
| `***metin***` | Kalın italik |
| `` `metin` `` | Satır içi kod |
| Üç ters tırnaklı satırlar arası | Kod bloğu |
| `[ad](https://adres)` | Adlı bağlantı (yalnız `http`, `https`, `mailto`) |
| `https://adres` | Bağlantı |
| `- madde`, `* madde`, `• madde` | Madde listesi |
| `1. madde`, `1) madde` | Numaralı liste |

Düz metnin bozulmaması için kurallar:

- İşaret yalnız **kelime sınırında** açılır ve ardından boşluk gelmez. `2*3*4`, `5 * 3`, `kelime*x*` ve eşi olmayan `*` metin olarak kalır.
- `_` kelime içinde kapanmaz. `snake_case`, `dosya_adi.pdf` ve `_ozel_ad` metin olarak kalır. `*` kelime içinde kapanabilir (`**Ayarlar**dan`), çünkü Türkçede ek bitişik yazılır.
- Adresler bütün olarak okunur; içindeki `_` ve `*` işaret sayılmaz. Adres her tür boşlukta biter (bölünemez boşluk dahil).
- Üst üste gelen işaret dizileri (`****x****`, `kelime***x***`) metin olarak kalır.
- Üstü çizili yazı kaynak biçimde yoktur; `~x~` metindir.

Go ayrıştırıcısı ile widget ayrıştırıcısı aynı dosyadaki örneklerle sınanır: `backend/internal/platform/channels/format/testdata/markdown_cases.json`. Kural değişirse iki ayrıştırıcı ve bu dosya birlikte güncellenir.

## Kanal tablosu

| Kanal | Gönderilen biçim | Not |
|---|---|---|
| `widget` | Kaynak biçim, değişmez | SDK kendi gösterir |
| `email` | HTML bölümü: `<strong>`, `<em>`, `<code>`, `<pre>`, `<ul>`/`<ol>`, `<a>`. Düz metin bölümü: işaretsiz | Bütün metin HTML'de kaçırılır |
| `whatsapp` | `*kalın*`, `_italik_`, `` `kod` ``, kod bloğu, `- ` ve `1. ` listeleri; adlı bağlantı `ad (adres)` | Metin, medya başlığı ve interaktif gövde çevrilir. Onaylı şablonlara dokunulmaz |
| `telegram` | Düz metin + `entities` (`bold`, `italic`, `code`, `pre`, `text_link`) | Kaçış gerekmez. Uzaklıklar UTF-16. Başlıkta `caption_entities` |
| `slack` | mrkdwn: `*kalın*`, `_italik_`, `<adres\|ad>`, `•` listesi | `&`, `<`, `>` her yerde kaçırılır |
| `teams` | CommonMark, `textFormat: "extendedmarkdown"` | Bloklar boş satırla ayrılır, satır sonu iki boşlukla yazılır. Düz metindeki `*`, `_`, `` ` ``, `[`, `~`, `$`, `<`, `&` ile satır başındaki `#`, `>`, `+ `, `---` ve `===` kaçırılır |
| `google_chat` | Markdown; istekte `markupSyntax: "MARKUP_SYNTAX_MARKDOWN"` | Aşağıya bakın |
| `instagram`, `messenger` | İşaretsiz düz metin; adlı bağlantı `ad (adres)` | Meta bu kanallar için metin biçimi belgelemiyor |
| `sms` | İşaretsiz düz metin | |
| `voice` | Okunacak metin: işaret, madde imi ve kod bloğu yok; adlı bağlantının yalnız adı | Satır içi kod okunur (çoğu zaman sipariş numarasıdır) |
| `api` | Kaynak biçim, değişmez | Aşağıya bakın |
| `test` | Değişmez | |

WhatsApp ve Slack'te kaçış karakteri olmadığı için düz metindeki `~x~` üstü çizili görünürdü. Bu iki kanalda böyle bir çiftin tildeleri görünüşü aynı olan `∼` (U+223C) ile yazılır; kod içindeki tildelere dokunulmaz.

Çeviri sonunda gönderilecek metin kalmazsa (mesaj yalnız bir kod çitinden oluşuyorsa) kaynak metin olduğu gibi gönderilir; kanal boş içerik hatası vermez.

Bir kanal işareti yalnız kelime sınırında tanıyorsa (WhatsApp, Slack) ve kalın metnin hemen ardından harf geliyorsa (`**Ayarlar**dan`), işaret hiç yazılmaz. Böylece müşterinin ekranında tek başına `*` kalmaz.

### Google Chat

Chat, `markupSyntax` verilmezse uygulamanın yapılandırmasındaki söz dizimini kullanır: eski uygulamalarda Chat söz dizimi (`*kalın*`), yeni uygulamalarda Markdown (`**kalın**`). Aynı metin iki uygulamada farklı görünmesin diye söz dizimi her istekte açıkça yazılır. Markdown seçildi, çünkü kaynak biçime en yakın olanıdır, numaralı listeyi destekler (Chat söz dizimi desteklemez) ve Google'ın yeni uygulamalardaki varsayılanıdır.

## Uzunluk sınırı

Sınır, çevrilmiş metne uygulanır (`**x**` WhatsApp'ta iki karakter kısalır, Slack'te `&` beş karaktere çıkar). Telegram'da uzun metin 4096 UTF-16 birimlik parçalara bölünür; `entities` aynı yerden kesilip her parçaya göre yeniden hesaplanır, yani parçalar biçimini korur ve hiçbir işaret ortadan bölünmez.

## Biçimli gönderim reddedilirse

Mesaj kaybolmaz, bir kez daha düz metin olarak gönderilir:

- **Telegram:** `entities` ile ilgili bir 400 yanıtında aynı metin `entities` olmadan gönderilir.
- **Teams:** 400 yanıtında `textFormat: "plain"` ve işaretsiz metinle gönderilir.
- **Google Chat:** 400 yanıtında `markupSyntax` olmadan, işaretsiz metinle gönderilir.

401, 403 ve 404 biçimle ilgili olmadığı için ikinci istek gönderilmez.

Diğer kanallarda biçim metnin içindedir ve sağlayıcı onu doğrulamaz; ayrı bir düz metin denemesi yoktur.

## API kanalı

API kanalında ClickFix çeviri yapmaz: webhook'unuza giden `text` alanı kaynak biçimdedir, yani `**kalın**`, `[ad](adres)` ve listeler içerebilir. Metni kendi sisteminizde nasıl göstereceğinize siz karar verirsiniz; işaretsiz metin isterseniz işaretleri kendi tarafınızda temizleyin.

## Panelde gösterim

- Web gelen kutusu, giden mesajları widget ayrıştırıcısıyla biçimli gösterir (`frontend/src/features/inbox/components/message-content.tsx`). Gelen mesajlar ve iç notlar yazıldığı gibi gösterilir.
- Gelen kutusu listesi önizlemesi yalnız **giden** (ajan ve AI) mesajlarda işaretleri temizler (`model.PreviewOutbound`). Müşterinin yazdığı `_abc_` veya `*x*` önizlemede yazıldığı gibi kalır. Bildirimler aynı önizlemeyi kullanır.

## Bilinen sınırlar

- Gelen mesajlar kaynak biçime çevrilmez (ör. WhatsApp'tan gelen `*kalın*`). Mobil uygulama gelen mesajları da Markdown olarak okuduğu için bu metin mobilde italik görünür.
- Google Chat'te düz metindeki işaretleri kaçırmanın belgelenmiş bir yolu yok; `2*3*4` olduğu gibi gönderilir.
- Parantez içeren adres adlı bağlantıda ilk `)` karakterinde kesilir (`[x](https://ex.com/a(b)c)`). Widget'ın eski davranışıdır, değiştirilmedi.
- Kanal çıktıları sağlayıcı dokümanlarına göre yazıldı; gerçek uygulamalarda denenmedi.
