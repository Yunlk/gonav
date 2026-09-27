/* =========================================================================
   gonav · DNUI 校内导航 — 交互层
   实时筛选（含拼音）/ 收藏（拖拽排序）/ 最近使用 / 主题 / 快捷键 / 时钟。
   无第三方依赖，无入场与滚动动画；所有样式改动走 CSSOM，兼容严格 CSP。
   ========================================================================= */
(function () {
  "use strict";

  var $ = function (sel, root) { return (root || document).querySelector(sel); };
  var $$ = function (sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  };

  var FAV_KEY = "gonav-favs";
  var RECENT_KEY = "gonav-recent";
  var RECENT_MAX = 8;

  var GRIP_SVG =
    '<svg class="ic" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><rect x="7" y="6" width="3" height="3"/><rect x="14" y="6" width="3" height="3"/><rect x="7" y="11" width="3" height="3"/><rect x="14" y="11" width="3" height="3"/><rect x="7" y="16" width="3" height="3"/><rect x="14" y="16" width="3" height="3"/></svg>';

  /* ---------- 小工具 ---------------------------------------------------- */

  function toast(message) {
    var el = $("[data-toast]");
    if (!el) return;
    el.textContent = message;
    el.classList.add("is-visible");
    window.clearTimeout(el._timer);
    el._timer = window.setTimeout(function () { el.classList.remove("is-visible"); }, 2000);
  }

  function readJSON(key, fallback) {
    try {
      var raw = window.localStorage.getItem(key);
      if (!raw) return fallback;
      var parsed = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed : fallback;
    } catch (err) {
      return fallback;
    }
  }

  function writeJSON(key, value) {
    try { window.localStorage.setItem(key, JSON.stringify(value)); } catch (err) { /* 忽略 */ }
  }

  function escapeHTML(text) {
    return String(text).replace(/[&<>"']/g, function (ch) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch];
    });
  }

  // 把命中的关键词包成 <mark>，原文保存在 data-raw 以便反复筛选
  function markElement(el, terms) {
    if (!el) return;
    if (el.dataset.raw === undefined) el.dataset.raw = el.textContent;

    var raw = el.dataset.raw;
    if (!terms.length || !raw) {
      if (el.innerHTML !== raw) el.textContent = raw;
      return;
    }

    var lower = raw.toLowerCase();
    var html = "";
    var pos = 0;
    while (pos < raw.length) {
      var best = -1;
      var bestLen = 0;
      for (var i = 0; i < terms.length; i++) {
        var at = lower.indexOf(terms[i], pos);
        if (at < 0) continue;
        if (best < 0 || at < best || (at === best && terms[i].length > bestLen)) {
          best = at;
          bestLen = terms[i].length;
        }
      }
      if (best < 0) {
        html += escapeHTML(raw.slice(pos));
        break;
      }
      html += escapeHTML(raw.slice(pos, best));
      html += '<mark class="hl">' + escapeHTML(raw.substr(best, bestLen)) + "</mark>";
      pos = best + bestLen;
    }
    el.innerHTML = html;
  }

  function termList(text) {
    return (text || "").split(/\s+/).filter(function (term) {
      return term && !(term.length < 2 && /^[a-z]+$/.test(term));
    });
  }

  /* ---------- 搜索框 ---------------------------------------------------- */

  function initSearch() {
    var form = $("[data-search-form]");
    if (!form) return;

    var select = $("[data-engine]", form);
    var input = $("[data-search-input]", form);

    function syncPlaceholder() {
      if (!select || !input) return;
      var opt = select.options[select.selectedIndex];
      if (opt && opt.dataset.placeholder) input.setAttribute("placeholder", opt.dataset.placeholder);
    }

    if (select) {
      syncPlaceholder();
      select.addEventListener("change", syncPlaceholder);
    }

    if (input) {
      input.addEventListener("keydown", function (event) {
        if ((event.ctrlKey || event.metaKey) && event.key === "Enter" && select) {
          event.preventDefault();
          select.value = "internal";
          syncPlaceholder();
          form.submit();
        }
      });
    }

    form.addEventListener("submit", function (event) {
      if (!input || input.value.trim() === "") {
        event.preventDefault();
        toast("先输入关键词再搜索");
        if (input) input.focus();
      }
    });
  }

  /* ---------- 实时筛选（拼音已在服务端写入 data-search） ---------------- */

  function initFilter() {
    var grid = $("[data-grid]");
    if (!grid) return;

    var input = $("[data-search-input]");
    var chips = $$("[data-chips] .chip");
    var empty = $("[data-empty]");
    var cards = $$("[data-card]", grid);
    var navLinks = $$(".nav a");
    var state = { category: "all", text: "" };

    function matches(item, terms) {
      var haystack = (item.getAttribute("data-search") || "").toLowerCase();
      for (var i = 0; i < terms.length; i++) {
        if (haystack.indexOf(terms[i]) === -1) return false;
      }
      return true;
    }

    function apply() {
      var terms = state.text ? state.text.split(/\s+/).filter(Boolean) : [];
      var marks = termList(state.text);
      var total = 0;

      cards.forEach(function (card) {
        var inCategory = state.category === "all" || card.getAttribute("data-category") === state.category;
        var visible = 0;

        $$("[data-item]", card).forEach(function (item) {
          var show = inCategory && (terms.length === 0 || matches(item, terms));
          item.hidden = !show;
          if (show) visible++;
          markElement($(".name-text", item), show ? marks : []);
          markElement($(".link-desc", item), show ? marks : []);
        });

        card.hidden = visible === 0;
        total += visible;
      });

      if (empty) empty.hidden = total !== 0;
    }

    if (input) {
      var onInput = function () {
        state.text = input.value.trim().toLowerCase();
        apply();
      };
      input.addEventListener("input", onInput);
      input.addEventListener("search", onInput);
    }

    chips.forEach(function (chip) {
      chip.addEventListener("click", function () {
        chips.forEach(function (c) { c.classList.toggle("is-active", c === chip); });
        state.category = chip.getAttribute("data-filter") || "all";
        apply();
      });
    });

    // 顶栏分类导航：点「图书资源」等价于点亮对应 chip
    navLinks.forEach(function (link) {
      link.addEventListener("click", function () {
        var id = (link.getAttribute("href") || "").replace("#cat-", "");
        var target = chips.filter(function (c) { return c.getAttribute("data-filter") === id; })[0];
        if (target) target.click();
      });
    });

    document.addEventListener("gonav:clear-filter", function () {
      state.text = "";
      state.category = "all";
      chips.forEach(function (c) { c.classList.toggle("is-active", c.getAttribute("data-filter") === "all"); });
      if (input) input.value = "";
      apply();
    });

    // 示例词：填充搜索框并立即筛选
    $$("[data-example]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        if (!input) return;
        input.value = btn.getAttribute("data-example") || "";
        state.text = input.value.trim().toLowerCase();
        apply();
        input.focus();
      });
    });

    apply();
  }

  /* ---------- 收藏夹 ---------------------------------------------------- */

  function initFavorites() {
    var strip = $("[data-fav-strip]");
    var body = $("[data-fav-body]");
    if (!strip || !body) return;

    var items = readJSON(FAV_KEY, []).filter(function (item) { return item && item.id; });
    var dragId = null;

    function indexOf(id) {
      for (var i = 0; i < items.length; i++) {
        if (items[i].id === id) return i;
      }
      return -1;
    }

    function save() {
      writeJSON(FAV_KEY, items);
      sync();
    }

    function sync() {
      $$("[data-fav]").forEach(function (btn) {
        var on = indexOf(btn.getAttribute("data-fav")) >= 0;
        btn.classList.toggle("is-on", on);
        btn.setAttribute("aria-pressed", on ? "true" : "false");
      });
    }

    function miniButton(label, attr, value, title) {
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "mini-btn";
      btn.textContent = label;
      btn.setAttribute(attr, value);
      if (title) btn.title = title;
      return btn;
    }

    function render() {
      body.textContent = "";
      strip.hidden = items.length === 0;

      items.forEach(function (item) {
        var card = document.createElement("article");
        card.className = "mini mini--fav";
        card.setAttribute("data-fav-id", item.id);
        card.setAttribute("data-accent", item.accent || "violet");

        var grip = document.createElement("span");
        grip.className = "mini__drag";
        grip.title = "拖拽排序";
        grip.setAttribute("data-drag-handle", "");
        grip.innerHTML = GRIP_SVG;

        var link = document.createElement("a");
        link.className = "mini__link";
        link.href = "/go/" + encodeURIComponent(item.id);
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        link.setAttribute("data-link", item.id);
        link.setAttribute("data-name", item.name || item.id);

        var icon = document.createElement("span");
        icon.className = "mini__icon";
        icon.innerHTML = /^<svg[\s>]/i.test(item.icon || "") ? item.icon : "";

        var text = document.createElement("span");
        text.className = "mini__text";
        var strong = document.createElement("strong");
        strong.textContent = item.name || item.id;
        var small = document.createElement("small");
        small.textContent = item.desc || "";
        text.appendChild(strong);
        text.appendChild(small);

        link.appendChild(icon);
        link.appendChild(text);

        var actions = document.createElement("span");
        actions.className = "mini__actions";
        actions.appendChild(miniButton("‹", "data-move", "-1", "前移"));
        actions.appendChild(miniButton("›", "data-move", "1", "后移"));
        actions.appendChild(miniButton("×", "data-fav-remove", item.id, "移出收藏"));

        card.appendChild(grip);
        card.appendChild(link);
        card.appendChild(actions);
        body.appendChild(card);
      });

      sync();
    }

    function moveTo(id, targetId, after) {
      var from = indexOf(id);
      if (from < 0) return;
      var moved = items.splice(from, 1)[0];
      var to = indexOf(targetId);
      if (to < 0) {
        to = items.length;
      } else if (after) {
        to += 1;
      }
      items.splice(to, 0, moved);
      save();
      render();
    }

    function shift(id, delta) {
      var from = indexOf(id);
      var to = from + delta;
      if (from < 0 || to < 0 || to >= items.length) return;
      var moved = items.splice(from, 1)[0];
      items.splice(to, 0, moved);
      save();
      render();
    }

    document.addEventListener("click", function (event) {
      var btn = event.target.closest ? event.target.closest("[data-fav]") : null;
      if (!btn) return;
      event.preventDefault();

      var id = btn.getAttribute("data-fav");
      var existing = indexOf(id);
      if (existing >= 0) {
        items.splice(existing, 1);
        save();
        render();
        toast("已移出我的收藏");
        return;
      }

      var holder = btn.closest(".link-item") || btn.closest(".result");
      var iconEl = holder && $(".link__icon", holder);
      var descEl = holder && $(".link-desc", holder);
      var nameEl = holder && $(".name-text", holder);
      var accentEl = btn.closest("[data-accent]");

      items.unshift({
        id: id,
        name: nameEl ? nameEl.textContent.trim() : id,
        desc: descEl ? descEl.textContent.trim() : "",
        accent: accentEl ? accentEl.getAttribute("data-accent") : "violet",
        icon: iconEl ? iconEl.innerHTML : ""
      });
      save();
      render();
      toast("已加入我的收藏");
    });

    body.addEventListener("click", function (event) {
      var remove = event.target.closest("[data-fav-remove]");
      if (remove) {
        var card = remove.closest(".mini--fav");
        var id = card && card.getAttribute("data-fav-id");
        var index = indexOf(id);
        if (index >= 0) {
          items.splice(index, 1);
          save();
          render();
          toast("已移出我的收藏");
        }
        return;
      }

      var move = event.target.closest("[data-move]");
      if (move) {
        var target = move.closest(".mini--fav");
        if (target) shift(target.getAttribute("data-fav-id"), parseInt(move.getAttribute("data-move"), 10));
      }
    });

    body.addEventListener("mousedown", function (event) {
      // 只有按住拖拽手柄才允许拖动，避免影响链接点击
      var card = event.target.closest && event.target.closest(".mini--fav");
      var handle = event.target.closest && event.target.closest("[data-drag-handle]");
      $$(".mini--fav", body).forEach(function (other) {
        if (card && handle && other === card) other.setAttribute("draggable", "true");
        else other.removeAttribute("draggable");
      });
    });

    body.addEventListener("dragstart", function (event) {
      var card = event.target.closest(".mini--fav");
      if (!card) return;
      dragId = card.getAttribute("data-fav-id");
      card.classList.add("is-dragging");
      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", dragId);
      }
    });

    body.addEventListener("dragend", function () {
      dragId = null;
      $$(".mini--fav", body).forEach(function (card) {
        card.classList.remove("is-dragging", "is-drop-target");
      });
    });

    body.addEventListener("dragover", function (event) {
      var card = event.target.closest(".mini--fav");
      if (!card || !dragId || card.getAttribute("data-fav-id") === dragId) return;
      event.preventDefault();
      $$(".mini--fav", body).forEach(function (other) {
        other.classList.toggle("is-drop-target", other === card);
      });
    });

    body.addEventListener("drop", function (event) {
      var card = event.target.closest(".mini--fav");
      if (!card || !dragId) return;
      event.preventDefault();
      var id = card.getAttribute("data-fav-id");
      if (id === dragId) return;
      // 落在目标卡片右半边就插到它后面
      var rect = card.getBoundingClientRect();
      moveTo(dragId, id, event.clientX > rect.left + rect.width / 2);
      dragId = null;
    });

    var clear = $("[data-fav-clear]");
    if (clear) {
      clear.addEventListener("click", function () {
        if (!items.length) return;
        items = [];
        save();
        render();
        toast("已清空我的收藏");
      });
    }

    render();
  }

  /* ---------- 最近使用 -------------------------------------------------- */

  function initRecent() {
    var strip = $("[data-recent]");
    var body = $("[data-recent-body]");
    if (!strip || !body) return;

    function render() {
      var items = readJSON(RECENT_KEY, []);
      body.textContent = "";

      if (!items.length) {
        strip.hidden = true;
        return;
      }
      strip.hidden = false;

      items.forEach(function (item) {
        var link = document.createElement("a");
        link.className = "mini";
        link.href = "/go/" + encodeURIComponent(item.id);
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        link.setAttribute("data-link", item.id);
        link.setAttribute("data-name", item.name || item.id);
        link.setAttribute("data-accent", item.accent || "violet");

        var icon = document.createElement("span");
        icon.className = "mini__icon";
        icon.innerHTML = /^<svg[\s>]/i.test(item.icon || "") ? item.icon : "";

        var text = document.createElement("span");
        text.className = "mini__text";
        var strong = document.createElement("strong");
        strong.textContent = item.name || item.id;
        var small = document.createElement("small");
        small.textContent = item.desc || "";
        text.appendChild(strong);
        text.appendChild(small);

        link.appendChild(icon);
        link.appendChild(text);
        body.appendChild(link);
      });
    }

    document.addEventListener("click", function (event) {
      var link = event.target.closest ? event.target.closest("a[data-link]") : null;
      if (!link) return;

      var iconEl = $(".link__icon", link) || $(".mini__icon", link);
      var descEl = $(".link-desc", link) || $("small", link);
      var entry = {
        id: link.getAttribute("data-link"),
        name: link.getAttribute("data-name") || link.textContent.trim(),
        accent: (link.closest("[data-accent]") || link).getAttribute("data-accent") || "violet",
        desc: descEl ? descEl.textContent.trim() : "",
        icon: iconEl ? iconEl.innerHTML : ""
      };
      if (!entry.id) return;

      var items = readJSON(RECENT_KEY, []).filter(function (it) { return it && it.id !== entry.id; });
      items.unshift(entry);
      writeJSON(RECENT_KEY, items.slice(0, RECENT_MAX));
      render();
    });

    var clear = $("[data-recent-clear]");
    if (clear) {
      clear.addEventListener("click", function () {
        writeJSON(RECENT_KEY, []);
        render();
        toast("已清空最近使用");
      });
    }

    render();
  }

  /* ---------- 主题 / 快捷键 / 时钟 -------------------------------------- */

  function initTheme() {
    var btn = $("[data-theme-toggle]");
    if (!btn || !window.GONAV_THEME) return;
    btn.addEventListener("click", function () {
      var next = window.GONAV_THEME.get() === "dark" ? "light" : "dark";
      window.GONAV_THEME.set(next);
      toast(next === "dark" ? "已切换到深色模式" : "已切换到浅色模式");
    });
  }

  function initShortcuts() {
    document.addEventListener("keydown", function (event) {
      var target = event.target;
      var typing =
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable);

      if (event.key === "/" && !typing) {
        var input = $("[data-search-input]");
        if (input) {
          event.preventDefault();
          input.focus();
          input.select();
        }
        return;
      }

      if (event.key === "Escape") {
        var el = $("[data-search-input]");
        if (el && el.value !== "") {
          el.value = "";
          document.dispatchEvent(new CustomEvent("gonav:clear-filter"));
        }
        if (document.activeElement === el) el.blur();
        return;
      }

      if ((event.key === "t" || event.key === "T") && !typing && !event.ctrlKey && !event.metaKey && !event.altKey) {
        if (window.GONAV_THEME) {
          window.GONAV_THEME.set(window.GONAV_THEME.get() === "dark" ? "light" : "dark");
        }
      }
    });
  }

  function initClock() {
    var nodes = $$("[data-clock]");
    if (!nodes.length) return;

    function tick() {
      var now = new Date();
      var pad = function (n) { return n < 10 ? "0" + n : String(n); };
      var text = pad(now.getHours()) + ":" + pad(now.getMinutes()) + ":" + pad(now.getSeconds());
      nodes.forEach(function (el) { el.textContent = text; });
    }
    tick();
    window.setInterval(tick, 1000);
  }

  /* ---------- 启动 ------------------------------------------------------ */

  function boot() {
    initSearch();
    initFilter();
    initFavorites();
    initRecent();
    initTheme();
    initShortcuts();
    initClock();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
