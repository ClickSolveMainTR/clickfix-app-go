// Yerel yükleme/hata sayfası. Tauri dışında (ör. tarayıcıda açılırsa) hiçbir
// şey yapmaz. Yalnız `connect` komutuna izin vardır (capabilities/local.json).
(function () {
  "use strict";

  var connecting = document.getElementById("connecting");
  var offline = document.getElementById("offline");
  var detail = document.getElementById("detail");
  var retry = document.getElementById("retry");
  var tauri = window.__TAURI_INTERNALS__;

  function show(state, message) {
    connecting.hidden = state !== "connecting";
    offline.hidden = state !== "offline";
    detail.textContent = message || "";
    retry.disabled = state === "connecting";
  }

  function connect() {
    show("connecting");
    if (!tauri) {
      show("offline", "Bu sayfa yalnız ClickFix Masaüstü içinde çalışır.");
      return;
    }
    // Başarılıysa pencere uygulama köküne gider ve bu sayfa kapanır.
    tauri.invoke("connect").catch(function (err) {
      show("offline", String(err));
    });
  }

  retry.addEventListener("click", connect);
  window.addEventListener("online", connect);
  connect();
})();
