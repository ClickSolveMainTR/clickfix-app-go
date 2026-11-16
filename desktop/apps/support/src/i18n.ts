// TR/EN metinler. Dil sistem dilinden seçilir (tr* → Türkçe, diğerleri →
// İngilizce). Metinler onaylı tasarımdan (Claude Design kanvası, CF-231) ve
// `docs/design/uzaktan-destek-metinleri.md` taslağından alınmıştır.
//
// Türkçe ek sorunu: kişi adlarına ek getirilmez ("Elif'in" yerine "Elif bu
// oturumda ..."), çünkü ünlü uyumu ada göre değişir.

export type Lang = "tr" | "en";

const tr = {
  appName: "ClickFix Destek",
  badge: "Destek",
  version: "v{v}",
  privacy: "Gizlilik",
  help: "Yardım",
  warning:
    "ClickFix çalışanları ve işletmeler sizden asla para transferi, banka girişi veya kart bilgisi istemez.",
  stepIdentity: "Kimlik",
  stepView: "Görme",
  stepControl: "Kontrol",
  stepRecording: "Kayıt",

  // Kod girişi
  codeEyebrow: "Uzaktan destek",
  codeTitle: "Temsilcinizin verdiği\nkodu girin",
  codeLead:
    "Kod, yazıştığınız işletmenin temsilcisi tarafından konuşmaya gönderildi. 15 dakika geçerlidir ve yalnız bir kez kullanılır.",
  codeLabel: "9 haneli kod",
  digitLabel: "{n}. hane",
  codeLinkHint: "Bağlantıyla açtıysanız kod kendiliğinden dolar.",
  paste: "Panodan yapıştır",
  pasteNone: "Panoda 9 haneli bir kod yok.",
  pasteBlocked: "Panoya erişilemedi. Kodu ⌘V / Ctrl+V ile yapıştırın.",
  continue: "Devam et",
  resolving: "Kod doğrulanıyor…",
  resolvingLink: "Bağlantı doğrulanıyor…",
  scamStrong: "Telefonla aranıp kod istendiyse devam etmeyin.",

  // Hatalar
  errCodeInvalid:
    "Kod geçersiz, kullanılmış ya da süresi dolmuş. Temsilcinizden yeni bir kod isteyin.",
  errRateLimited: "Çok fazla deneme yapıldı. Birkaç dakika sonra yeniden deneyin.",
  errDisabled: "Bu işletmenin uzaktan desteği şu an kapalı.",
  errEnded: "Bu oturum bitti. Temsilcinizden yeni bir kod isteyin.",
  errNetwork: "ClickFix sunucusuna ulaşılamıyor. İnternet bağlantınızı denetleyin.",
  errGeneric: "Beklenmeyen bir hata oluştu. Yeniden deneyin.",
  errSidecar: "Yardımcı süreç yanıt vermiyor. Uygulamayı kapatıp yeniden açın.",
  errUnsupported: "Ekran paylaşımı bu bilgisayarda henüz desteklenmiyor.",
  errCapture: "Ekran paylaşımı başlatılamadı. Yeniden deneyin.",

  // Kimlik
  idTitle: "Size kim bağlanmak istiyor?",
  idLead:
    "Bu bilgiler ClickFix tarafından doğrulandı. Tanıdığınız işletme değilse devam etmeyin.",
  verified: "Doğrulanmış işletme",
  unverified: "Doğrulanmamış işletme",
  agentRole: "Destek temsilcisi",
  conversation: "Konuşma",
  requested: "İstenen",
  chipView: "Ekranı görme",
  chipControl: "Kontrol",
  chipRecording: "Kayıt",
  cancel: "Vazgeç",
  idConfirm: "Evet, bu işletmeyle yazışıyorum",
  reportLink: "Bu bir dolandırıcılık, bildir",
  reportTitle: "Bu oturumu siz mi istediniz?",
  reportBody:
    "Bu kodu yazıştığınız işletmeden almadıysanız ya da biri sizi telefonla arayıp istediyse bildirin. Oturum hemen biter ve işletme incelemeye alınır.",
  reportConfirm: "Bu oturumu ben istemedim, bildir",
  reportBack: "Geri dön",

  // Görme onayı
  viewTitle: "{agent} ekranınızı görebilir mi?",
  viewSees: "Görecek: birincil ekranınızın tamamı",
  viewNotShared:
    "Ses, kamera, pano ve dosyalar paylaşılmaz. {agent} bu adımda hiçbir şeye tıklayamaz.",
  notice: "Aydınlatma metni",
  noticeBody:
    "{org}, destek talebinizi çözmek için bu oturum boyunca ekran görüntünüzü işler. Görüntü ClickFix sunucuları üzerinden aktarılır ve kayıt için ayrıca onayınız istenir. [Hukuki onaylı metin buraya gelecek]",
  viewConsent:
    "Ekran görüntümün **bu oturum süresince** {org} ile paylaşılmasına açık rıza veriyorum.",
  viewDuration: "En çok {min} dakika. İstediğiniz an **Durdur**’a basabilirsiniz.",
  decline: "Reddet",
  viewAccept: "Ekranımı göster",

  // Kontrol ve kayıt
  liveSeeing: "{agent} ekranınızı görüyor · {time}",
  controlTitle: "{agent} fare ve klavyenizi kullanmak istiyor",
  recordingTitle: "{org} oturumu kaydetmek istiyor",
  can: "Yapabilir",
  canBody: "Tıklama, yazma, kaydırma, pencere açma",
  cannot: "Yapamaz",
  cannotBody: "Sistem kısayolları, pano, dosya aktarımı",
  shortcutHint: "her an oturumu bitirir",
  controlConsent:
    "{agent} bu oturumda fare ve klavyemi kullanabilir. İstediğim an geri alabilirim.",
  recordingConsent: "Oturum kaydedilsin",
  optional: "İsteğe bağlı",
  recordingHint: "İşletme kaydı {days} gün saklar. İşaretlemezseniz oturum kayıtsız sürer.",
  recordingHintNoDays: "İşaretlemezseniz oturum kayıtsız sürer.",
  viewOnly: "Yalnız izlesin",
  allowControl: "Kontrole izin ver",

  // macOS izinleri
  permScreen: "Ekran ve Sistem Ses Kaydı",
  permScreenWhy: "{agent} ekranınızı görebilsin diye",
  permScreenWhyGeneric: "Temsilciniz ekranınızı görebilsin diye",
  permAx: "Erişilebilirlik",
  permAxWhy: "Yalnız kontrol izni verdiyseniz",
  permGranted: "Verildi ✓",
  permWaiting: "Bekleniyor",
  permNext: "Sırada",
  permSkipped: "Gerekmiyor",
  permScreenTitle: "macOS ekranınızı paylaşmak için izin istiyor",
  permScreenLead:
    "Bu izin yalnız ClickFix Destek içindir ve oturum bitince kimse ekranınızı göremez.",
  permAxTitle: "Kontrol için bir izin daha",
  permAxLead:
    "{agent} fare ve klavyeyi kullanabilsin diye macOS Erişilebilirlik iznini ister. Kontrolü vermediyseniz bu adımı atlayabilirsiniz.",
  permPath: "Gizlilik ve Güvenlik › {pane}",
  permOtherApp: "Toplantı Uygulaması",
  permStep1: "Aşağıdaki “Sistem Ayarları’nı aç” düğmesine basın.",
  permStep2: "Listede ClickFix Destek’i bulun ve yanındaki anahtarı açın.",
  permStep3Screen: "“Çık ve Yeniden Aç”a basın. Uygulama kaldığı adımdan devam eder.",
  permStep3Ax: "Parolanızı ya da Touch ID’yi onaylayın.",
  permMonthly:
    "macOS bu izni ayda bir yeniden sorabilir; bu normaldir. Uygulama izin verilince kaldığı yerden devam eder.",
  permRecheck: "Yeniden denetle",
  permRestart: "Yeniden başlat",
  permOpen: "Sistem Ayarları’nı aç",
  permStillMissing: "İzin henüz görünmüyor. Anahtarı açtıysanız uygulamayı yeniden başlatın.",
  permSkipControl: "Kontrolsüz devam et",

  // Paylaşım
  connecting: "Bağlanıyor…",
  frameTab: "ClickFix · ekranınız paylaşılıyor",
  barSeeing: "{agent} ekranınızı görüyor",
  barControlling: "{agent} ekranınızı kontrol ediyor",
  barVerified: "{org} · doğrulanmış",
  chipAgentControl: "Kontrol temsilcide",
  chipViewOnly: "Yalnız izleme",
  revoke: "Kontrolü geri al",
  stop: "Durdur",
  barHintMac: "⌃⌥⇧S oturumu hemen bitirir · çubuğu sürükleyebilirsiniz",
  barHintWin: "Ctrl+Alt+Shift+S oturumu hemen bitirir · çubuğu sürükleyebilirsiniz",
  barDrag: "Çubuğu taşı",
  sharingTitle: "Ekranınız paylaşılıyor",
  sharingLead:
    "Ekranınızın kenarındaki yeşil çerçeve paylaşım sürdükçe görünür. Durdurmak için çubuktaki Durdur düğmesine ya da kısayola basın.",
  controlRevokedByAgent: "{agent} kontrolü bıraktı. Ekranınızı görmeye devam ediyor.",
  controlRevokedByYou: "Kontrolü geri aldınız. {agent} yalnız izliyor.",

  // Bitiş
  endedTitle: "Oturum bitti",
  endedBody:
    "Ekran paylaşımı kapandı. ClickFix Destek artık ekranınızı göremez ve fare ya da klavyenizi kullanamaz.",
  notStartedTitle: "Oturum başlamadı",
  notStartedBody: "Ekranınız paylaşılmadı. Bu pencereyi kapatabilirsiniz.",
  reportedTitle: "Bildiriminiz alındı",
  reportedBody:
    "Oturum bitti ve kimse ekranınızı göremez. ClickFix bu işletmeyi inceleyecek. Sizi telefonla arayan biri varsa telefonu kapatın.",
  rowAgent: "Temsilci",
  rowDuration: "Süre",
  rowControl: "Kontrol",
  rowRecording: "Kayıt",
  rowEndedBy: "Bitiren",
  duration: "{m} dk {s} sn",
  controlNone: "Verilmedi",
  controlYouRevoked: "{m} dk, siz geri aldınız",
  controlAgentRevoked: "{m} dk, temsilci bıraktı",
  controlUsed: "{m} dk",
  recordingNo: "Yapılmadı",
  recordingYes: "Yapıldı",
  recordingYesDays: "Yapıldı, {days} gün saklanır",
  byYouStop: "Siz (Durdur)",
  byYouEmergency: "Siz (acil kısayol)",
  byYou: "Siz",
  byAgent: "Temsilci",
  byTimeout: "Süre doldu",
  byReported: "Siz (bildirim)",
  bySystem: "Sistem",
  byConnection: "Bağlantı koptu",
  endedUninstall: "Uygulamayı silebilirsiniz. Bir dahaki desteğe yeniden indirmeniz yeter.",
  close: "Kapat",
  endedReport: "Bir sorun mu vardı? Bildir",

  // Önizleme paneli (yalnız sahte IPC)
  devTitle: "Önizleme (sahte IPC)",
  devTheme: "Tema",
  devLang: "Dil",
  devPlatform: "Platform",
  devAgentEnds: "Temsilci bitirsin",
  devAgentReleases: "Temsilci kontrolü bıraksın",
  devRevokeScreen: "Ekran iznini geri al",
  devCodes: "Kodlar: 000… geçersiz · 111… izleme · 222… kontrol · diğerleri kontrol + kayıt",
  system: "Sistem",
  light: "Açık",
  dark: "Koyu",
};

export type Key = keyof typeof tr;

const en: Record<Key, string> = {
  appName: "ClickFix Support",
  badge: "Support",
  version: "v{v}",
  privacy: "Privacy",
  help: "Help",
  warning:
    "ClickFix staff and businesses will never ask you for a money transfer, bank login or card details.",
  stepIdentity: "Identity",
  stepView: "View",
  stepControl: "Control",
  stepRecording: "Recording",

  codeEyebrow: "Remote support",
  codeTitle: "Enter the code\nfrom your agent",
  codeLead:
    "The agent of the business you are chatting with sent this code in the conversation. It is valid for 15 minutes and works only once.",
  codeLabel: "9-digit code",
  digitLabel: "Digit {n}",
  codeLinkHint: "If you opened a link, the code fills in by itself.",
  paste: "Paste from clipboard",
  pasteNone: "There is no 9-digit code on the clipboard.",
  pasteBlocked: "The clipboard is not available. Paste the code with ⌘V / Ctrl+V.",
  continue: "Continue",
  resolving: "Checking the code…",
  resolvingLink: "Checking the link…",
  scamStrong: "If someone phoned you and asked for this code, stop here.",

  errCodeInvalid:
    "The code is invalid, already used or expired. Ask your agent for a new code.",
  errRateLimited: "Too many attempts. Try again in a few minutes.",
  errDisabled: "Remote support is turned off for this business right now.",
  errEnded: "This session has ended. Ask your agent for a new code.",
  errNetwork: "Cannot reach the ClickFix server. Check your internet connection.",
  errGeneric: "Something went wrong. Try again.",
  errSidecar: "The helper process is not responding. Close and reopen the app.",
  errUnsupported: "Screen sharing is not supported on this computer yet.",
  errCapture: "Screen sharing could not start. Try again.",

  idTitle: "Who wants to connect to you?",
  idLead:
    "ClickFix verified these details. If you do not know this business, do not continue.",
  verified: "Verified business",
  unverified: "Unverified business",
  agentRole: "Support agent",
  conversation: "Conversation",
  requested: "Requested",
  chipView: "View screen",
  chipControl: "Control",
  chipRecording: "Recording",
  cancel: "Cancel",
  idConfirm: "Yes, I am chatting with this business",
  reportLink: "This is a scam, report it",
  reportTitle: "Did you ask for this session?",
  reportBody:
    "Report it if you did not get this code from the business you are chatting with, or if someone phoned you and asked for it. The session ends at once and the business is reviewed.",
  reportConfirm: "I did not ask for this session, report",
  reportBack: "Go back",

  viewTitle: "Can {agent} see your screen?",
  viewSees: "Will see: your whole main screen",
  viewNotShared:
    "Audio, camera, clipboard and files are not shared. {agent} cannot click anything at this step.",
  notice: "Privacy notice",
  noticeBody:
    "{org} processes the image of your screen during this session to solve your support request. The image passes through ClickFix servers, and a recording needs your separate consent. [Legally approved text goes here]",
  viewConsent:
    "I explicitly consent to my screen being shared with {org} **for this session only**.",
  viewDuration: "At most {min} minutes. You can press **Stop** at any time.",
  decline: "Decline",
  viewAccept: "Show my screen",

  liveSeeing: "{agent} can see your screen · {time}",
  controlTitle: "{agent} wants to use your mouse and keyboard",
  recordingTitle: "{org} wants to record the session",
  can: "Can",
  canBody: "Click, type, scroll, open windows",
  cannot: "Cannot",
  cannotBody: "System shortcuts, clipboard, file transfer",
  shortcutHint: "ends the session at any time",
  controlConsent:
    "{agent} may use my mouse and keyboard during this session. I can take it back at any time.",
  recordingConsent: "Record the session",
  optional: "Optional",
  recordingHint:
    "The business keeps the recording for {days} days. If you leave this unticked, the session is not recorded.",
  recordingHintNoDays: "If you leave this unticked, the session is not recorded.",
  viewOnly: "View only",
  allowControl: "Allow control",

  permScreen: "Screen & System Audio Recording",
  permScreenWhy: "So {agent} can see your screen",
  permScreenWhyGeneric: "So your agent can see your screen",
  permAx: "Accessibility",
  permAxWhy: "Only if you allowed control",
  permGranted: "Granted ✓",
  permWaiting: "Waiting",
  permNext: "Next",
  permSkipped: "Not needed",
  permScreenTitle: "macOS asks for permission to share your screen",
  permScreenLead:
    "This permission is only for ClickFix Support, and no one can see your screen once the session ends.",
  permAxTitle: "One more permission for control",
  permAxLead:
    "macOS asks for the Accessibility permission so {agent} can use the mouse and keyboard. If you did not allow control, you can skip this step.",
  permPath: "Privacy & Security › {pane}",
  permOtherApp: "Meeting app",
  permStep1: "Press “Open System Settings” below.",
  permStep2: "Find ClickFix Support in the list and turn on its switch.",
  permStep3Screen: "Press “Quit & Reopen”. The app continues where it left off.",
  permStep3Ax: "Confirm with your password or Touch ID.",
  permMonthly:
    "macOS may ask for this permission again once a month; that is normal. The app continues where it left off once you allow it.",
  permRecheck: "Check again",
  permRestart: "Restart",
  permOpen: "Open System Settings",
  permStillMissing:
    "The permission does not show yet. If you turned the switch on, restart the app.",
  permSkipControl: "Continue without control",

  connecting: "Connecting…",
  frameTab: "ClickFix · your screen is shared",
  barSeeing: "{agent} can see your screen",
  barControlling: "{agent} is controlling your screen",
  barVerified: "{org} · verified",
  chipAgentControl: "Agent has control",
  chipViewOnly: "View only",
  revoke: "Take back control",
  stop: "Stop",
  barHintMac: "⌃⌥⇧S ends the session at once · you can drag the bar",
  barHintWin: "Ctrl+Alt+Shift+S ends the session at once · you can drag the bar",
  barDrag: "Move the bar",
  sharingTitle: "Your screen is shared",
  sharingLead:
    "The green frame around your screen stays while sharing lasts. To stop, press Stop on the bar or use the shortcut.",
  controlRevokedByAgent: "{agent} released control and can still see your screen.",
  controlRevokedByYou: "You took back control. {agent} can only watch.",

  endedTitle: "Session ended",
  endedBody:
    "Screen sharing is off. ClickFix Support can no longer see your screen or use your mouse and keyboard.",
  notStartedTitle: "The session did not start",
  notStartedBody: "Your screen was not shared. You can close this window.",
  reportedTitle: "Thanks for reporting",
  reportedBody:
    "The session has ended and no one can see your screen. ClickFix will review this business. If someone phoned you, hang up.",
  rowAgent: "Agent",
  rowDuration: "Duration",
  rowControl: "Control",
  rowRecording: "Recording",
  rowEndedBy: "Ended by",
  duration: "{m} min {s} s",
  controlNone: "Not given",
  controlYouRevoked: "{m} min, you took it back",
  controlAgentRevoked: "{m} min, agent released it",
  controlUsed: "{m} min",
  recordingNo: "Not recorded",
  recordingYes: "Recorded",
  recordingYesDays: "Recorded, kept for {days} days",
  byYouStop: "You (Stop)",
  byYouEmergency: "You (emergency shortcut)",
  byYou: "You",
  byAgent: "Agent",
  byTimeout: "Time limit",
  byReported: "You (report)",
  bySystem: "System",
  byConnection: "Connection lost",
  endedUninstall: "You can delete the app. Download it again next time you need support.",
  close: "Close",
  endedReport: "Was something wrong? Report",

  devTitle: "Preview (mock IPC)",
  devTheme: "Theme",
  devLang: "Language",
  devPlatform: "Platform",
  devAgentEnds: "Agent ends",
  devAgentReleases: "Agent releases control",
  devRevokeScreen: "Revoke screen permission",
  devCodes: "Codes: 000… invalid · 111… view · 222… control · others control + recording",
  system: "System",
  light: "Light",
  dark: "Dark",
};

const dicts: Record<Lang, Record<Key, string>> = { tr, en };

export function systemLang(): Lang {
  const langs = navigator.languages?.length ? navigator.languages : [navigator.language];
  return langs[0]?.toLowerCase().startsWith("tr") ? "tr" : "en";
}

export type T = (key: Key, vars?: Record<string, string | number>) => string;

export function translator(lang: Lang): T {
  const d = dicts[lang];
  return (key, vars) => {
    let s = d[key];
    if (vars) {
      for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v));
    }
    return s;
  };
}

/** Ad soyaddan ilk ad ("Elif Kaya" → "Elif"). */
export function firstName(full: string): string {
  return full.trim().split(/\s+/)[0] ?? full;
}

/** Ad soyaddan baş harfler ("Elif Kaya" → "EK"). */
export function initials(full: string): string {
  const parts = full.trim().split(/\s+/).filter(Boolean);
  const first = parts[0]?.[0] ?? "";
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return (first + last).toLocaleUpperCase();
}
