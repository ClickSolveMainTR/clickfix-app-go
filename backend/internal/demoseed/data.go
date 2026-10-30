package demoseed

// Fixture data of the demo organization. Every name, company, e-mail and
// phone number is fictional: e-mails use example.com / example.com.tr and
// phones the unassigned +90 555 000 xx xx range.

// AgentDomain is the e-mail domain of the demo staff accounts.
const AgentDomain = "anadolu-yazilim.example.com"

type demoUser struct {
	key, name, surname, role string
	teams                    []string
	lead                     bool
	skills                   []string
}

var demoUsers = []demoUser{
	{key: "selin.arslan", name: "Selin", surname: "Arslan", role: "owner"},
	{key: "mert.demir", name: "Mert", surname: "Demir", role: "admin", teams: []string{"Teknik Destek"}, skills: []string{"entegrasyon", "api"}},
	{key: "elif.kara", name: "Elif", surname: "Kara", role: "supervisor", teams: []string{"Teknik Destek", "Müşteri Başarısı"}, lead: true, skills: []string{"eskalasyon"}},
	{key: "burak.sahin", name: "Burak", surname: "Şahin", role: "agent", teams: []string{"Teknik Destek"}, skills: []string{"api", "mobil"}},
	{key: "zeynep.aydin", name: "Zeynep", surname: "Aydın", role: "agent", teams: []string{"Müşteri Başarısı"}, skills: []string{"eğitim"}},
	{key: "emre.koc", name: "Emre", surname: "Koç", role: "agent", teams: []string{"Faturalandırma"}, lead: true, skills: []string{"fatura", "iade"}},
	{key: "deniz.yildiz", name: "Deniz", surname: "Yıldız", role: "agent", teams: []string{"Teknik Destek", "Faturalandırma"}, skills: []string{"fatura", "mobil"}},
}

type demoTeam struct{ name, description, color string }

var demoTeams = []demoTeam{
	{"Teknik Destek", "Hata bildirimleri, entegrasyonlar ve API soruları", "#2563eb"},
	{"Müşteri Başarısı", "Kurulum, eğitim ve hesap sağlığı", "#16a34a"},
	{"Faturalandırma", "Fatura, ödeme ve iade talepleri", "#d97706"},
}

type demoCompany struct {
	name, domain, tier, notes string
	contacts                  []demoContact
}

type demoContact struct{ name, local, title string }

var demoCompanies = []demoCompany{
	{"Kuzey Lojistik A.Ş.", "kuzeylojistik.example.com.tr", "enterprise", "Filo takip modülünü 3 şubede kullanıyor.", []demoContact{
		{"Ahmet Yalçın", "ahmet.yalcin", "BT Müdürü"}, {"Gül Erdem", "gul.erdem", "Operasyon Uzmanı"}, {"Kaan Öztürk", "kaan.ozturk", "Yazılım Geliştirici"}, {"Sevgi Tunç", "sevgi.tunc", "Satın Alma"}}},
	{"Ege Gıda San. ve Tic. Ltd.", "egegida.example.com.tr", "growth", "Bayi portalı ve sipariş entegrasyonu.", []demoContact{
		{"Hakan Çelik", "hakan.celik", "Genel Müdür Yardımcısı"}, {"Ayten Kurt", "ayten.kurt", "Muhasebe"}, {"Onur Polat", "onur.polat", "Bayi Kanalı Sorumlusu"}}},
	{"Boğaziçi Mimarlık", "bogazicimimarlik.example.com", "standard", "Proje yönetimi paketi, 12 kullanıcı.", []demoContact{
		{"Derya Aksoy", "derya.aksoy", "Ofis Yöneticisi"}, {"Cem Tekin", "cem.tekin", "Kurucu Ortak"}}},
	{"Toros Enerji A.Ş.", "torosenerji.example.com.tr", "enterprise", "SCADA raporlama entegrasyonu, öncelikli destek sözleşmesi.", []demoContact{
		{"Levent Güneş", "levent.gunes", "Bilgi İşlem Direktörü"}, {"Pınar Acar", "pinar.acar", "Sistem Yöneticisi"}, {"Serkan Uysal", "serkan.uysal", "Saha Mühendisi"}, {"Nazlı Bulut", "nazli.bulut", "Sözleşme Yöneticisi"}}},
	{"Marmara Eğitim Kurumları", "marmaraegitim.example.com.tr", "growth", "Öğrenci bilgi sistemi, 4 kampüs.", []demoContact{
		{"Fatma Şen", "fatma.sen", "Kampüs Koordinatörü"}, {"Oğuz Kaplan", "oguz.kaplan", "BT Sorumlusu"}, {"İrem Doğan", "irem.dogan", "Öğrenci İşleri"}}},
	{"Karadeniz Tekstil", "karadeniztekstil.example.com", "standard", "Stok ve sevkiyat modülü.", []demoContact{
		{"Yusuf Karaca", "yusuf.karaca", "Üretim Planlama"}, {"Esra Avcı", "esra.avci", "Finans"}}},
	{"Akdeniz Turizm", "akdenizturizm.example.com.tr", "growth", "Rezervasyon kanalı entegrasyonu; sezonda yoğun.", []demoContact{
		{"Tolga Ersoy", "tolga.ersoy", "Rezervasyon Müdürü"}, {"Melis Uçar", "melis.ucar", "Dijital Pazarlama"}, {"Barış Kılıç", "baris.kilic", "Ön Büro Şefi"}}},
	{"Başkent Sağlık Grubu", "baskentsaglik.example.com.tr", "enterprise", "Randevu sistemi; KVKK hassasiyeti yüksek.", []demoContact{
		{"Dr. Ceren Özkan", "ceren.ozkan", "Başhekim Yardımcısı"}, {"Volkan Turan", "volkan.turan", "Bilgi Güvenliği"}, {"Aslı Yavuz", "asli.yavuz", "Hasta Hizmetleri"}}},
}

// Individual customers (no company) who write over WhatsApp / Instagram / the widget.
var demoIndividuals = []demoContact{
	{"Ali Vural", "ali.vural", ""}, {"Büşra Çetin", "busra.cetin", ""}, {"Can Akın", "can.akin", ""},
	{"Dilek Sarı", "dilek.sari", ""}, {"Eren Özdemir", "eren.ozdemir", ""}, {"Hande Gök", "hande.gok", ""},
	{"İsmail Keskin", "ismail.keskin", ""}, {"Merve Çakır", "merve.cakir", ""},
}

type demoChannel struct{ key, kind, name, externalID string }

var demoChannels = []demoChannel{
	{"email", "email", "Destek e-postası", "destek@" + AgentDomain},
	{"widget", "widget", "Web sitesi sohbeti", ""},
	{"whatsapp", "whatsapp", "WhatsApp Destek Hattı (demo)", "demo-whatsapp-905550000000"},
	{"instagram", "instagram", "Instagram @anadoluyazilim (demo)", "demo-instagram-anadoluyazilim"},
}

type demoTag struct{ name, color string }

var demoTags = []demoTag{
	{"fatura", "#d97706"}, {"hata", "#dc2626"}, {"özellik-talebi", "#7c3aed"}, {"entegrasyon", "#2563eb"},
	{"acil", "#b91c1c"}, {"kvkk", "#0f766e"}, {"eğitim", "#16a34a"}, {"iade", "#ea580c"},
}

// A scenario is one support story; the seeder repeats scenarios across
// contacts and channels to reach the target conversation count.
type scenario struct {
	subject  string
	team     string
	priority string
	tags     []string
	// msgs alternate starting with the customer: even index = customer,
	// odd index = agent. A leading "#" marks an internal note.
	msgs []string
}

var scenarios = []scenario{
	{"Faturada KDV tutarı hatalı görünüyor", "Faturalandırma", "normal", []string{"fatura"}, []string{
		"Merhaba, Ekim faturamızda KDV tutarı %20 yerine %18 üzerinden hesaplanmış görünüyor. Kontrol edebilir misiniz?",
		"Merhaba, bildiriminiz için teşekkürler. Faturayı inceliyorum, kısa süre içinde dönüş yapacağım.",
		"Teşekkürler, muhasebe kapanışı için cuma gününe kadar düzeltilmiş halini rica ediyoruz.",
		"Düzeltilmiş faturanız oluşturuldu ve e-posta adresinize gönderildi. Eski fatura iptal edildi.",
	}},
	{"API isteklerinde 429 hatası alıyoruz", "Teknik Destek", "high", []string{"hata", "entegrasyon"}, []string{
		"Sipariş senkronizasyonu sırasında saatte birkaç kez 429 Too Many Requests hatası alıyoruz.",
		"Merhaba, hesabınızın limiti dakikada 120 istek. Toplu uçları (bulk) kullanmanızı öneririm; dokümantasyon bağlantısını iletiyorum.",
		"#Müşterinin istek deseni gece 02:00'de patlıyor, limit artışı için ürün ekibine soralım.",
		"Bulk uca geçtik, sorun çözüldü gibi görünüyor. Teşekkürler!",
	}},
	{"Mobil uygulamada giriş yapılamıyor", "Teknik Destek", "urgent", []string{"hata", "acil"}, []string{
		"Sahadaki ekiplerimiz bu sabahtan beri mobil uygulamaya giriş yapamıyor, \"oturum süresi doldu\" hatası veriyor.",
		"Hemen inceliyoruz. Uygulama sürümünüzü paylaşabilir misiniz? (Ayarlar > Hakkında)",
		"Sürüm 4.2.1, hem Android hem iOS.",
		"4.2.2 sürümünde düzeltme yayınlandı. Mağazadan güncelleme sonrası giriş yapabilirsiniz.",
	}},
	{"Yeni kullanıcı eğitimi talebi", "Müşteri Başarısı", "low", []string{"eğitim"}, []string{
		"Ekibimize 5 yeni arkadaş katıldı, temel kullanım eğitimi planlayabilir miyiz?",
		"Elbette! Salı veya perşembe 14:00'te 45 dakikalık çevrim içi oturum açabiliriz. Hangisi uygun?",
		"Perşembe uygun, davetiyeyi gönderebilirsiniz.",
	}},
	{"Excel dışa aktarımına filtre eklenmesi", "Müşteri Başarısı", "low", []string{"özellik-talebi"}, []string{
		"Raporları Excel'e aktarırken tarih filtresinin de dosyaya yansımasını istiyoruz.",
		"Güzel öneri, ürün ekibimize talep olarak ilettim. Gelişmeleri bu konuşmadan paylaşacağım.",
	}},
	{"Kişisel verilerin silinmesi talebi (KVKK)", "Müşteri Başarısı", "high", []string{"kvkk"}, []string{
		"KVKK kapsamında ayrılan bir çalışanımızın kişisel verilerinin sistemden silinmesini talep ediyoruz.",
		"Talebinizi aldık. Kimlik doğrulaması için yetkili e-posta adresinizden onay rica ediyoruz.",
		"Onay e-postası gönderildi.",
		"Silme işlemi tamamlandı; ilgili kayıtlar anonimleştirildi. Yasal süre içinde yanıtlanmıştır.",
	}},
	{"Yıllık plana geçiş ve iade", "Faturalandırma", "normal", []string{"fatura", "iade"}, []string{
		"Aylık plandan yıllık plana geçmek istiyoruz. Bu ay ödediğimiz tutar mahsup edilir mi?",
		"Evet, kalan gün kadar tutar yıllık faturanızdan düşülür. Geçişi sizin için başlatayım mı?",
		"Evet lütfen.",
		"Geçiş tamamlandı, yeni faturanız hesabınızda görüntülenebilir.",
	}},
	{"Webhook bildirimleri gelmiyor", "Teknik Destek", "high", []string{"entegrasyon", "hata"}, []string{
		"Dünden beri sipariş webhook'ları sunucumuza ulaşmıyor.",
		"Kayıtlarda uç noktanızın 503 döndüğünü görüyoruz. Sunucunuzda bakım var mıydı?",
		"Evet, yük dengeleyici değişti. Düzelttik; kaçan olayları tekrar gönderebilir misiniz?",
		"Son 24 saatin olayları yeniden kuyruğa alındı.",
	}},
	{"Şifre sıfırlama e-postası ulaşmıyor", "Teknik Destek", "normal", []string{"hata"}, []string{
		"Şifremi sıfırlamak istiyorum ama e-posta gelmiyor.",
		"Spam klasörünü kontrol edebilir misiniz? Kurumsal e-posta filtreniz bizim alan adımızı engelliyor olabilir.",
		"Spam klasöründeymiş, teşekkürler.",
	}},
	{"Rezervasyon modülünde çift kayıt", "Teknik Destek", "high", []string{"hata", "entegrasyon"}, []string{
		"Kanal yöneticisinden gelen bazı rezervasyonlar sistemde iki kez görünüyor.",
		"Örnek rezervasyon numarası paylaşabilir misiniz?",
		"RZ-20481 ve RZ-20495.",
		"#Kanal yöneticisi aynı olayı iki kez gönderiyor, idempotency anahtarını kontrol etmiyoruz. Ürün talebi açıldı.",
	}},
	{"Fiyat teklifi ve kurulum süresi", "Müşteri Başarısı", "normal", []string{}, []string{
		"Kurumsal paket için 50 kullanıcılık teklif alabilir miyiz? Kurulum ne kadar sürer?",
		"Teklifi hazırlayıp e-postayla iletiyorum. Ortalama kurulum süresi veri taşıma dahil 2 hafta.",
	}},
	{"Randevu hatırlatma SMS'leri gecikiyor", "Teknik Destek", "urgent", []string{"acil", "entegrasyon"}, []string{
		"Hastalarımıza giden hatırlatma SMS'leri 2-3 saat gecikmeli gidiyor.",
		"SMS sağlayıcısında yoğunluk var, yedek sağlayıcıya geçiş yapıyoruz.",
		"#Sağlayıcı durum sayfası: kısmi kesinti. Müşteriye saatlik bilgi verelim.",
	}},
	{"Kargo entegrasyonu nasıl kurulur?", "Müşteri Başarısı", "low", []string{"entegrasyon", "eğitim"}, []string{
		"Merhaba, kargo firması entegrasyonunu nasıl açabilirim?",
		"Ayarlar > Entegrasyonlar > Kargo menüsünden API anahtarınızı girmeniz yeterli. Yardım merkezinde adım adım rehberimiz var.",
	}},
	{"Ödeme kartı değişikliği", "Faturalandırma", "normal", []string{"fatura"}, []string{
		"Şirket kartımız değişti, otomatik ödemeyi yeni karta nasıl alırız?",
		"Faturalandırma > Ödeme yöntemleri ekranından yeni kartı ekleyip varsayılan yapabilirsiniz.",
		"Tamamdır, ekledim.",
	}},
	{"Rapor ekranı çok yavaş açılıyor", "Teknik Destek", "normal", []string{"hata"}, []string{
		"Aylık satış raporu açılırken 30 saniyeden fazla bekliyoruz.",
		"Rapor tarih aralığınız 3 yıl görünüyor; önbellekli özet görünümü açtık, tekrar dener misiniz?",
	}},
	{"Instagram üzerinden ürün sorusu", "Müşteri Başarısı", "low", []string{}, []string{
		"Merhaba, mobil uygulamanız tablette de çalışıyor mu?",
		"Merhaba! Evet, iPad ve Android tabletlerde tam ekran destekleniyor.",
	}},
	{"Fatura adresi güncelleme", "Faturalandırma", "low", []string{"fatura"}, []string{
		"Şirket adresimiz değişti, faturalarda yeni adres görünsün istiyoruz.",
		"Faturalandırma profilinizi güncelledim, sonraki faturalar yeni adresle kesilecek.",
	}},
	{"Toplu kullanıcı içe aktarımı hata veriyor", "Teknik Destek", "normal", []string{"hata"}, []string{
		"CSV ile kullanıcı yüklerken 'geçersiz e-posta' hatası alıyoruz.",
		"Dosyanızda Türkçe karakterli e-posta adresleri var; bunlar desteklenmiyor. Düzelttiğiniz dosyayı tekrar yükleyebilirsiniz.",
	}},
}

type demoMacro struct {
	name, description string
	actions           []map[string]string
}

var demoMacros = []demoMacro{
	{"Fatura düzeltildi", "Düzeltilmiş faturayı bildirir, fatura etiketi ekler ve çözer", []map[string]string{
		{"type": "reply", "value": "Merhaba, düzeltilmiş faturanız oluşturuldu ve kayıtlı e-posta adresinize gönderildi. Başka bir konuda yardımcı olabilir miyiz?"},
		{"type": "tag", "value": "fatura"},
		{"type": "status", "value": "resolved"},
	}},
	{"Bilgi bekleniyor", "Müşteriden ek bilgi ister ve beklemeye alır", []map[string]string{
		{"type": "reply", "value": "Merhaba, konuyu inceleyebilmemiz için ekran görüntüsü ve hatanın oluştuğu saati paylaşabilir misiniz?"},
		{"type": "status", "value": "pending"},
	}},
	{"Acil olarak işaretle", "Önceliği acil yapar ve etiketler", []map[string]string{
		{"type": "priority", "value": "urgent"},
		{"type": "tag", "value": "acil"},
		{"type": "note", "value": "Acil olarak işaretlendi; ekip liderine bilgi verildi."},
	}},
}

type cannedReply struct{ title, shortcut, category, content string }

var demoCanned = []cannedReply{
	{"Karşılama", "selam", "Genel", "Merhaba, ClickFix destek ekibine hoş geldiniz! Size nasıl yardımcı olabilirim?"},
	{"Kapanış", "kapanis", "Genel", "Talebiniz çözüldüğü için seviniyoruz. Başka bir konuda yardıma ihtiyacınız olursa buradayız. İyi çalışmalar!"},
	{"Şifre sıfırlama", "sifre", "Hesap", "Giriş ekranındaki \"Şifremi unuttum\" bağlantısından sıfırlama e-postası isteyebilirsiniz. E-posta gelmezse spam klasörünü kontrol edin."},
	{"Fatura bilgisi", "fatura", "Faturalandırma", "Faturalarınızı Faturalandırma > Faturalar ekranından PDF olarak indirebilirsiniz."},
}

type demoArticle struct {
	collection, slug, title, body string
	tags                          []string
	public                        bool
}

type demoCollection struct{ slug, name string }

var demoCollections = []demoCollection{
	{"baslarken", "Başlarken"},
	{"faturalandirma", "Faturalandırma"},
	{"entegrasyonlar", "Entegrasyonlar"},
}

var demoArticles = []demoArticle{
	{"baslarken", "hesap-olusturma", "Hesabınızı oluşturma ve ilk adımlar",
		"## Hesap oluşturma\n\nKayıt formunu doldurduktan sonra e-posta adresinize bir doğrulama bağlantısı gönderilir. Bağlantıya tıkladığınızda kurulum sihirbazı açılır.\n\n## Ekip arkadaşlarını davet etme\n\nAyarlar > Ekip ekranından e-posta adresiyle davet gönderebilir, her kişiye rol atayabilirsiniz.\n\n## Kanalları bağlama\n\nE-posta, web sitesi sohbeti ve WhatsApp kanallarını Kanallar menüsünden ekleyin.",
		[]string{"başlangıç", "kurulum"}, true},
	{"baslarken", "sifre-sifirlama", "Şifremi unuttum, nasıl sıfırlarım?",
		"Giriş ekranında **Şifremi unuttum** bağlantısına tıklayın ve e-posta adresinizi girin. Sıfırlama bağlantısı 60 dakika geçerlidir.\n\nE-posta gelmezse spam klasörünü kontrol edin veya BT ekibinizden alan adımızın izinli listeye eklenmesini isteyin.",
		[]string{"hesap", "şifre"}, true},
	{"faturalandirma", "fatura-indirme", "Faturalarımı nereden indirebilirim?",
		"Faturalandırma > Faturalar ekranında tüm faturalarınız listelenir. Her satırdaki **PDF** düğmesiyle faturayı indirebilirsiniz. E-Fatura mükellefiyseniz faturalar GİB üzerinden de iletilir.",
		[]string{"fatura"}, true},
	{"faturalandirma", "plan-degistirme", "Planımı nasıl yükseltirim veya düşürürüm?",
		"Faturalandırma > Plan ekranından yeni planı seçin. Yükseltmeler hemen geçerli olur ve kalan gün kadar fark faturalandırılır; düşürmeler dönem sonunda uygulanır.",
		[]string{"fatura", "plan"}, true},
	{"faturalandirma", "havale-eft-odeme", "Havale / EFT ile ödeme",
		"Yıllık planlarda havale/EFT ile ödeme yapabilirsiniz. Açıklama alanına fatura numaranızı yazmanız ödemenin otomatik eşleşmesini sağlar.",
		[]string{"fatura", "ödeme"}, true},
	{"entegrasyonlar", "webhook-kurulumu", "Webhook bildirimlerini kurma",
		"Ayarlar > Geliştirici > Webhook ekranından uç nokta adresinizi ekleyin. Her istek `X-Signature` başlığıyla imzalanır; imzayı paylaşılan anahtarla doğrulayın.\n\nUç noktanız 2xx dönmezse olay 24 saat boyunca artan aralıklarla yeniden denenir.",
		[]string{"entegrasyon", "api"}, true},
	{"entegrasyonlar", "api-limitleri", "API istek limitleri",
		"Standart planda dakikada 120, kurumsal planda 600 istek sınırı vardır. Sınır aşıldığında `429` döner; `Retry-After` başlığındaki süre kadar bekleyin. Toplu işlemler için bulk uçlarını kullanın.",
		[]string{"api"}, true},
	{"entegrasyonlar", "eskalasyon-proseduru", "İç prosedür: Kurumsal müşteri eskalasyonu",
		"Kurumsal müşterilerde 2 saat içinde çözülemeyen acil talepler ekip liderine, 4 saati aşanlar teknik direktöre eskale edilir. Müşteriye saatlik durum bilgisi verilir.",
		[]string{"iç-prosedür"}, false},
}

type demoIssue struct{ title, description, status, priority string }

var demoIssues = []demoIssue{
	{"Kanal yöneticisinden gelen rezervasyonlar çift kaydediliyor", "Aynı olay iki kez geldiğinde idempotency anahtarı kontrol edilmiyor.", "in_progress", "high"},
	{"Mobil uygulamada oturum yenileme hatası (4.2.1)", "Refresh token süresi saat dilimi farkı nedeniyle erken doluyor.", "done", "urgent"},
	{"Excel dışa aktarımında filtre bilgisinin yazılması", "Müşteri talebi: aktarılan dosyanın ilk satırında uygulanan filtreler görünsün.", "backlog", "low"},
	{"Rapor ekranında büyük tarih aralıklarında yavaşlık", "3 yıllık aralıkta sorgu 30 sn sürüyor; özet tablo gerekli.", "todo", "normal"},
}
