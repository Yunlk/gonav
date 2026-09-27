import { translate, type Locale } from "./i18n";
import type { InstalledPlugin } from "../types";

export type TFn = (key: string, vars?: Record<string, string | number>) => string;

export interface SourceItem {
  title: string;
  url: string;
  icon?: string;
  desc?: string;
}

export interface SourceGroup {
  title: string;
  items: SourceItem[];
}

/**
 * 数据源插件是**声明式**的：清单声明 `entry`（JSON 地址）与 `network:<origin>` 权限，
 * 由宿主去取数据并映射成图标。这样第三方不需要在我们的页面里执行代码。
 */
export async function runSourcePlugin(
  plugin: InstalledPlugin,
  locale: Locale,
  t?: TFn
): Promise<{ groups?: SourceGroup[]; error?: string }> {
  const tr: TFn = t ?? ((key, vars) => translate(locale, key, vars));

  if (!plugin.enabled) return { error: tr("source.error.disabled", { id: plugin.id }) };

  let entry: URL;
  try {
    entry = new URL(plugin.entryUrl, window.location.href);
  } catch {
    return { error: tr("source.error.entry", { url: plugin.entryUrl }) };
  }

  const sameOrigin = entry.origin === window.location.origin;
  if (!sameOrigin && !plugin.manifest.permissions.includes(`network:${entry.origin}`)) {
    return { error: tr("source.error.permission", { perm: `network:${entry.origin}` }) };
  }

  let payload: unknown;
  try {
    const response = await fetch(entry.toString(), { headers: { accept: "application/json" } });
    if (!response.ok) return { error: tr("source.error.fetch", { message: `HTTP ${response.status}` }) };
    payload = await response.json();
  } catch (err) {
    return { error: tr("source.error.fetch", { message: (err as Error).message }) };
  }

  const groups = mapPayload(payload);
  if (groups.length === 0) return { error: tr("source.error.shape") };
  return { groups };
}

/** 兼容两种形状：navkit 原生 groups，以及 gonav /api/links 的 categories。 */
function mapPayload(payload: unknown): SourceGroup[] {
  const data = payload as {
    groups?: { title?: string; items?: { title?: string; name?: string; url?: string; icon?: string; desc?: string }[] }[];
    categories?: { name?: string; links?: { name?: string; url?: string; icon?: string; desc?: string; title?: string }[] }[];
  };

  if (Array.isArray(data?.groups)) {
    return data.groups
      .map((group) => ({
        title: group.title ?? "导入分组",
        items: (group.items ?? [])
          .filter((item) => item.url)
          .map((item) => ({
            title: item.title ?? item.name ?? item.url ?? "",
            url: item.url ?? "",
            icon: item.icon,
            desc: item.desc,
          })),
      }))
      .filter((group) => group.items.length > 0);
  }

  if (Array.isArray(data?.categories)) {
    return data.categories
      .map((category) => ({
        title: category.name ?? "导入分组",
        items: (category.links ?? [])
          .filter((link) => link.url)
          .map((link) => ({
            title: link.name ?? link.title ?? link.url ?? "",
            url: link.url ?? "",
            icon: link.icon,
            desc: link.desc,
          })),
      }))
      .filter((group) => group.items.length > 0);
  }

  return [];
}
