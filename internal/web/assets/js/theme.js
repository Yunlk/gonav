/* gonav · 主题初始化：默认浅色，跟随系统并记忆。必须同步执行，否则刷新时会闪烁。 */
(function () {
  "use strict";

  var KEY = "gonav-theme";
  var stored = null;
  try {
    stored = window.localStorage.getItem(KEY);
  } catch (err) {
    stored = null;
  }

  var prefersDark =
    window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  var theme = stored === "light" || stored === "dark" ? stored : prefersDark ? "dark" : "light";

  document.documentElement.setAttribute("data-theme", theme);

  window.GONAV_THEME = {
    key: KEY,
    get: function () {
      return document.documentElement.getAttribute("data-theme") || "light";
    },
    set: function (next) {
      document.documentElement.setAttribute("data-theme", next);
      try {
        window.localStorage.setItem(KEY, next);
      } catch (err) {
        /* 隐私模式下忽略 */
      }
    }
  };
})();
