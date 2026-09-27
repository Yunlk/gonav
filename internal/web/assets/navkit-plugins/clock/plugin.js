/* navkit 插件示例：时钟
 *
 * 这个文件运行在 sandbox="allow-scripts" 的 iframe 里（没有 allow-same-origin），
 * 所以它拿不到宿主的 DOM / localStorage / IndexedDB，只能通过 postMessage 走 RPC。
 * 它声明了 theme:read 权限，用来把自己的配色对齐当前主题。
 */
(function () {
  "use strict";

  var NS = "navkit-plugin";
  var settings = { showSeconds: true, label: "" };

  function post(payload) {
    parent.postMessage(Object.assign({ ns: NS, v: 1 }, payload), "*");
  }

  function render() {
    var now = new Date();
    var pad = function (n) { return n < 10 ? "0" + n : String(n); };
    var time = pad(now.getHours()) + ":" + pad(now.getMinutes());
    if (settings.showSeconds) time += ":" + pad(now.getSeconds());

    document.getElementById("time").textContent = time;
    document.getElementById("date").textContent =
      now.getFullYear() + " 年 " + (now.getMonth() + 1) + " 月 " + now.getDate() + " 日";
    var label = document.getElementById("label");
    label.textContent = settings.label || "";
    label.style.display = settings.label ? "" : "none";
  }

  function applyTheme(tokens) {
    if (!tokens) return;
    var root = document.documentElement.style;
    root.setProperty("--fg", tokens.fg);
    root.setProperty("--muted", tokens.muted);
    root.setProperty("--accent", tokens.accent);
  }

  window.addEventListener("message", function (event) {
    var data = event.data;
    if (!data || data.ns !== NS) return;

    if (data.type === "init") {
      // 主动取一次主题：证明权限校验链路是通的
      post({ type: "rpc", id: "theme", method: "theme.get", params: { themeId: data.themeId } });
      if (data.settings) settings = Object.assign(settings, data.settings);
      applyTheme(data.theme);
      render();
      post({ type: "log", level: "info", message: "clock ready" });
      return;
    }

    if (data.type === "settings") {
      settings = Object.assign({ showSeconds: true, label: "" }, data.settings);
      render();
      return;
    }

    if (data.type === "theme") {
      applyTheme(data.theme);
      return;
    }

    if (data.type === "rpc:result" && data.id === "theme") {
      applyTheme(data.result);
      return;
    }

    if (data.type === "rpc:error" && data.id === "theme") {
      post({ type: "error", message: "读取主题失败：" + data.error });
    }
  });

  setInterval(render, 1000);
  render();
  post({ type: "ready" });
})();
