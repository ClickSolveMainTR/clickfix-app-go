// Adım 2: kişi ve şirket için ayrı dış kimlik. user_id kişinin, company.id şirketin.
// @ts-check
ClickFix("identify", {
  user_id: "1001", // API: contacts.external_id
  user_hash: "SUNUCUDA_HESAPLANAN",
  company: { id: "F-77", name: "Acme A.Ş.", domain: "acme.com.tr" }, // API: companies.external_id
});
ClickFix("on", "identify", ({ verified }) =>
  console.log("doğrulandı:", verified),
);
