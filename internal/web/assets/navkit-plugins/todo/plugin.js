/* navkit 插件示例：待办
 *
 * 演示 storage 权限：所有读写都经宿主 RPC，数据按 pluginId 隔离在宿主的 IndexedDB 里。
 * 插件自己既没有 localStorage 也没有 IndexedDB（沙箱是不透明源）。
 */
(function () {
  "use strict";

  var NS = "navkit-plugin";
  var seq = 0;
  var pending = {};
  var items = [];
  var settings = { title: "今天要做的事" };

  function call(method, params) {
    return new Promise(function (resolve, reject) {
      var id = "r" + ++seq;
      pending[id] = { resolve: resolve, reject: reject };
      parent.postMessage({ ns: NS, v: 1, type: "rpc", id: id, method: method, params: params || {} }, "*");
    });
  }

  function render() {
    document.getElementById("title").textContent = settings.title || "待办";
    var list = document.getElementById("list");
    list.innerHTML = "";
    document.getElementById("empty").style.display = items.length ? "none" : "";

    items.forEach(function (item) {
      var li = document.createElement("li");
      li.className = "todo__item" + (item.done ? " is-done" : "");
      li.setAttribute("data-todo-item", item.id);

      var box = document.createElement("input");
      box.type = "checkbox";
      box.checked = Boolean(item.done);
      box.addEventListener("change", function () {
        item.done = box.checked;
        void save();
      });

      var text = document.createElement("span");
      text.textContent = item.text;

      var del = document.createElement("button");
      del.type = "button";
      del.className = "todo__del";
      del.textContent = "×";
      del.addEventListener("click", function () {
        items = items.filter(function (entry) { return entry.id !== item.id; });
        void save();
      });

      li.appendChild(box);
      li.appendChild(text);
      li.appendChild(del);
      list.appendChild(li);
    });

    // 让宿主的格子跟着内容长一点（上限由宿主的行数决定）
    parent.postMessage({ ns: NS, v: 1, type: "resize", height: document.body.scrollHeight }, "*");
  }

  function save() {
    render();
    return call("storage.set", { key: "items", value: items }).catch(function (err) {
      parent.postMessage({ ns: NS, v: 1, type: "error", message: String(err && err.message) });
    });
  }

  function add(text) {
    var value = String(text || "").trim();
    if (!value) return;
    items.push({ id: "t" + Date.now() + Math.random().toString(36).slice(2, 6), text: value, done: false });
    void save();
  }

  document.getElementById("form").addEventListener("submit", function (event) {
    event.preventDefault();
    var input = document.getElementById("input");
    add(input.value);
    input.value = "";
  });

  window.addEventListener("message", function (event) {
    var data = event.data;
    if (!data || data.ns !== NS) return;

    if (data.type === "rpc:result" && pending[data.id]) {
      pending[data.id].resolve(data.result);
      delete pending[data.id];
      return;
    }
    if (data.type === "rpc:error" && pending[data.id]) {
      pending[data.id].reject(new Error(data.error));
      delete pending[data.id];
      return;
    }

    if (data.type === "init") {
      if (data.settings) settings = Object.assign(settings, data.settings);
      var root = document.documentElement.style;
      if (data.theme) {
        root.setProperty("--fg", data.theme.fg);
        root.setProperty("--muted", data.theme.muted);
        root.setProperty("--rule", data.theme.rule);
        root.setProperty("--accent", data.theme.accent);
        root.setProperty("--surface", data.theme.surfaceStrong);
      }
      call("storage.get", { key: "items" })
        .then(function (value) {
          items = Array.isArray(value) ? value : [];
          render();
          parent.postMessage({ ns: NS, v: 1, type: "log", level: "info", message: "todo ready, " + items.length + " items" });
        })
        .catch(function (err) {
          parent.postMessage({ ns: NS, v: 1, type: "error", message: String(err && err.message) });
        });
      return;
    }

    if (data.type === "settings") {
      settings = Object.assign({ title: "今天要做的事" }, data.settings);
      render();
    }
  });

  render();
  parent.postMessage({ ns: NS, v: 1, type: "ready" });
})();
