import type { FeatureCatalog } from "./features";

export const featuresTr: FeatureCatalog = {
  inbox: {
    name: "Çok kanallı gelen kutusu",
    summary:
      "E-posta, WhatsApp, Instagram, Messenger, Telegram, Slack, Teams, Google Chat, SMS ve web sohbeti tek listede.",
    eyebrow: "Çok kanallı gelen kutusu",
    title: "Her kanal, tek gelen kutusu.",
    lead: "Müşterileriniz hangi kanaldan yazarsa yazsın konuşma aynı ekrana düşer. Ekibiniz atama, iç notlar, SLA ve hazır yanıtlarla kanal değiştirmeden çalışır.",
    description:
      "E-posta, WhatsApp, Instagram, Messenger, Telegram, Slack, Teams, Google Chat, SMS, API ve web widget konuşmaları tek gelen kutusunda.",
    chips: [
      "WhatsApp",
      "Instagram",
      "Messenger",
      "Telegram",
      "E-posta",
      "Slack",
      "Microsoft Teams",
      "Google Chat",
      "SMS",
      "Web widget",
      "API kanalı",
    ],
    highlights: [
      {
        title: "Resmi API'lerle bağlantı",
        body: "WhatsApp Business Cloud API, Instagram ve Messenger Meta Graph API üzerinden; Telegram, Slack, Microsoft Teams ve Google Chat kendi bot API'leriyle bağlanır. WhatsApp için gömülü kayıt ve şablon mesajları desteklenir.",
      },
      {
        title: "E-posta, kendi adresinizle",
        body: "Destek adresinizi ClickFix'e yönlendirin ya da IMAP ile posta kutunuzu bağlayın; yanıtlar SMTP üzerinden kendi alan adınızdan gider.",
      },
      {
        title: "SMS ve sesli kanal",
        body: "Netgsm, İleti Merkezi veya Twilio ile SMS gönderin; Twilio ile gelen SMS'ler de konuşmaya düşer. İYS izin kontrolü gönderimden önce yapılır. Twilio ve Netgsm arama kayıtları konuşmaya işlenir.",
      },
      {
        title: "Web widget",
        body: "Tek satır kodla sitenize eklenen sohbet penceresi markanızın renkleriyle açılır, ziyaretçiyi tanır ve konuşmayı gelen kutusuna taşır.",
      },
      {
        title: "Akıllı yönlendirme",
        body: "Sıralı dağıtım, yük dengeli dağıtım ve yetenek tabanlı yönlendirme; temsilci kapasite sınırlarıyla birlikte.",
      },
      {
        title: "SLA ve çalışma saatleri",
        body: "İlk yanıt ve çözüm hedefleri, çalışma saatleri ve tatiller. Süre dolmadan uyarı, kaçırılan hedefte eskalasyon.",
      },
    ],
    capabilities: [
      "Atama, öncelik, etiket ve durumlar",
      "İç notlar ve @bahsetme",
      "Kayıtlı görünümler ve filtreler",
      "Konuşma birleştirme",
      "Hazır yanıtlar (makrolar)",
      "CSAT, NPS ve CES anketleri",
      "Çoklu marka desteği",
      "Ekip içi sohbet",
      "API kanalı ile kendi kaynaklarınızdan konuşma açma",
      "Türkçe ve İngilizce arayüz",
    ],
  },
  ai: {
    name: "Yapay zekâ",
    summary:
      "AI ajanı, temsilci asistanı, kalite puanlama, görüşme transkripsiyonu, ClickFix Asistan ve rehberler.",
    eyebrow: "Yapay zekâ",
    title: "Önce AI yanıtlar, gerektiğinde ekibiniz devralır.",
    lead: "AI ajanı bilgi bankanızdan ve tanımladığınız prosedürlerden yanıt verir; emin olmadığında konuşmayı bağlamıyla birlikte ekibe devreder. Temsilcileriniz ise her yanıtta bir asistanla çalışır.",
    description:
      "AI ajanı, copilot, AI kalite puanlama, ses ve video transkripsiyonu, ClickFix Asistan: bilgi bankanızla çalışan, denetlenebilir yapay zekâ.",
    chips: [
      "AI ajanı",
      "Copilot",
      "Sınıflandırma",
      "Bilgi kaynakları",
      "Prosedürler",
      "Değerlendirme",
      "Transkripsiyon",
      "Kalite puanı",
    ],
    highlights: [
      {
        title: "AI ajanı",
        body: "Prosedürler, araçlar (HTTP çağrıları dahil) ve bilgi kaynaklarıyla yapılandırılır. Web sitenizi tarayarak kaynak ekleyebilir, yayına almadan önce değerlendirme senaryolarıyla test edebilirsiniz.",
      },
      {
        title: "Temsilci asistanı",
        body: "Gelen kutusunda tek tıkla taslak yanıt, özet, yeniden yazma, ton ayarı, çeviri, genişletme ve kısaltma.",
      },
      {
        title: "Otomatik sınıflandırma",
        body: "Gelen her konuşmanın dili, niyeti, kategorisi, duygu durumu ve önceliği belirlenir; etiket ve ekip önerisi yönlendirmeye yardımcı olur.",
      },
      {
        title: "Yapay zekâ destekli kurulum",
        body: "Web sitenizi ve sektörünüzü anlatın; ClickFix kanal, ekip, bilgi bankası ve otomasyon önerilerini hazır şablonlardan derleyip onayınıza sunar.",
      },
      {
        title: "ClickFix Asistan ve rehberler",
        body: "Panelde ⌘J ile açılan ClickFix Asistan dokümanlarda arar, kurulumunuzun neresinde olduğunuza bakar, önerdiği adımları sizin onayınızla uygular ve gerektiğinde sizi destek ekibimize bağlar. Her sayfada adım adım rehber turu da var.",
      },
      {
        title: "Transkripsiyon ve özet",
        body: "Sesli aramaların ve video görüşme kayıtlarının metni çıkarılır, kısa bir özetle konuşmanın zaman çizelgesine eklenir. Kayıt silindiğinde metni de silinir.",
      },
      {
        title: "AI kalite puanlama",
        body: "Konuşmalar kendi rubriklerinizle puanlanır; AI ön değerlendirmeyi yapar, kalite ekibi onaylar ve koçluk notuna dönüştürür.",
      },
      {
        title: "Denetlenebilir",
        body: "AI yanıtları kayıt altındadır ve kalite incelemesine alınabilir. Ajanın performansını çözüm ve devir oranlarıyla izlersiniz.",
      },
      {
        title: "ClickFix Asistan: iş yapan yardımcı",
        body: "Panelde ⌘J ile açılır; bulunduğunuz sayfada neyi eksik bıraktığınızı söyler, kanal, iş akışı, SLA, webhook ve widget kurulumunu onay kartıyla uygular, kuru çalıştırır ve sonucu doğrular. Aynı yetenekler MCP sunucusuyla Claude ve Cursor'a da açılır.",
      },
      {
        title: "Sisteminizi okur",
        body: '"Sistemini bağla" ile tanımladığınız kavramları (fatura, sipariş, lisans) müşterinin dış kimliğiyle sizin sisteminizden okur, tarifteki adımı izler, onaylı işleri çalıştırır. Tek çalışma saati kaynağıyla mesai dışı metni, widget durumu ve SLA hep aynı şeyi söyler; her yeni konuşma 10 saniyede karşılanır.',
      },
    ],
    capabilities: [
      "Bilgi bankası makalelerinden yanıt (RAG)",
      "İnsan temsilciye bağlamıyla devir",
      "Taslak, özet, çeviri ve ton araçları",
      "Niyet, kategori, duygu ve öncelik tespiti",
      "Web sitesi tarama ile bilgi kaynağı",
      "Değerlendirme senaryoları",
      "Türkçe ve İngilizce yanıt",
      "8 sektör için hazır kurulum şablonu",
      "Ses ve video kayıtlarından transkript ve özet",
      "Rubrik tabanlı AI kalite puanlama",
      "MCP sunucusu ile kendi AI araçlarınıza bağlantı",
      "ClickFix Asistan: onay kartıyla kurulum ve test",
      "Sistemini bağla ile sisteminizden okuma",
      "Tek çalışma saati ve 10 saniyede anında yanıt",
    ],
  },
  "knowledge-base": {
    name: "Bilgi bankası ve yardım merkezi",
    summary:
      "Makaleler, herkese açık yardım merkezi ve topluluk forumu; AI ajanının da kaynağı.",
    eyebrow: "Self servis",
    title: "Müşterileriniz cevabı kendisi bulsun.",
    lead: "Bilgi bankasında yazdığınız makaleler hem herkese açık yardım merkezinizde yayınlanır hem de AI ajanının yanıt kaynağı olur. Topluluk forumu, müşterilerinizin birbirine yardım ettiği alanı ekler.",
    description:
      "Bilgi bankası, markanıza özel herkese açık yardım merkezi ve topluluk forumu. Makaleleriniz AI ajanının da kaynağıdır.",
    chips: ["Makaleler", "Koleksiyonlar", "Arama", "Yardım merkezi", "Forum"],
    highlights: [
      {
        title: "Zengin editör",
        body: "Başlıklar, listeler, tablolar, kod blokları ve görsellerle makale yazın; koleksiyonlarla düzenleyin.",
      },
      {
        title: "Herkese açık yardım merkezi",
        body: "Her marka için ayrı yardım merkezi adresi, Türkçe ve İngilizce sayfalar, site haritası ve arama.",
      },
      {
        title: "Topluluk forumu",
        body: "Kategoriler ve konular altında müşterileriniz soru sorar, birbirine ve ekibinize yanıt verir.",
      },
      {
        title: "AI ile bağlantılı",
        body: "AI ajanı yanıtlarını yayınlanmış makalelere dayandırır; makaleyi güncellediğinizde ajanın bilgisi de güncellenir.",
      },
    ],
    capabilities: [
      "Koleksiyon ve makale yapısı",
      "Tam metin arama",
      "Marka başına yardım merkezi",
      "Türkçe ve İngilizce içerik",
      "Topluluk forumu",
      "Arama motorları için site haritası",
    ],
  },
  "appointments-video": {
    name: "Randevu ve video odaları",
    summary:
      "Herkese açık randevu sayfaları ve tarayıcıda çalışan görüntülü görüşme odaları.",
    eyebrow: "Randevu ve görüntülü görüşme",
    title: "Randevu alın, görüşmeyi tarayıcıda yapın.",
    lead: "Müşterileriniz herkese açık randevu sayfanızdan uygun saati seçer. Görüntülü görüşmeler için kurulum gerektirmeyen video odaları açın; misafir bağlantısı, PIN ve uçtan uca şifreleme seçenekleriyle.",
    description:
      "Herkese açık randevu sayfaları, takvim ve LiveKit tabanlı video odaları: misafir bağlantısı, PIN, uçtan uca şifreleme, kayıt ve transkript.",
    chips: [
      "Randevu sayfası",
      "Müsaitlik",
      "Misafir bağlantısı",
      "PIN",
      "E2EE",
      "Kayıt",
    ],
    highlights: [
      {
        title: "Randevu sayfaları",
        body: "Randevu türleri, süreler ve temsilci müsaitliği. Müşteri kendi bağlantısından randevusunu yönetir.",
      },
      {
        title: "Tarayıcıda video",
        body: "LiveKit altyapısıyla çalışan odalar; müşterinizin uygulama indirmesine gerek yoktur.",
      },
      {
        title: "Erişim kontrolü",
        body: "Misafir bağlantıları, isteğe bağlı PIN ve oda bazında zorunlu uçtan uca şifreleme.",
      },
      {
        title: "Kayıt",
        body: "İsteğe bağlı görüşme kaydı; saklama süresi ve kullanım sınırlarıyla. Kayıtların transkripti ve özeti otomatik çıkarılır.",
      },
    ],
    capabilities: [
      "Herkese açık randevu sayfası",
      "Temsilci takvimleri ve müsaitlik",
      "Müşteri tarafında randevu yönetimi",
      "Misafir bağlantısı ve PIN",
      "Uçtan uca şifreleme seçeneği",
      "Görüşme kaydı ve saklama süresi",
      "Kayıt transkripti ve AI özeti",
    ],
  },
  "customer-relationships": {
    name: "Müşteri ilişkileri",
    summary:
      "Şirketler, kişiler, doğrulanmış müşteri kimliği, dış kimlik, özel alanlar, sağlık skoru ve tek zaman çizelgesi.",
    eyebrow: "CRM-lite",
    title: "Konuşmanın arkasındaki müşteriyi tanıyın.",
    lead: "Kişiler şirketlerine bağlanır; her şirketin konuşmaları, gelir bilgisi ve sağlık skoru tek ekranda. Kendi sisteminizdeki müşteri numarası dış kimlik olarak gelir, doğrulanmış müşteri rozetle ayrılır, paket ve lisans gibi özel alanlar kartta durur.",
    description:
      "Şirket ve kişi kayıtları, imzalı user_hash ile doğrulanmış müşteri, dış kimlik, özel alanlar, 0-100 müşteri sağlık skoru ve tüm etkileşimleri gösteren zaman çizelgesi.",
    chips: [
      "Şirketler",
      "Kişiler",
      "Doğrulanmış",
      "Dış kimlik",
      "Özel alanlar",
      "Sağlık skoru",
    ],
    highlights: [
      {
        title: "Doğrulanmış müşteri",
        body: "Sitenizde ya da uygulamanızda oturum açan kullanıcı, sunucunuzda imzalanan user_hash ile tanıtılır; firma kimliği de imzaya girer. Panelde doğrulanmış, anonim ve adını kendisi yazan ayrılır; süzgeçle listelenir.",
      },
      {
        title: "Dış kimlik",
        body: "Kişi ve firmanın sizin sisteminizdeki numarası birinci sınıf alandır: kartta görünür, aranır, API ve webhook'larda döner. Kişiyi dış kimlikle API'den açıp müşteri siteye girmeden ilk mesajı atabilirsiniz.",
      },
      {
        title: "Özel alanlar: tanımla, gönder, bitti",
        body: "Alanı bir kez tanımlayın; widget ve API aynı anahtarla gönderir, tanımsız alan uyarı olarak görünür. Liste alanları katalogdur: SLA, iş akışı ve makale hedefleme aynı seçenekleri kullanır; hassas alanlar AI'a gitmez.",
      },
      {
        title: "Sağlık skoru ve zaman çizelgesi",
        body: "Kullanım, gelir ve ürün olayı sinyallerinden ağırlıklı 0-100 skor; kişi ve şirket bazında konuşmalar, notlar ve olaylar kronolojik olarak. Müşterinin hangi ekrandan yazdığı konuşma başlığında.",
      },
    ],
    capabilities: [
      "Şirket ve kişi kayıtları",
      "Doğrulanmış / anonim / adını kendisi yazdı ayrımı",
      "Kişi ve firma dış kimliği",
      "Özel alan kataloğu ve tanımsız alan uyarısı",
      "Şirket gelir ve kullanım sinyali",
      "Ağırlıklı sağlık skoru",
      "Kişi ve şirket zaman çizelgesi",
      "Ekran bağlamı (hangi ekrandan yazdı)",
      "CSV / XLSX içe ve dışa aktarma",
    ],
  },
  engineering: {
    name: "Mühendislik köprüsü",
    summary:
      "Destek konuşmalarını Linear, Jira ve GitHub kayıtlarına bağlayın.",
    eyebrow: "Mühendislik köprüsü",
    title: "Destek ve yazılım ekibi aynı kaydı konuşsun.",
    lead: "Bir hata bildirimi geldiğinde konuşmayı Linear, Jira veya GitHub kaydına bağlayın. Kayıt ilerledikçe ekibiniz bilgilenir, çözüldüğünde etkilenen müşterilere dönüş yapılır.",
    description:
      "Destek konuşmalarını Linear, Jira, GitHub ve Sentry kayıtlarına bağlayın; takip ve gelir etkisiyle birlikte.",
    chips: ["Linear", "Jira", "GitHub", "Sentry", "Takip", "Gelir etkisi"],
    highlights: [
      {
        title: "Kayıt bağlama",
        body: "Konuşmadan yeni kayıt açın ya da mevcut bir kayda bağlayın; ClickFix'in yerleşik kayıtlarını da kullanabilirsiniz.",
      },
      {
        title: "Takip",
        body: "Kayıt durumu değiştiğinde bağlı konuşmalar için takip görevleri oluşur; müşteriye dönüş unutulmaz.",
      },
      {
        title: "Gelir etkisi",
        body: "Bir hatadan etkilenen şirketleri ve toplam gelirlerini görerek önceliklendirin.",
      },
    ],
    capabilities: [
      "Linear, Jira ve GitHub bağlantısı",
      "Sentry hata kayıtları",
      "Yerleşik kayıtlar",
      "Konuşma-kayıt ilişkisi",
      "Takip görevleri",
      "Etkilenen gelir görünümü",
    ],
  },
  automation: {
    name: "Otomasyon",
    summary: "Kurallar, görsel iş akışı editörü, makrolar ve SLA.",
    eyebrow: "Otomasyon",
    title: "Tekrarlayan işi kurala, kalanını ekibe bırakın.",
    lead: "Görsel editörde adım adım iş akışı çizin, makrolar ve yönlendirmeyle tekrarlayan işi azaltın. Sürümleri, çalışma geçmişi ve test ekranıyla neyin neden çalıştığını görürsünüz.",
    description:
      "Kural tabanlı otomasyon, görsel iş akışı editörü, makrolar, SLA politikaları ve akıllı yönlendirme.",
    chips: ["Kurallar", "İş akışları", "Makrolar", "SLA", "Yönlendirme"],
    highlights: [
      {
        title: "API ile kurallar",
        body: "Olaylara bağlı tetikleyici, koşul ve eylem kuralları API üzerinden tanımlanır: etiketle, ata, öncelik ver, bildirim gönder.",
      },
      {
        title: "Görsel iş akışı editörü",
        body: "Adımları sürükleyip bağlayın; sürümleyin, test edin ve her çalışmanın kaydını inceleyin.",
      },
      {
        title: "Makrolar",
        body: "Birden fazla eylemi tek tıkla uygulayan hazır yanıt ve işlem paketleri.",
      },
      {
        title: "SLA ve yönlendirme",
        body: "SLA politikaları, çalışma saatleri, sıralı, yük dengeli ve yetenek tabanlı dağıtım.",
      },
    ],
    capabilities: [
      "Olay tabanlı tetikleyiciler",
      "Koşul ve eylem kuralları (API)",
      "Görsel iş akışı editörü",
      "Sürümler, test ve çalışma geçmişi",
      "Makrolar",
      "SLA politikaları ve eskalasyon",
      "Yetenek tabanlı yönlendirme",
    ],
  },
  analytics: {
    name: "Analitik ve ekip yönetimi",
    summary: "Raporlar, kalite değerlendirme, koçluk ve iş gücü planlama.",
    eyebrow: "Analitik ve ekip",
    title: "Ne olduğunu görün, ekibinizi geliştirin.",
    lead: "Yanıt süreleri, SLA uyumu ve memnuniyet raporlarıyla başlayın; kalite değerlendirme, koçluk ve iş gücü planlamasıyla ekibinizi büyütün.",
    description:
      "Destek raporları, CSAT/NPS/CES, kalite değerlendirme rubrikleri, koçluk ve Erlang tabanlı iş gücü tahmini.",
    chips: ["Raporlar", "CSAT / NPS / CES", "Kalite", "Koçluk", "İş gücü"],
    highlights: [
      {
        title: "Raporlar",
        body: "Hacim, yanıt ve çözüm süreleri, SLA uyumu, kanal ve temsilci kırılımları.",
      },
      {
        title: "Kalite değerlendirme",
        body: "Puanlama rubrikleriyle manuel inceleme ve kontrollü AI incelemesi.",
      },
      {
        title: "Koçluk",
        body: "Kör kalibrasyon oturumları, gelişim görevleri ve KPI rozetleri.",
      },
      {
        title: "İş gücü planlama",
        body: "Erlang tabanlı ihtiyaç tahmini, temsilci takvimleri ve müsaitlik.",
      },
    ],
    capabilities: [
      "Hazır destek raporları",
      "CSAT, NPS ve CES anketleri",
      "Kalite rubrikleri",
      "AI destekli kalite incelemesi",
      "Kalibrasyon ve gelişim görevleri",
      "Erlang tahmini ve vardiya takvimleri",
    ],
  },
  "status-page": {
    name: "Durum sayfası ve nöbet",
    summary: "Herkese açık durum sayfası, olay yönetimi ve nöbet çizelgeleri.",
    eyebrow: "Olay yönetimi",
    title: "Kesinti olduğunda herkes aynı sayfada.",
    lead: "Bileşenlerinizin durumunu herkese açık sayfada yayınlayın, olayları ve bakımları duyurun, aboneleri bilgilendirin. Nöbet çizelgeleri doğru kişiye ulaşmanızı sağlar.",
    description:
      "Herkese açık durum sayfası, olay ve bakım duyuruları, abonelikler; rotasyon, eskalasyon ve çağrı ile nöbet yönetimi.",
    chips: ["Bileşenler", "Olaylar", "Bakım", "Abonelik", "Nöbet"],
    highlights: [
      {
        title: "Durum sayfası",
        body: "Bileşen bazında durum, olay geçmişi ve planlı bakım duyuruları.",
      },
      {
        title: "Abonelikler",
        body: "Müşterileriniz durum güncellemelerine abone olur; olay ilerledikçe bilgilendirilir.",
      },
      {
        title: "Nöbet",
        body: "Rotasyonlar, eskalasyon politikaları, çağrı ve onaylama.",
      },
    ],
    capabilities: [
      "Bileşen ve olay yönetimi",
      "Planlı bakım duyuruları",
      "Durum aboneliği",
      "Nöbet rotasyonları",
      "Eskalasyon ve çağrı",
      "Onaylama takibi",
    ],
  },
  security: {
    name: "Güvenlik ve KVKK",
    summary: "KVKK araçları, SSO, SCIM, iki adımlı doğrulama ve denetim kaydı.",
    eyebrow: "Güvenlik ve KVKK",
    title: "Müşteri verisi, kurallara uygun ve kontrol altında.",
    lead: "KVKK kapsamındaki dışa aktarma, silme ve saklama süreleri ürünün içinde. Kurumsal oturum açma, kullanıcı senkronizasyonu ve denetim kaydı hazır.",
    description:
      "KVKK veri talepleri, saklama süreleri, SSO (SAML/OIDC), SCIM, iki adımlı doğrulama, passkey ve denetim kaydı.",
    chips: ["KVKK", "SSO", "SCIM", "2FA", "Passkey", "Denetim kaydı"],
    highlights: [],
    capabilities: [],
  },
  mobile: {
    name: "Mobil",
    summary:
      "Ekibiniz için iOS ve Android uygulaması; müşterileriniz için uygulama içi destek (webview köprüsü, Flutter paketi, uygulama anahtarı).",
    eyebrow: "Mobil",
    title: "Gelen kutunuz cebinizde, desteğiniz müşterinizin uygulamasında.",
    lead: "ClickFix iOS ve Android uygulamasıyla sahadan yanıtlayın. Kendi mobil uygulamanıza ise oturumu taşıyan güvenli bir webview ile destek ekranı ekleyin: Flutter, React Native, iOS ve Android aynı köprü sözleşmesiyle bağlanır.",
    description:
      "ClickFix iOS ve Android uygulaması; müşteri uygulamalarına webview köprüsü, Flutter clickfix_webview paketi, visitor-core çekirdeği ve uygulama anahtarıyla uygulama içi destek.",
    chips: [
      "iOS",
      "Android",
      "Anlık bildirim",
      "Webview köprüsü",
      "Flutter",
      "React Native",
    ],
    highlights: [
      {
        title: "Anlık bildirim ve yanıtlama",
        body: "Yeni ve size atanan konuşmalar için bildirim; konuşma geçmişi, yanıt, iç not ve çevrimiçi durum uygulamadan.",
      },
      {
        title: "Uygulama içi destek (webview)",
        body: "Uygulamanız getclickfix.com'daki widget sayfasını güvenli bir webview'de açar; kullanıcı sunucunuzda imzalanan user_hash ile tanıtılır, derin bağlantıyla doğrudan konuşmaya gidilir, dış bağlantılar sistem tarayıcısında açılır.",
      },
      {
        title: "Flutter paketi ve yerel çekirdek",
        body: "clickfix_webview (pub.dev) köprü sözleşmesini ClickFixController ve ClickFixWebView olarak uygular. @getclickfix/visitor-core ile uygulama anahtarı kullanarak widget'ı webview olmadan da çağırır, oturumu webview'e devredersiniz.",
      },
      {
        title: "Bildirimden konuşmaya",
        body: "message.sent webhook olayından kendi anlık bildiriminizi gönderin; bildirimin derin bağlantısı destek ekranını o konuşmada açar. Okunmamış rozeti widget'ın unread olayından gelir.",
      },
    ],
    capabilities: [
      "iOS ve Android ekip uygulaması",
      "Anlık bildirimler ve çevrimiçi durum",
      "Webview köprü sözleşmesi v1 (identify, track, open, session)",
      "Flutter clickfix_webview paketi",
      "React Native için visitor-core ve köprü yardımcıları",
      "Uygulama anahtarı (Origin'siz güvenli erişim)",
      "Derin bağlantı ve bildirimden konuşmaya",
      "Türkçe ve İngilizce arayüz",
    ],
  },
  developers: {
    name: "Geliştirici platformu",
    summary:
      "REST API, imzalı webhook'lar, resmi SDK'lar (Node.js, Go, PHP, Laravel, React, Flutter), API kanalı ve MCP sunucusu.",
    eyebrow: "Geliştiriciler",
    title: "ClickFix'i kendi sistemlerinize bağlayın.",
    lead: "Kapsamlı anahtarlarla çalışan REST API, imzalı webhook'lar, altı dilde resmi SDK, kendi sohbetinizi kanal yapan API kanalı, sisteminizi AI'a okutan \"Sistemini bağla\" ve her adımda çalışan örnek içeren entegrasyon kılavuzu.",
    description:
      "ClickFix REST API, imzalı webhook'lar, Node.js / Go / PHP / Laravel / React / Flutter SDK'ları, API kanalı, MCP sunucusu ve adım adım entegrasyon kılavuzu.",
    chips: ["REST API", "Webhook", "SDK", "OpenAPI", "API kanalı", "MCP"],
    highlights: [
      {
        title: "REST API ve başvuru",
        body: "Kapsam tanımlı API anahtarları, tutarlı yanıt zarfı, Idempotency-Key ile güvenli tekrar. Her uç için şema, örnek istek/yanıt, curl/Node.js/Go/PHP örneği ve tarayıcıda deneyebileceğiniz playground; dokümantasyon Türkçe ve İngilizce.",
      },
      {
        title: "Resmi SDK'lar",
        body: "@getclickfix/node, Go modülü, clickfix/sdk (PHP), clickfix/laravel, @getclickfix/react-widget ve @getclickfix/visitor-core; hepsi MIT lisanslı, açık kaynak ve çalışan örnek uygulamalarıyla (clickfix-examples) yayımlanır.",
      },
      {
        title: "Entegrasyon kılavuzu",
        body: "Kurulum sihirbazıyla aynı sırada dokuz adım: kimlik doğrulama ve firma imzası, dış kimlik, özel alanlar, olaylar, sistemini bağla, API kanalı, webhook'lar, mobil. Her adımda widget, Node.js, Go, PHP ve curl örneği; örnekler her sürümde çalıştırılarak doğrulanır.",
      },
      {
        title: "İmzalı webhook'lar ve API kanalı",
        body: "Her teslimat HMAC-SHA256 ile imzalanır ve zaman damgası taşır; başarısız teslimatlar 24 saate kadar yeniden denenir. API kanalıyla kendi sohbet uygulamanız bir kanal olur: müşteri mesajını gönderirsiniz, temsilci yanıtı webhook'la gelir.",
      },
      {
        title: "Sistemini bağla",
        body: "Muhasebe, e-ticaret ya da SaaS'ınızın kavramlarını (fatura, sipariş, lisans) ve işlerini tek ekranda tanımlayın; AI ve temsilci müşterinin dış kimliğiyle sizin sisteminizden okur, onaylı işleri çalıştırır.",
      },
      {
        title: "Otomasyon platformları ve MCP",
        body: "REST hook abonelikleri ve yoklama tetikleyicileriyle Zapier, Make ve n8n; MCP sunucusuyla Claude ve Cursor gibi istemcilerden ClickFix araçlarına erişim. Kenar çubuğu uygulamaları imzalı bağlam belirteciyle konuşma ekranına gömülür.",
      },
    ],
    capabilities: [
      "Kapsam tanımlı API anahtarları",
      "İki dilli API başvurusu ve playground",
      "Her uçta curl / Node.js / Go / PHP örneği",
      "Idempotency-Key desteği",
      "HMAC-SHA256 imzalı webhook'lar",
      "Node.js, Go, PHP, Laravel, React ve Flutter SDK'ları",
      "clickfix-examples ile çalışan örnek uygulamalar",
      "API kanalı (kendi sohbetiniz kanal olur)",
      "Sistemini bağla: kavramlar, uçlar, işler, AI tarifi",
      "REST hook abonelikleri ve MCP sunucusu",
      "Kenar çubuğu uygulamaları",
      "llms.txt ve her sayfanın Markdown çıktısı",
    ],
  },
};
