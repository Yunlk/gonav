import { useRef, useState } from "react";
import { BUILTIN_PLUGINS, buildInstalled, fetchManifest, permissionLabel } from "../lib/plugins";
import { buildExport, downloadText, restoreImages } from "../lib/backup";
import { applyCustomCss } from "../lib/customCss";
import { DEFAULT_WALLPAPERS, THEMES, uid } from "../lib/defaults";
import { putImage } from "../lib/db";
import { LOCALES, useT } from "../lib/i18n";
import { parseImported } from "../lib/schema";
import { runSourcePlugin } from "../lib/sources";
import { useNavkit } from "../lib/store";
import { buildThemePack, packToOverrides, parseThemePack } from "../lib/themePack";
import { EDITABLE_TOKENS, themeTokens } from "../lib/theme";
import type { Engine } from "../types";

export default function SettingsPanel({ onClose }: { onClose: () => void }) {
  const t = useT();
  const profile = useNavkit((s) => s.profile);
  const store = useNavkit();
  const [importErrors, setImportErrors] = useState<string[]>([]);
  const [notice, setNotice] = useState("");
  const [pluginUrl, setPluginUrl] = useState("");
  const [cssDraft, setCssDraft] = useState<string | null>(null);
  const [scriptName, setScriptName] = useState("我的用户脚本");
  const [scriptId, setScriptId] = useState("myscript");
  const [scriptCode, setScriptCode] = useState("");
  const [scriptPerms, setScriptPerms] = useState<string[]>(["storage"]);
  const fileRef = useRef<HTMLInputElement>(null);
  const themePackRef = useRef<HTMLInputElement>(null);

  if (!profile) return null;

  const activePage = profile.pages.find((p) => p.id === profile.activePageId) ?? profile.pages[0];
  const wallpaper = profile.wallpaper;
  const tokens = themeTokens(profile);

  async function handleExport() {
    const text = await buildExport(profile!, true);
    downloadText(`navkit-${new Date().toISOString().slice(0, 10)}.json`, text);
    setNotice(t("settings.notice.exported"));
  }

  async function handleImport(file: File) {
    setImportErrors([]);
    setNotice("");
    const result = parseImported(await file.text());
    if (!result.ok || !result.profile) {
      setImportErrors(result.errors);
      return;
    }
    const restored = await restoreImages(result.images);
    store.replaceProfile(result.profile);
    setNotice(restored > 0 ? t("settings.notice.importedImages", { n: restored }) : t("settings.notice.imported"));
  }

  async function handleUpload(file: File) {
    const id = uid("img");
    await putImage({ id, name: file.name, blob: file });
    store.setWallpaper({ kind: "image", imageId: id, blur: 0, dim: 25 });
    setNotice(t("settings.notice.wallpaper", { name: file.name }));
  }

  async function installFrom(url: string, source: "builtin" | "url") {
    setNotice("");
    const { manifest, error } = await fetchManifest(url);
    if (!manifest || error) {
      setImportErrors([error ?? "install failed"]);
      return;
    }
    if (profile!.plugins.some((p) => p.id === manifest.id)) {
      setNotice(t("settings.notice.pluginReplaced", { name: manifest.name }));
    }
    store.installPlugin(buildInstalled(manifest, url, source));
    setNotice(
      manifest.permissions.length > 0
        ? t("settings.notice.pluginInstalledPerm", {
            name: manifest.name,
            perms: manifest.permissions.map((permission) => permissionLabel(permission, t)).join("；"),
          })
        : t("settings.notice.pluginInstalled", { name: manifest.name })
    );
  }

  async function importSource(pluginId: string) {
    const plugin = profile!.plugins.find((p) => p.id === pluginId);
    if (!plugin) return;
    const { groups, error } = await runSourcePlugin(plugin, profile!.locale, t);
    if (error || !groups) {
      setImportErrors([error ?? "import failed"]);
      return;
    }
    const nextGroups = groups.map((group) => ({
      id: uid("group"),
      title: group.title,
      items: group.items.map((item) => ({
        kind: "link" as const,
        id: uid("link"),
        title: item.title,
        url: item.url,
        icon: item.icon ?? "",
      })),
    }));
    store.replacePage(activePage.id, [...activePage.groups, ...nextGroups]);
    const total = groups.reduce((sum, g) => sum + g.items.length, 0);
    setNotice(t("settings.notice.sourceImported", { name: plugin.manifest.name, count: total, groups: groups.length }));
  }

  /** 保存用户脚本：交给宿主托管包装页，再作为沙箱插件装上（不在主页面执行）。 */
  async function saveUserScript() {
    setImportErrors([]);
    setNotice("");
    let payload: { wrapUrl?: string };
    try {
      const response = await fetch(`/api/userscripts/${encodeURIComponent(scriptId)}`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name: scriptName, code: scriptCode, permissions: scriptPerms }),
      });
      if (!response.ok) {
        const detail = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
        setImportErrors([String((detail as { error?: string }).error ?? response.status)]);
        return;
      }
      payload = (await response.json()) as { wrapUrl?: string };
    } catch (err) {
      setImportErrors([(err as Error).message]);
      return;
    }

    const wrapUrl = payload.wrapUrl ?? `/navkit-userscript/${scriptId}.html`;
    const pluginId = `userscript-${scriptId}`;
    store.installPlugin({
      id: pluginId,
      manifest: {
        id: pluginId,
        name: scriptName,
        version: "1.0.0",
        apiVersion: 1,
        kind: "widget",
        description: t("settings.geek.scriptDesc"),
        entry: wrapUrl,
        permissions: scriptPerms,
        settings: [],
        defaultSize: { w: 1, h: 1 },
      },
      manifestUrl: wrapUrl,
      entryUrl: wrapUrl,
      enabled: true,
      settings: {},
      installedAt: Date.now(),
      source: "url",
    });
    setNotice(t("settings.notice.userScript", { name: scriptName }));
  }

  function handleThemePack(file: File) {    void file.text().then((text) => {
      const { pack, error } = parseThemePack(text);
      if (!pack || error) {
        setImportErrors([error ?? "theme pack parse failed"]);
        return;
      }
      store.patch({ themeOverrides: packToOverrides(pack) });
      setNotice(t("settings.notice.themePack", { name: pack.name }));
    });
  }

  return (
    <aside className="drawer" role="dialog" aria-modal="true" aria-label={t("settings.title")} data-settings>
      <header className="drawer__head">
        <h2>{t("settings.title")}</h2>
        <button type="button" className="drawer__close" onClick={onClose} aria-label={t("settings.close")} data-settings-close>
          ×
        </button>
      </header>

      <div className="drawer__body">
        {notice ? <p className="notice" data-settings-notice>{notice}</p> : null}
        {importErrors.length > 0 ? (
          <div className="notice notice--error" data-import-errors>
            <b>{t("settings.error.title")}</b>
            <ul>
              {importErrors.map((error) => (
                <li key={error}>{error}</li>
              ))}
            </ul>
          </div>
        ) : null}

        <section className="panel">
          <h3>{t("settings.locale.title")}</h3>
          <div className="theme-row">
            {LOCALES.map((locale) => (
              <button
                key={locale.id}
                type="button"
                className={`theme-chip${profile.locale === locale.id ? " is-selected" : ""}`}
                onClick={() => store.setLocale(locale.id)}
                data-locale={locale.id}
              >
                {locale.label}
              </button>
            ))}
          </div>
          <p className="panel__hint">{t("settings.locale.hint")}</p>
        </section>

        <section className="panel">
          <h3>{t("settings.theme.title")}</h3>
          <div className="theme-row">
            {THEMES.map((theme) => (
              <button
                key={theme.id}
                type="button"
                className={`theme-chip${profile.themeId === theme.id ? " is-selected" : ""}`}
                onClick={() => store.setTheme(theme.id)}
                data-settings-theme={theme.id}
              >
                <span
                  className="theme-chip__dot"
                  style={{ background: theme.tokens.bg, boxShadow: `inset 0 0 0 2px ${theme.tokens.accent}` }}
                />
                {t(theme.nameKey)}
              </button>
            ))}
          </div>

          <h4 className="panel__sub">{t("settings.theme.tokens")}</h4>
          <div className="token-grid">
            {EDITABLE_TOKENS.map((token) => {
              const value = tokens[token.key];
              return (
                <label className="token" key={token.key}>
                  <span>{token.label}</span>
                  {token.type === "color" ? (
                    <input
                      type="color"
                      value={String(value).startsWith("#") ? String(value) : "#000000"}
                      onChange={(event) => store.setThemeOverride(token.key, event.target.value)}
                      data-token={token.key}
                    />
                  ) : (
                    <input
                      type="number"
                      value={Number(value)}
                      min={0}
                      max={48}
                      onChange={(event) => store.setThemeOverride(token.key, event.target.value)}
                      data-token={token.key}
                    />
                  )}
                </label>
              );
            })}
          </div>
          <div className="field-row">
            <button type="button" className="btn" onClick={() => store.clearThemeOverrides()} data-token-reset>
              {t("settings.theme.clear")}
            </button>
            <button
              type="button"
              className="btn"
              onClick={() => {
                downloadText("navkit-theme.json", buildThemePack("navkit theme", tokens));
                setNotice(t("settings.notice.themePackExported"));
              }}
              data-theme-export
            >
              {t("settings.theme.export")}
            </button>
            <button type="button" className="btn" onClick={() => themePackRef.current?.click()} data-theme-import>
              {t("settings.theme.import")}
            </button>
            <input
              ref={themePackRef}
              type="file"
              accept="application/json,.json"
              hidden
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) handleThemePack(file);
                event.target.value = "";
              }}
            />
          </div>
        </section>

        <section className="panel">
          <h3>{t("settings.plugins.title")}</h3>
          <p className="panel__hint">{t("settings.plugins.hint")}</p>

          <div className="plugin-row">
            {BUILTIN_PLUGINS.map((builtin) => {
              const installed = profile.plugins.some((p) => p.id === builtin.id);
              return (
                <button
                  key={builtin.id}
                  type="button"
                  className="plugin-chip"
                  disabled={installed}
                  onClick={() => void installFrom(builtin.manifestUrl, "builtin")}
                  data-install-builtin={builtin.id}
                >
                  <b>{t(builtin.nameKey)}</b>
                  <small>{installed ? t("settings.plugins.installed") : t(builtin.hintKey)}</small>
                </button>
              );
            })}
          </div>

          <div className="field-row">
            <input
              className="plugin-url"
              value={pluginUrl}
              placeholder={t("settings.plugins.urlPlaceholder")}
              onChange={(event) => setPluginUrl(event.target.value)}
              data-plugin-url
            />
            <button
              type="button"
              className="btn"
              disabled={!pluginUrl.trim()}
              onClick={() => void installFrom(pluginUrl.trim(), "url")}
              data-plugin-install
            >
              {t("settings.plugins.install")}
            </button>
          </div>

          <ul className="plugin-list">
            {profile.plugins.length === 0 ? <li className="plugin-empty">{t("settings.plugins.empty")}</li> : null}
            {profile.plugins.map((plugin) => (
              <li className="plugin-item" key={plugin.id} data-plugin={plugin.id}>
                <div className="plugin-item__head">
                  <b>{plugin.manifest.name}</b>
                  <span className="plugin-item__ver">
                    v{plugin.manifest.version} ·{" "}
                    {plugin.manifest.kind === "widget"
                      ? t("settings.plugins.kind.widget")
                      : t("settings.plugins.kind.source")}{" "}
                    ·{" "}
                    {plugin.source === "builtin"
                      ? t("settings.plugins.source.builtin")
                      : t("settings.plugins.source.url")}
                  </span>
                  <label className="plugin-toggle">
                    <input
                      type="checkbox"
                      checked={plugin.enabled}
                      onChange={() => store.togglePlugin(plugin.id)}
                      data-plugin-toggle={plugin.id}
                    />
                    {t("settings.plugins.enable")}
                  </label>
                  <button
                    type="button"
                    className="engine-row__del"
                    onClick={() => store.removePlugin(plugin.id)}
                    aria-label={t("settings.plugins.uninstall", { name: plugin.manifest.name })}
                    data-plugin-remove={plugin.id}
                  >
                    ×
                  </button>
                </div>

                {plugin.manifest.description ? <p className="plugin-item__desc">{plugin.manifest.description}</p> : null}

                <p className="plugin-item__perm">
                  {t("settings.plugins.perms")}
                  {plugin.manifest.permissions.length === 0
                    ? t("settings.plugins.noPerms")
                    : plugin.manifest.permissions.map((permission) => permissionLabel(permission, t)).join("；")}
                </p>

                {plugin.manifest.kind === "source" ? (
                  <button
                    type="button"
                    className="btn btn--mini"
                    onClick={() => void importSource(plugin.id)}
                    data-source-import={plugin.id}
                  >
                    {t("settings.plugins.import")}
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        </section>

        <section className="panel">
          <h3>{t("settings.geek.title")}</h3>
          <p className="panel__hint">{t("settings.geek.hint")}</p>
          <textarea
            className="css-box"
            rows={6}
            spellCheck={false}
            value={cssDraft ?? profile.customCss}
            placeholder={".tile { box-shadow: 0 0 0 1px var(--nk-accent); }"}
            onChange={(event) => setCssDraft(event.target.value)}
            data-custom-css
          />
          <div className="field-row">
            <button
              type="button"
              className="btn"
              onClick={() => {
                const css = cssDraft ?? "";
                const result = applyCustomCss(css);
                if (!result.ok) {
                  setImportErrors([result.error ?? "inject failed"]);
                  return;
                }
                store.setCustomCss(css);
                setNotice(t("settings.notice.customCss"));
              }}
              data-custom-css-apply
            >
              {t("settings.geek.apply")}
            </button>
            <button
              type="button"
              className="btn btn--danger"
              onClick={() => {
                applyCustomCss("");
                store.setCustomCss("");
                setCssDraft("");
                setNotice(t("settings.notice.customCssCleared"));
              }}
              data-custom-css-clear
            >
              {t("settings.geek.clear")}
            </button>
          </div>

          <h4 className="panel__sub">{t("settings.geek.scriptTitle")}</h4>
          <p className="panel__hint">{t("settings.geek.scriptHint")}</p>
          <div className="field-row">
            <label className="field">
              <span>{t("settings.geek.scriptName")}</span>
              <input value={scriptName} onChange={(event) => setScriptName(event.target.value)} data-script-name />
            </label>
            <label className="field">
              <span>{t("settings.geek.scriptId")}</span>
              <input
                value={scriptId}
                onChange={(event) => setScriptId(event.target.value.toLowerCase().replace(/[^a-z0-9-_.]/g, ""))}
                data-script-id
              />
            </label>
          </div>
          <div className="field-row">
            {["storage", "theme:read", "search:query"].map((permission) => (
              <label className="field field--inline" key={permission}>
                <input
                  type="checkbox"
                  checked={scriptPerms.includes(permission)}
                  onChange={(event) =>
                    setScriptPerms((current) =>
                      event.target.checked ? [...current, permission] : current.filter((p) => p !== permission)
                    )
                  }
                  data-script-perm={permission}
                />
                <span>{permissionLabel(permission, t)}</span>
              </label>
            ))}
          </div>
          <textarea
            className="css-box"
            rows={8}
            spellCheck={false}
            value={scriptCode}
            placeholder={"// 沙箱里运行：没有 DOM 访问权，也没有 localStorage\n// 用 navkit 的 RPC 拿数据：\n//   navkit.rpc('storage.get', { key: 'x' })"}
            onChange={(event) => setScriptCode(event.target.value)}
            data-script-code
          />
          <div className="field-row">
            <button
              type="button"
              className="btn"
              disabled={!scriptCode.trim() || !scriptId.trim()}
              onClick={() => void saveUserScript()}
              data-script-save
            >
              {t("settings.geek.scriptSave")}
            </button>
          </div>
        </section>

        <section className="panel">
          <h3>{t("settings.wallpaper.title")}</h3>
          <div className="wallpaper-row">
            {DEFAULT_WALLPAPERS.map((preset) => (
              <button
                key={preset.id}
                type="button"
                className={`wallpaper-chip${wallpaper.kind === "preset" && wallpaper.value === preset.id ? " is-selected" : ""}`}
                style={{ background: preset.css }}
                title={preset.name}
                onClick={() => store.setWallpaper({ kind: "preset", value: preset.id })}
                data-wallpaper-preset={preset.id}
              >
                <span>{preset.name}</span>
              </button>
            ))}
          </div>

          <div className="field-row">
            <label className="field field--inline">
              <span>{t("settings.wallpaper.solid")}</span>
              <input
                type="color"
                value={wallpaper.kind === "solid" ? wallpaper.value : "#1f2a44"}
                onChange={(event) => store.setWallpaper({ kind: "solid", value: event.target.value })}
                data-wallpaper-solid
              />
            </label>

            <label className="field field--inline">
              <span>{t("settings.wallpaper.gradient")}</span>
              <input
                type="color"
                value={wallpaper.kind === "gradient" ? wallpaper.from : "#1f2a44"}
                onChange={(event) =>
                  store.setWallpaper({
                    kind: "gradient",
                    from: event.target.value,
                    to: wallpaper.kind === "gradient" ? wallpaper.to : "#7f8fa6",
                    angle: wallpaper.kind === "gradient" ? wallpaper.angle : 150,
                  })
                }
                aria-label={t("settings.wallpaper.gradientFrom")}
              />
              <input
                type="color"
                value={wallpaper.kind === "gradient" ? wallpaper.to : "#7f8fa6"}
                onChange={(event) =>
                  store.setWallpaper({
                    kind: "gradient",
                    from: wallpaper.kind === "gradient" ? wallpaper.from : "#1f2a44",
                    to: event.target.value,
                    angle: wallpaper.kind === "gradient" ? wallpaper.angle : 150,
                  })
                }
                aria-label={t("settings.wallpaper.gradientTo")}
                data-wallpaper-gradient-to
              />
            </label>
          </div>

          <button type="button" className="btn" onClick={() => fileRef.current?.click()} data-wallpaper-upload>
            {t("settings.wallpaper.upload")}
          </button>
          <input
            ref={fileRef}
            type="file"
            accept="image/*"
            hidden
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) void handleUpload(file);
              event.target.value = "";
            }}
          />

          {wallpaper.kind === "image" ? (
            <div className="field-row">
              <label className="field">
                <span>{t("settings.wallpaper.blur", { n: wallpaper.blur })}</span>
                <input
                  type="range"
                  min={0}
                  max={30}
                  value={wallpaper.blur}
                  onChange={(event) =>
                    store.setWallpaper({
                      kind: "image",
                      imageId: wallpaper.imageId,
                      blur: Number(event.target.value),
                      dim: wallpaper.dim,
                    })
                  }
                  data-wallpaper-blur
                />
              </label>
              <label className="field">
                <span>{t("settings.wallpaper.dim", { n: wallpaper.dim })}</span>
                <input
                  type="range"
                  min={0}
                  max={70}
                  value={wallpaper.dim}
                  onChange={(event) =>
                    store.setWallpaper({
                      kind: "image",
                      imageId: wallpaper.imageId,
                      blur: wallpaper.blur,
                      dim: Number(event.target.value),
                    })
                  }
                  data-wallpaper-dim
                />
              </label>
            </div>
          ) : null}
        </section>

        <section className="panel">
          <h3>
            {t("settings.tileSize.title")} <span className="panel__value">{profile.tileSize}px</span>
          </h3>
          <input
            type="range"
            min={56}
            max={180}
            step={4}
            value={profile.tileSize}
            onChange={(event) => store.setTileSize(Number(event.target.value))}
            data-tile-size
          />
        </section>

        <section className="panel">
          <h3>{t("settings.engines.title")}</h3>
          <ul className="engine-list">
            {profile.engines.map((engine) => (
              <li key={engine.id} className="engine-row" data-engine-row={engine.id}>
                <input
                  className="engine-row__name"
                  value={engine.name}
                  onChange={(event) => store.updateEngine({ ...engine, name: event.target.value } as Engine)}
                  aria-label={t("settings.engines.name")}
                />
                <input
                  className="engine-row__tpl"
                  value={engine.urlTemplate}
                  onChange={(event) => store.updateEngine({ ...engine, urlTemplate: event.target.value } as Engine)}
                  aria-label={t("settings.engines.template")}
                  spellCheck={false}
                />
                <label className="engine-row__default" title={t("settings.engines.default")}>
                  <input
                    type="radio"
                    name="default-engine"
                    checked={profile.defaultEngineId === engine.id}
                    onChange={() => store.setDefaultEngine(engine.id)}
                    data-engine-default={engine.id}
                  />
                </label>
                <button
                  type="button"
                  className="engine-row__del"
                  onClick={() => store.removeEngine(engine.id)}
                  disabled={profile.engines.length <= 1}
                  aria-label={t("settings.engines.remove", { name: engine.name })}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
          <button
            type="button"
            className="btn"
            onClick={() => store.addEngine({ name: t("settings.engines.new"), urlTemplate: "https://example.com/search?q=%s" })}
            data-engine-add
          >
            {t("settings.engines.add")}
          </button>
          <p className="panel__hint">{t("settings.engines.hint")}</p>
        </section>

        <section className="panel">
          <h3>{t("settings.pages.title")}</h3>
          <ul className="page-list">
            {profile.pages.map((page) => (
              <li key={page.id} className={page.id === profile.activePageId ? "is-active" : ""}>
                <button type="button" className="link-btn" onClick={() => store.setActivePage(page.id)}>
                  {page.title}
                </button>
                <span className="page-list__meta">{t("settings.pages.groups", { n: page.groups.length })}</span>
                <button
                  type="button"
                  className="engine-row__del"
                  onClick={() => store.removePage(page.id)}
                  disabled={profile.pages.length <= 1}
                  aria-label={t("settings.pages.removePage", { name: page.title })}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
          <button type="button" className="btn" onClick={() => store.addPage("")} data-settings-add-page>
            {t("settings.pages.addPage")}
          </button>

          <h4 className="panel__sub">{t("settings.pages.currentGroups")}</h4>
          <ul className="page-list">
            {activePage.groups.map((group) => (
              <li key={group.id}>
                <span>{group.title}</span>
                <span className="page-list__meta">{t("settings.pages.items", { n: group.items.length })}</span>
                <button
                  type="button"
                  className="engine-row__del"
                  onClick={() => store.removeGroup(activePage.id, group.id)}
                  aria-label={t("settings.pages.removeGroup", { name: group.title })}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
          <button
            type="button"
            className="btn"
            onClick={() => store.addGroup(activePage.id, t("settings.pages.newGroup"))}
            data-settings-add-group
          >
            {t("settings.pages.addGroup")}
          </button>
        </section>

        <section className="panel">
          <h3>{t("settings.data.title")}</h3>
          <div className="field-row">
            <button type="button" className="btn" onClick={() => void handleExport()} data-export>
              {t("settings.data.export")}
            </button>
            <button type="button" className="btn" onClick={() => document.getElementById("navkit-import")?.click()} data-import>
              {t("settings.data.import")}
            </button>
            <input
              id="navkit-import"
              type="file"
              accept="application/json,.json"
              hidden
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) void handleImport(file);
                event.target.value = "";
              }}
            />
          </div>
          <button
            type="button"
            className="btn btn--danger"
            onClick={() => {
              if (window.confirm(t("settings.data.resetConfirm"))) {
                void store.reset();
                setNotice(t("settings.notice.reset"));
              }
            }}
            data-reset
          >
            {t("settings.data.reset")}
          </button>
          <p className="panel__hint">{t("settings.data.hint")}</p>
        </section>
      </div>
    </aside>
  );
}
