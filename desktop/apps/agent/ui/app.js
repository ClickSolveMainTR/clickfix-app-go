// Yerel yükleme/giriş/hata sayfası. Tauri dışında (ör. tarayıcıda açılırsa)
// hiçbir şey yapmaz. Yalnız `connect`, `start_login` ve `cancel_login`
// komutlarına izin vardır (capabilities/local.json).
//
// Rust tarafı bu sayfayı `?view=signed-out` (oturum yok, çıkış yapıldı) ya da
// `?view=error&message=...` (giriş/oturum kurulamadı) ile yeniden açar.
(function () {
  "use strict";

  var views = ["connecting", "offline", "signed-out", "waiting", "error"];
  var tauri = window.__TAURI_INTERNALS__;

  function el(id) {
    return document.getElementById(id);
  }

  function show(view, message) {
    views.forEach(function (v) {
      el(v).hidden = v !== view;
    });
    if (view === "offline") el("offline-detail").textContent = message || "";
    if (view === "error") el("error-detail").textContent = message || "";
  }

  function invoke(cmd) {
    if (!tauri) {
      return Promise.reject("Bu sayfa yalnız ClickFix Masaüstü içinde çalışır.");
    }
    return tauri.invoke(cmd);
  }

  function connect() {
    show("connecting");
    // Oturum varsa Rust pencereyi uygulama köküne götürür ve bu sayfa kapanır.
    invoke("connect")
      .then(function (res) {
        if (res && res.status === "signed_out") show("signed-out");
      })
      .catch(function (err) {
        show("offline", String(err));
      });
  }

  function signIn() {
    invoke("start_login")
      .then(function () {
        show("waiting");
      })
      .catch(function (err) {
        show("error", String(err));
      });
  }

  function cancel() {
    invoke("cancel_login").finally(function () {
      show("signed-out");
    });
  }

  el("retry").addEventListener("click", connect);
  el("error-retry").addEventListener("click", connect);
  el("sign-in").addEventListener("click", signIn);
  el("error-sign-in").addEventListener("click", signIn);
  el("reopen").addEventListener("click", signIn);
  el("cancel").addEventListener("click", cancel);
  window.addEventListener("online", function () {
    if (!el("offline").hidden) connect();
  });

  var params = new URLSearchParams(window.location.search);
  var view = params.get("view");
  if (view === "signed-out") {
    show("signed-out");
  } else if (view === "error") {
    show("error", params.get("message") || "");
  } else {
    connect();
  }
})();
