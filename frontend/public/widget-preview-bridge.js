// Mirrors the editor's light/dark preview choice on the fake page behind
// the widget. The widget itself receives its state through widget.js.
window.addEventListener("message", function (e) {
  if (e.origin !== window.location.origin) return;
  var d = e.data;
  if (!d || d.type !== "clickfix:preview" || !d.state) return;
  var theme = d.state.theme;
  document.documentElement.className =
    theme === "dark" || theme === "light" ? theme : "";
  document.documentElement.dir = d.state.dir === "rtl" ? "rtl" : "ltr";
});
