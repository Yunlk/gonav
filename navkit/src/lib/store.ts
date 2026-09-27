import { create } from "zustand";
import { defaultProfile, uid } from "./defaults";
import { loadProfile, saveProfile } from "./db";
import type {
  Engine,
  Group,
  InstalledPlugin,
  LinkItem,
  NavItem,
  Page,
  Profile,
  RecentEntry,
  Wallpaper,
  WidgetItem,
} from "../types";

const RECENT_MAX = 8;

/** 新建单元：链接与组件都允许不带 id（由 store 生成）。 */
export type NewItem = (Omit<LinkItem, "id"> & { id?: string }) | (Omit<WidgetItem, "id"> & { id?: string });

export interface ItemLocation {
  pageId: string;
  groupId: string;
  index: number;
}

export function findItem(profile: Profile, itemId: string): ItemLocation | null {
  for (const page of profile.pages) {
    for (const group of page.groups) {
      const index = group.items.findIndex((i) => i.id === itemId);
      if (index >= 0) return { pageId: page.id, groupId: group.id, index };
    }
  }
  return null;
}

export function findItemById(profile: Profile, itemId: string): NavItem | null {
  for (const page of profile.pages) {
    for (const group of page.groups) {
      const found = group.items.find((i) => i.id === itemId);
      if (found) return found;
    }
  }
  return null;
}

interface State {
  profile: Profile | null;
  ready: boolean;
  boot: () => Promise<void>;
  patch: (partial: Partial<Profile>) => void;
  completeOnboarding: (themeId: string) => void;
  setTheme: (themeId: string) => void;
  setThemeOverride: (key: string, value: string) => void;
  clearThemeOverrides: () => void;
  setCustomCss: (css: string) => void;
  setLocale: (locale: Profile["locale"]) => void;
  setTileSize: (size: number) => void;
  setWallpaper: (wallpaper: Wallpaper) => void;
  setDefaultEngine: (engineId: string) => void;
  addEngine: (engine: Omit<Engine, "id">) => void;
  updateEngine: (engine: Engine) => void;
  removeEngine: (engineId: string) => void;
  setActivePage: (pageId: string) => void;
  addPage: (title: string) => void;
  renamePage: (pageId: string, title: string) => void;
  removePage: (pageId: string) => void;
  addGroup: (pageId: string, title: string) => void;
  renameGroup: (pageId: string, groupId: string, title: string) => void;
  removeGroup: (pageId: string, groupId: string) => void;
  moveGroup: (pageId: string, groupId: string, delta: number) => void;
  addItem: (pageId: string, groupId: string, item: NewItem) => void;
  updateItem: (pageId: string, groupId: string, item: NavItem) => void;
  removeItem: (pageId: string, groupId: string, itemId: string) => void;
  moveItem: (activeId: string, overId: string) => void;
  recordRecent: (itemId: string) => void;
  clearRecent: () => void;
  installPlugin: (plugin: InstalledPlugin) => void;
  removePlugin: (pluginId: string) => void;
  togglePlugin: (pluginId: string) => void;
  updatePluginSettings: (pluginId: string, patch: Record<string, string | number | boolean>) => void;
  replacePage: (pageId: string, groups: Group[]) => void;
  replaceProfile: (profile: Profile) => void;
  reset: () => Promise<void>;
}

function mapGroup(pages: Page[], pageId: string, groupId: string, fn: (group: Group) => Group): Page[] {
  return pages.map((page) =>
    page.id !== pageId
      ? page
      : { ...page, groups: page.groups.map((group) => (group.id !== groupId ? group : fn(group))) }
  );
}

export const useNavkit = create<State>((set, get) => {
  function commit(updater: (profile: Profile) => Profile): void {
    const current = get().profile;
    if (!current) return;
    set({ profile: updater(current) });
  }

  return {
    profile: null,
    ready: false,

    async boot() {
      const stored = await loadProfile();
      set({ profile: stored ?? defaultProfile(), ready: true });
    },

    patch(partial) {
      commit((p) => ({ ...p, ...partial }));
    },

    completeOnboarding(themeId) {
      commit((p) => ({ ...p, onboarded: true, themeId }));
    },

    setTheme(themeId) {
      commit((p) => ({ ...p, themeId }));
    },

    setThemeOverride(key, value) {
      commit((p) => ({ ...p, themeOverrides: { ...p.themeOverrides, [key]: value } }));
    },

    clearThemeOverrides() {
      commit((p) => ({ ...p, themeOverrides: {} }));
    },

    setCustomCss(customCss) {
      commit((p) => ({ ...p, customCss }));
    },

    setLocale(locale) {
      commit((p) => ({ ...p, locale }));
    },

    setTileSize(tileSize) {
      commit((p) => ({ ...p, tileSize: Math.min(180, Math.max(56, Math.round(tileSize))) }));
    },

    setWallpaper(wallpaper) {
      commit((p) => ({ ...p, wallpaper }));
    },

    setDefaultEngine(defaultEngineId) {
      commit((p) => ({ ...p, defaultEngineId }));
    },

    addEngine(engine) {
      commit((p) => ({ ...p, engines: [...p.engines, { ...engine, id: uid("engine") }] }));
    },

    updateEngine(engine) {
      commit((p) => ({ ...p, engines: p.engines.map((e) => (e.id === engine.id ? engine : e)) }));
    },

    removeEngine(engineId) {
      commit((p) => {
        if (p.engines.length <= 1) return p;
        const engines = p.engines.filter((e) => e.id !== engineId);
        const defaultEngineId = p.defaultEngineId === engineId ? engines[0].id : p.defaultEngineId;
        return { ...p, engines, defaultEngineId };
      });
    },

    setActivePage(activePageId) {
      commit((p) => ({ ...p, activePageId }));
    },

    addPage(title) {
      commit((p) => {
        const page: Page = { id: uid("page"), title: title || `页面 ${p.pages.length + 1}`, groups: [] };
        return { ...p, pages: [...p.pages, page], activePageId: page.id };
      });
    },

    renamePage(pageId, title) {
      commit((p) => ({ ...p, pages: p.pages.map((page) => (page.id === pageId ? { ...page, title } : page)) }));
    },

    removePage(pageId) {
      commit((p) => {
        if (p.pages.length <= 1) return p;
        const pages = p.pages.filter((page) => page.id !== pageId);
        const activePageId = p.activePageId === pageId ? pages[0].id : p.activePageId;
        return { ...p, pages, activePageId };
      });
    },

    addGroup(pageId, title) {
      commit((p) => ({
        ...p,
        pages: p.pages.map((page) =>
          page.id !== pageId
            ? page
            : { ...page, groups: [...page.groups, { id: uid("group"), title: title || "新分组", items: [] }] }
        ),
      }));
    },

    renameGroup(pageId, groupId, title) {
      commit((p) => ({ ...p, pages: mapGroup(p.pages, pageId, groupId, (g) => ({ ...g, title })) }));
    },

    removeGroup(pageId, groupId) {
      commit((p) => ({
        ...p,
        pages: p.pages.map((page) =>
          page.id !== pageId ? page : { ...page, groups: page.groups.filter((g) => g.id !== groupId) }
        ),
      }));
    },

    /** 编辑模式里上下移动整个分组。 */
    moveGroup(pageId, groupId, delta) {
      commit((p) => ({
        ...p,
        pages: p.pages.map((page) => {
          if (page.id !== pageId) return page;
          const from = page.groups.findIndex((g) => g.id === groupId);
          const to = from + delta;
          if (from < 0 || to < 0 || to >= page.groups.length) return page;
          const groups = [...page.groups];
          const [moved] = groups.splice(from, 1);
          if (moved) groups.splice(to, 0, moved);
          return { ...page, groups };
        }),
      }));
    },

    addItem(pageId, groupId, item) {
      commit((p) => ({
        ...p,
        pages: mapGroup(p.pages, pageId, groupId, (g) => ({ ...g, items: [...g.items, { ...item, id: item.id || uid("item") }] })),
      }));
    },

    updateItem(pageId, groupId, item) {
      commit((p) => ({
        ...p,
        pages: mapGroup(p.pages, pageId, groupId, (g) => ({
          ...g,
          items: g.items.map((i) => (i.id === item.id ? item : i)),
        })),
      }));
    },

    removeItem(pageId, groupId, itemId) {
      commit((p) => ({
        ...p,
        pages: mapGroup(p.pages, pageId, groupId, (g) => ({ ...g, items: g.items.filter((i) => i.id !== itemId) })),
      }));
    },

    /** 同一分组内排序，或跨分组/跨页搬移（拖拽落点决定）。 */
    moveItem(activeId, overId) {
      if (activeId === overId) return;
      commit((p) => {
        const from = findItem(p, activeId);
        const to = findItem(p, overId);
        if (!from || !to) return p;

        // 同组内：等价于 arrayMove(items, from.index, to.index)
        if (from.pageId === to.pageId && from.groupId === to.groupId) {
          return {
            ...p,
            pages: mapGroup(p.pages, from.pageId, from.groupId, (g) => {
              const items = [...g.items];
              const [moved] = items.splice(from.index, 1);
              if (moved) items.splice(to.index, 0, moved);
              return { ...g, items };
            }),
          };
        }

        const moving = findItemById(p, activeId);
        if (!moving) return p;

        const withoutItem = mapGroup(p.pages, from.pageId, from.groupId, (g) => ({
          ...g,
          items: g.items.filter((i) => i.id !== activeId),
        }));

        return {
          ...p,
          pages: mapGroup(withoutItem, to.pageId, to.groupId, (g) => {
            const items = [...g.items];
            items.splice(Math.min(to.index, items.length), 0, moving);
            return { ...g, items };
          }),
        };
      });
    },

    recordRecent(itemId) {
      commit((p) => {
        const entry: RecentEntry = { id: itemId, at: Date.now() };
        const recent = [entry, ...p.recent.filter((r) => r.id !== itemId)].slice(0, RECENT_MAX);
        return { ...p, recent };
      });
    },

    clearRecent() {
      commit((p) => ({ ...p, recent: [] }));
    },

    installPlugin(plugin) {
      commit((p) => {
        const others = p.plugins.filter((item) => item.id !== plugin.id);
        return { ...p, plugins: [...others, plugin] };
      });
    },

    removePlugin(pluginId) {
      commit((p) => {
        // 同时清掉引用了它的组件，避免格子里留一个永远加载不出来的空壳
        const pages = p.pages.map((page) => ({
          ...page,
          groups: page.groups.map((group) => ({
            ...group,
            items: group.items.filter((item) => !(item.kind === "widget" && item.pluginId === pluginId)),
          })),
        }));
        return { ...p, plugins: p.plugins.filter((item) => item.id !== pluginId), pages };
      });
    },

    togglePlugin(pluginId) {
      commit((p) => ({
        ...p,
        plugins: p.plugins.map((item) => (item.id === pluginId ? { ...item, enabled: !item.enabled } : item)),
      }));
    },

    updatePluginSettings(pluginId, patch) {
      commit((p) => ({
        ...p,
        plugins: p.plugins.map((item) =>
          item.id === pluginId ? { ...item, settings: { ...item.settings, ...patch } } : item
        ),
      }));
    },

    replacePage(pageId, groups) {
      commit((p) => ({ ...p, pages: p.pages.map((page) => (page.id === pageId ? { ...page, groups } : page)) }));
    },

    replaceProfile(profile) {
      set({ profile });
    },

    async reset() {
      const fresh = defaultProfile();
      set({ profile: fresh });
      await saveProfile(fresh);
    },
  };
});

/** 把 profile 变更防抖写回 IndexedDB。 */
export function persistProfile(getProfile: () => Profile | null, delay = 250): () => void {
  let timer: number | undefined;
  return () => {
    const profile = getProfile();
    if (!profile) return;
    window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      void saveProfile(profile);
    }, delay);
  };
}
