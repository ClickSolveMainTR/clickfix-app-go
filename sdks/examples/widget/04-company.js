// Adım 4: şirket identify ile oluşur ya da dış kimliğiyle eşleşir (ikinci anahtar: domain).
// @ts-check
ClickFix("boot", {
  widgetId: "WIDGET_ID",
  user: {
    user_id: "1001",
    user_hash: "SUNUCUDA_HESAPLANAN",
    company: { id: "F-77", name: "Acme A.Ş.", domain: "acme.com.tr" },
  },
});
