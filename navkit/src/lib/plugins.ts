import { getImage, openPluginStore } from "./db";
import { THEMES } from "./defaults";
import { translate, type Locale } from "./i18n";
import { manifestSchema } from "./schema";
import type { InstalledPlugin, PluginManifest, PluginSetting, ThemeTokens } from "../types";

/** 内置插件目录：清单与代码都由 gonav 的 Go 服务托管。 */
export const BUILTIN_PLUGINS: { id: string; nameKey: string; hintKey: string; manifestUrl: string }[] = [
  {
    id: "clock",
    nameKey: "builtin.clock.name",
    hintKey: "builtin.clock.hint",
    manifestUrl: "/static/navkit-plugins/clock/plugin.json",
  },
  {
    id: "todo",
    nameKey: "builtin.todo.name",
    hintKey: "builtin.todo.hint",
    manifestUrl: "/static/navkit-plugins/todo/plugin.json",
  },
  {
    id: "gonav",
    nameKey: "builtin.gonav.name",
    hintKey: "builtin.gonav.hint",
    manifestUrl: "/static/navkit-plugins/gonav/plugin.json",
  },
];

export type HostContext = {
  pluginId: string;
  locale: string;
  version: string;
};

/** 权限是「能力字符串」，network: 前缀后面跟允许访问的源。 */
export function permissionLabel(permission: string, t: (key: string, vars?: Record<string, string | number>) => string): string {
  if (permission === "storage") return t("perms.storage");
  if (permission === "theme:read") return t("perms.theme");
  if (permission === "search:query") return t("perms.search");
  if (permission.startsWith("network:")) return t("perms.network", { origin: permission.slice("network:".length) });
  return permission;
}

export function hasPermission(manifest: PluginManifest, permission: string): boolean {
  return manifest.permissions.includes(permission);
}

/** 把清单里的相对 entry 解析成可加载的绝对地址。 */
export function resolveEntry(manifestUrl: string, entry: string): string {
  return new URL(entry, new URL(manifestUrl, window.location.href)).toString();
}

export async function fetchManifest(manifestUrl: string): Promise<{ manifest?: PluginManifest; error?: string }> {
  let raw: unknown;
  try {
    const response = await fetch(manifestUrl, { headers: { accept: "application/json" } });
    if (!response.ok) return { error: `读取清单失败：HTTP ${response.status}` };
    raw = await response.json();
  } catch (err) {
    return { error: `读取清单失败：${(err as Error).message}` };
  }

  const parsed = manifestSchema.safeParse(raw);
  if (!parsed.success) {
    return { error: parsed.error.issues.map((i) => `${i.path.join(".") || "(根)"}: ${i.message}`).join("；") };
  }
  return { manifest: parsed.data as PluginManifest };
}

export function buildInstalled(
  manifest: PluginManifest,
  manifestUrl: string,
  source: "builtin" | "url"
): InstalledPlugin {
  const settings: Record<string, string | number | boolean> = {};
  for (const item of manifest.settings ?? []) settings[item.key] = item.default;
  return {
    manifest,
    manifestUrl: new URL(manifestUrl, window.location.href).toString(),
    entryUrl: resolveEntry(manifestUrl, manifest.entry),
    enabled: true,
    settings,
    installedAt: Date.now(),
    source,
    id: manifest.id,
  };
}

/* ------------------------------------------------------------------ 存储 */

const STORAGE_PREFIX = "plugin:";

export async function pluginStorageGet(pluginId: string, key: string): Promise<unknown> {
  try {
    const store = await openPluginStore();
    const value = await store.get(`${STORAGE_PREFIX}${pluginId}`, key);
    return value ?? null;
  } catch {
    return null;
  }
}

export async function pluginStorageSet(pluginId: string, key: string, value: unknown): Promise<void> {
  const store = await openPluginStore();
  await store.put(value, `${STORAGE_PREFIX}${pluginId}`, key);
}

export async function pluginStorageRemove(pluginId: string, key: string): Promise<void> {
  const store = await openPluginStore();
  await store.delete(`${STORAGE_PREFIX}${pluginId}`, key);
}

export async function pluginStorageClear(pluginId: string): Promise<void> {
  const store = await openPluginStore();
  await store.clear(`${STORAGE_PREFIX}${pluginId}`);
}

/* --------------------------------------------------------------- 宿主 RPC */

export interface RpcRequest {
  method: string;
  params?: Record<string, unknown>;
}

/** 宿主侧 RPC 分发：所有能力都要先过权限清单，未声明的直接拒绝。 */
export async function handlePluginRpc(
  plugin: InstalledPlugin,
  request: RpcRequest,
  ctx: HostContext
): Promise<unknown> {
  const { manifest } = plugin;
  const params = request.params ?? {};
  const t = (key: string, vars?: Record<string, string | number>) =>
    translate(ctx.locale as Locale, key, vars);

  switch (request.method) {
    case "host.info":
      return { pluginId: manifest.id, apiVersion: manifest.apiVersion, hostVersion: ctx.version, locale: ctx.locale };

    case "storage.get":
      require(manifest, "storage", t);
      return pluginStorageGet(manifest.id, String(params.key ?? ""));

    case "storage.set":
      require(manifest, "storage", t);
      await pluginStorageSet(manifest.id, String(params.key ?? ""), params.value);
      return true;

    case "storage.remove":
      require(manifest, "storage", t);
      await pluginStorageRemove(manifest.id, String(params.key ?? ""));
      return true;

    case "theme.get": {
      require(manifest, "theme:read", t);
      const themeId = String(params.themeId ?? "");
      const preset = THEMES.find((t2) => t2.id === themeId) ?? THEMES[0];
      return preset.tokens satisfies ThemeTokens;
    }

    case "search.query": {
      require(manifest, "search:query", t);
      const q = String(params.q ?? "").trim();
      if (!q) return { matches: [] };
      const limit = Math.min(Number(params.limit ?? 10) || 10, 50);
      const response = await fetch(`/api/search?q=${encodeURIComponent(q)}&limit=${limit}`);
      if (!response.ok) throw new Error(t("plugin.error.search", { status: response.status }));
      return await response.json();
    }

    default:
      throw new Error(t("plugin.error.unsupported", { method: request.method }));
  }
}

function require(manifest: PluginManifest, permission: string, t: (key: string, vars?: Record<string, string | number>) => string): void {
  if (!hasPermission(manifest, permission)) {
    throw new Error(t("plugin.error.permission", { id: manifest.id, perm: permission }));
  }
}

/* ------------------------------------------------------------ widget 尺寸 */

export const WIDGET_UNIT = 92; // 与 .tile 的基准高度对齐

export function widgetPixels(size: { w: number; h: number }, tileSize: number) {
  return {
    width: Math.max(tileSize, size.w * (tileSize + 8)),
    height: Math.max(96, size.h * WIDGET_UNIT),
  };
}

/** 供数据源插件读取壁纸（目前只用于展示，避免权限过多）。 */
export async function readWallpaperThumb(imageId: string): Promise<Blob | null> {
  const stored = await getImage(imageId);
  return stored?.blob ?? null;
}

export function settingLabel(setting: PluginSetting): string {
  return setting.label || setting.key;
}
