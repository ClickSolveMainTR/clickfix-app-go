// Adım 1: sunucunuzdan aldığınız user_hash ile giriş yapmış kullanıcıyı tanıtın.
// @ts-check
ClickFix("boot", { widgetId: "WIDGET_ID", locale: "tr" });

fetch("/api/support-session") // sizin ucunuz: { user_id, user_hash } döner
  .then((r) => r.json())
  .then((session) =>
    ClickFix("identify", {
      user_id: session.user_id, // kişinin dış kimliği
      user_hash: session.user_hash, // hex(HMAC-SHA256(anahtar, user_id)), sunucuda
      name: "Ayşe Yılmaz",
      email: "ayse@example.com",
    }),
  );

ClickFix("on", "error", (e) => {
  if (e.code === "WIDGET_IDENTITY_INVALID")
    console.warn("user_hash eşleşmedi:", e.message);
});
