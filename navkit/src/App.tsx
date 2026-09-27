import { useEffect, useMemo, useState } from "react";
import Grid from "./components/Grid";
import CommandPalette, { type Command } from "./components/CommandPalette";
import Onboarding from "./components/Onboarding";
import PagesBar from "./components/PagesBar";
import SearchBar from "./components/SearchBar";
import SettingsPanel from "./components/SettingsPanel";
import TileEditor, { type TileDraft } from "./components/TileEditor";
import WallpaperLayer from "./components/WallpaperLayer";
import WidgetEditor from "./components/WidgetEditor";
import { applyTheme, themeTokens } from "./lib/theme";
import { applyCustomCss } from "./lib/customCss";
import { useT } from "./lib/i18n";
import { persistProfile, useNavkit } from "./lib/store";
import { THEMES, uid } from "./lib/defaults";
import { isWidget, type LinkItem, type WidgetItem } from "./types";

const HOST_VERSION = "0.1.0";

interface Editing {
  mode: "add" | "edit";
  groupId: string;
  item?: LinkItem;
}

export default function App() {
  const t = useT();
  const profile = useNavkit((s) => s.profile);
  const ready = useNavkit((s) => s.ready);
  const boot = useNavkit((s) => s.boot);
  const store = useNavkit();

  const [query, setQuery] = useState("");
  const [editing, setEditing] = useState<Editing | null>(null);
  const [widgetEditing, setWidgetEditing] = useState<{ item: WidgetItem; groupId: string } | null>(null);
  const [widgetPickGroup, setWidgetPickGroup] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [editMode, setEditMode] = useState(false);

  useEffect(() => {
    void boot();
  }, [boot]);

  // profile 变化 → 防抖写回 IndexedDB
  useEffect(() => useNavkit.subscribe(persistProfile(() => useNavkit.getState().profile)), []);

  const activePage = useMemo(() => {
    if (!profile) return null;
    return profile.pages.find((p) => p.id === profile.activePageId) ?? profile.pages[0];
  }, [profile]);

  const tokens = useMemo(() => (profile ? themeTokens(profile) : null), [profile]);

  // 主题令牌（预设 + 用户覆盖）→ CSS 变量
  useEffect(() => {
    if (tokens && profile) applyTheme(tokens, profile.themeId);
  }, [tokens, profile?.themeId, profile]);

  // 极客模式：自定义 CSS 走 CSSOM 注入，不放宽 CSP
  useEffect(() => {
    if (profile) applyCustomCss(profile.customCss ?? "");
  }, [profile?.customCss, profile]);

  // 语言切换同步到 <html lang>
  useEffect(() => {
    if (profile) document.documentElement.lang = profile.locale;
  }, [profile?.locale, profile]);

  const recentItems = useMemo(() => {
    if (!profile) return [];
    const index = new Map<string, LinkItem>();
    for (const page of profile.pages) {
      for (const group of page.groups) {
        for (const item of group.items) {
          if (!isWidget(item)) index.set(item.id, item);
        }
      }
    }
    return profile.recent.map((entry) => index.get(entry.id)).filter((item): item is LinkItem => Boolean(item));
  }, [profile]);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
        return;
      }
      if (event.key === "Escape") {
        setEditing(null);
        setWidgetEditing(null);
        setWidgetPickGroup(null);
        setSettingsOpen(false);
        setPaletteOpen(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  /** 命令面板的候选：动作 + 页面跳转 + 打开图标，全键盘可操作。 */
  const commands = useMemo<Command[]>(() => {
    if (!profile) return [];
    const list: Command[] = [
      { id: "settings", title: t("palette.cmd.settings"), group: t("palette.group.action"), run: () => setSettingsOpen(true) },
      {
        id: "add-link",
        title: t("palette.cmd.addLink"),
        group: t("palette.group.action"),
        run: () =>
          setEditing({
            mode: "add",
            groupId: profile.pages.find((p) => p.id === profile.activePageId)?.groups[0]?.id ?? "",
          }),
      },
      {
        id: "add-group",
        title: t("palette.cmd.addGroup"),
        group: t("palette.group.action"),
        run: () => store.addGroup(profile.activePageId, t("settings.pages.newGroup")),
      },
      { id: "add-page", title: t("palette.cmd.addPage"), group: t("palette.group.action"), run: () => store.addPage("") },
      {
        id: "edit-mode",
        title: editMode ? t("app.action.editModeExit") : t("app.action.editMode"),
        group: t("palette.group.action"),
        run: () => setEditMode((on) => !on),
      },
      {
        id: "add-widget",
        title: t("palette.cmd.addWidget"),
        group: t("palette.group.action"),
        run: () => setWidgetPickGroup(profile.pages.find((p) => p.id === profile.activePageId)?.groups[0]?.id ?? null),
      },
    ];

    for (const theme of THEMES) {
      list.push({
        id: `theme-${theme.id}`,
        title: t("palette.cmd.theme", { name: t(theme.nameKey) }),
        group: t("palette.group.theme"),
        run: () => store.setTheme(theme.id),
      });
    }

    for (const page of profile.pages) {
      if (page.id === profile.activePageId) continue;
      list.push({
        id: `page-${page.id}`,
        title: t("palette.cmd.page", { name: page.title }),
        group: t("palette.group.page"),
        run: () => store.setActivePage(page.id),
      });
    }

    for (const page of profile.pages) {
      for (const group of page.groups) {
        for (const item of group.items) {
          if (isWidget(item)) {
            continue;
          }
          list.push({
            id: `link-${item.id}`,
            title: item.title,
            hint: group.title,
            group: t("palette.group.link"),
            run: () => {
              store.recordRecent(item.id);
              window.open(item.url, "_blank", "noopener");
            },
          });
        }
      }
    }

    return list;
  }, [profile, store, t, editMode]);

  if (!ready || !profile || !activePage || !tokens) {
    return <div className="boot">{t("app.loading")}</div>;
  }

  if (!profile.onboarded) {
    return <Onboarding onDone={(themeId) => store.completeOnboarding(themeId)} />;
  }

  const groups = activePage.groups;
  const widgetPlugins = profile.plugins.filter((p) => p.enabled && p.manifest.kind === "widget");

  function handleSave(draft: TileDraft) {
    if (!editing || !activePage) return;
    const payload: LinkItem = {
      kind: "link",
      id: editing.item?.id ?? uid("link"),
      title: draft.title,
      url: draft.url,
      icon: draft.icon,
      color: draft.color || undefined,
    };

    if (editing.mode === "add") {
      store.addItem(activePage.id, draft.groupId, payload);
    } else if (editing.item) {
      if (draft.groupId !== editing.groupId) {
        store.removeItem(activePage.id, editing.groupId, editing.item.id);
        store.addItem(activePage.id, draft.groupId, payload);
      } else {
        store.updateItem(activePage.id, editing.groupId, payload);
      }
    }
    setEditing(null);
  }

  function addWidget(pluginId: string) {
    if (!widgetPickGroup || !activePage) return;
    const plugin = profile!.plugins.find((p) => p.id === pluginId);
    const size = plugin?.manifest.defaultSize ?? { w: 1, h: 1 };
    store.addItem(activePage.id, widgetPickGroup, {
      kind: "widget",
      id: uid("widget"),
      pluginId,
      title: plugin?.manifest.name ?? pluginId,
      w: size.w,
      h: size.h,
    });
    setWidgetPickGroup(null);
  }

  return (
    <div className="app">
      <WallpaperLayer wallpaper={profile.wallpaper} tokens={tokens} />

      <div className="shell">
        <header className="bar">
          <a className="brand" href="/" title={t("app.brand.back")}>
            <b>navkit</b>
            <em>{t("app.brand.tagline")}</em>
          </a>

          <SearchBar
            engines={profile.engines}
            defaultEngineId={profile.defaultEngineId}
            onQueryChange={setQuery}
            onEngineChange={(engineId) => store.setDefaultEngine(engineId)}
          />

          <div className="bar__actions">
            <button
              type="button"
              className="icon-btn"
              onClick={() => setEditing({ mode: "add", groupId: groups[0]?.id ?? "" })}
              title={t("app.action.addLink")}
              data-add-tile
              disabled={!groups.length}
            >
              ＋
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={() => setWidgetPickGroup(groups[0]?.id ?? null)}
              title={t("app.action.addWidget")}
              data-add-widget
              disabled={!groups.length || widgetPlugins.length === 0}
            >
              ▦
            </button>
            <button
              type="button"
              className={`icon-btn${editMode ? " is-active" : ""}`}
              onClick={() => setEditMode((on) => !on)}
              title={editMode ? t("app.action.editModeExit") : t("app.action.editMode")}
              data-toggle-edit
              aria-pressed={editMode}
            >
              ✥
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={() => setSettingsOpen(true)}
              title={t("app.action.settings")}
              data-open-settings
            >
              ⚙
            </button>
          </div>
        </header>

        <PagesBar
          pages={profile.pages}
          activePageId={activePage.id}
          onSelect={(pageId) => store.setActivePage(pageId)}
          onAdd={(title) => store.addPage(title)}
          onRename={(pageId, title) => store.renamePage(pageId, title)}
          onRemove={(pageId) => store.removePage(pageId)}
        />

        <main className="content">
          <Grid
            groups={groups}
            recent={recentItems}
            plugins={profile.plugins}
            tileSize={profile.tileSize}
            query={query}
            tokens={tokens}
            themeId={profile.themeId}
            locale={profile.locale}
            hostVersion={HOST_VERSION}
            editMode={editMode}
            onOpen={(item) => store.recordRecent(item.id)}
            onEdit={(item, groupId) => setEditing({ mode: "edit", groupId, item })}
            onConfigureWidget={(item, groupId) => setWidgetEditing({ item, groupId })}
            onAdd={(groupId) => setEditing({ mode: "add", groupId })}
            onAddWidget={(groupId) => setWidgetPickGroup(groupId)}
            onRenameGroup={(groupId, title) => store.renameGroup(activePage.id, groupId, title)}
            onRemoveGroup={(groupId) => store.removeGroup(activePage.id, groupId)}
            onMoveGroup={(groupId, delta) => store.moveGroup(activePage.id, groupId, delta)}
            onResizeWidget={(item, groupId, dw, dh) => {
              const next = {
                ...item,
                w: Math.min(4, Math.max(1, item.w + dw)),
                h: Math.min(4, Math.max(1, item.h + dh)),
              };
              store.updateItem(activePage.id, groupId, next);
            }}
            onClearRecent={() => store.clearRecent()}
            onMove={(activeId, overId) => store.moveItem(activeId, overId)}
          />
        </main>

        <footer className="foot">
          <span>
            {t("app.foot.hint", {
              k: "Ctrl+K",
              s: "/",
              e: "Esc",
            })}
          </span>
          <span className="foot__actions">
            <button type="button" className="link-btn" onClick={() => setPaletteOpen(true)} data-open-palette>
              {t("app.foot.palette")}
            </button>
            <button
              type="button"
              className="link-btn"
              onClick={() => store.addGroup(activePage.id, t("settings.pages.newGroup"))}
              data-add-group
            >
              {t("app.foot.addGroup")}
            </button>
          </span>
        </footer>
      </div>

      {paletteOpen ? <CommandPalette commands={commands} onClose={() => setPaletteOpen(false)} /> : null}

      {editing ? (
        <TileEditor
          mode={editing.mode}
          groups={groups}
          item={editing.item}
          groupId={editing.groupId}
          onSave={handleSave}
          onDelete={
            editing.item
              ? () => {
                  store.removeItem(activePage.id, editing.groupId, editing.item!.id);
                  setEditing(null);
                }
              : undefined
          }
          onClose={() => setEditing(null)}
        />
      ) : null}

      {widgetEditing ? (
        <WidgetEditor
          item={widgetEditing.item}
          plugin={profile.plugins.find((p) => p.id === widgetEditing.item.pluginId)}
          onSave={(next, settings) => {
            store.updateItem(activePage.id, widgetEditing.groupId, next);
            store.updatePluginSettings(next.pluginId, settings);
            setWidgetEditing(null);
          }}
          onDelete={() => {
            store.removeItem(activePage.id, widgetEditing.groupId, widgetEditing.item.id);
            setWidgetEditing(null);
          }}
          onClose={() => setWidgetEditing(null)}
        />
      ) : null}

      {widgetPickGroup ? (
        <div className="modal" role="dialog" aria-modal="true" aria-label={t("widget.picker.title")} data-widget-picker>
          <div className="modal__scrim" onClick={() => setWidgetPickGroup(null)} />
          <div className="modal__panel">
            <header className="modal__head">
              <h2>{t("widget.picker.title")}</h2>
              <button
                type="button"
                className="modal__close"
                onClick={() => setWidgetPickGroup(null)}
                aria-label={t("widget.close")}
              >
                ×
              </button>
            </header>
            <ul className="picker">
              {widgetPlugins.map((plugin) => (
                <li key={plugin.id}>
                  <button
                    type="button"
                    className="picker__item"
                    onClick={() => addWidget(plugin.id)}
                    data-widget-option={plugin.id}
                  >
                    <b>{plugin.manifest.name}</b>
                    <small>{plugin.manifest.description ?? plugin.manifest.version}</small>
                  </button>
                </li>
              ))}
            </ul>
            <p className="panel__hint">{t("widget.picker.hint")}</p>
          </div>
        </div>
      ) : null}

      {settingsOpen ? <SettingsPanel onClose={() => setSettingsOpen(false)} /> : null}
    </div>
  );
}
