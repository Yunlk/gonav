import { z } from "zod";
import type { Profile } from "../types";

const linkSchema = z.object({
  kind: z.literal("link").optional(),
  id: z.string().min(1),
  title: z.string().min(1, "标题不能为空"),
  url: z.string().min(1, "地址不能为空"),
  icon: z.string().default(""),
  color: z.string().optional(),
});

const widgetSchema = z.object({
  kind: z.literal("widget"),
  id: z.string().min(1),
  pluginId: z.string().min(1),
  title: z.string().default("组件"),
  w: z.number().min(1).max(4).default(1),
  h: z.number().min(1).max(4).default(1),
});

const itemSchema = z.union([widgetSchema, linkSchema]);

const groupSchema = z.object({
  id: z.string().min(1),
  title: z.string().default(""),
  items: z.array(itemSchema).default([]),
});

const pageSchema = z.object({
  id: z.string().min(1),
  title: z.string().default("页面"),
  groups: z.array(groupSchema).default([]),
});

const engineSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  urlTemplate: z.string().min(1),
});

const wallpaperSchema = z.union([
  z.object({ kind: z.literal("solid"), value: z.string() }),
  z.object({ kind: z.literal("gradient"), from: z.string(), to: z.string(), angle: z.number() }),
  z.object({ kind: z.literal("preset"), value: z.string() }),
  z.object({ kind: z.literal("image"), imageId: z.string(), blur: z.number(), dim: z.number() }),
]);

export const settingSchema = z.object({
  key: z.string().min(1),
  label: z.string().min(1),
  type: z.enum(["string", "number", "boolean"]),
  default: z.union([z.string(), z.number(), z.boolean()]),
});

export const manifestSchema = z.object({
  id: z
    .string()
    .min(1)
    .max(64)
    .regex(/^[a-z0-9][a-z0-9-_.]*$/, "id 只能用小写字母、数字、- _ ."),
  name: z.string().min(1).max(40),
  version: z.string().min(1).max(20),
  apiVersion: z.literal(1),
  kind: z.enum(["widget", "source"]),
  description: z.string().max(200).optional(),
  author: z.string().max(60).optional(),
  entry: z.string().min(1).max(200),
  permissions: z.array(z.string().max(200)).default([]),
  settings: z.array(settingSchema).default([]),
  defaultSize: z.object({ w: z.number().min(1).max(4), h: z.number().min(1).max(4) }).optional(),
});

const installedPluginSchema = z.object({
  id: z.string().min(1),
  manifest: manifestSchema,
  manifestUrl: z.string().min(1),
  entryUrl: z.string().min(1),
  enabled: z.boolean().default(true),
  settings: z.record(z.string(), z.union([z.string(), z.number(), z.boolean()])).default({}),
  installedAt: z.number().default(0),
  source: z.enum(["builtin", "url"]).default("url"),
});

export const profileSchema = z.object({
  version: z.literal(1),
  onboarded: z.boolean().default(true),
  themeId: z.string().default("bone"),
  themeOverrides: z.record(z.string(), z.string()).default({}),
  customCss: z.string().default(""),
  locale: z.enum(["zh-CN", "en"]).default("zh-CN"),
  tileSize: z.number().min(56).max(180).default(96),
  wallpaper: wallpaperSchema,
  engines: z.array(engineSchema).min(1, "至少要有一个搜索引擎"),
  defaultEngineId: z.string(),
  pages: z.array(pageSchema).min(1, "至少要有一个页面"),
  activePageId: z.string(),
  recent: z.array(z.object({ id: z.string(), at: z.number() })).default([]),
  plugins: z.array(installedPluginSchema).default([]),
});

export const exportSchema = z.object({
  format: z.literal("navkit"),
  version: z.literal(1),
  exportedAt: z.string(),
  profile: profileSchema,
  images: z.record(z.string(), z.string()).optional(),
});

export const themePackSchema = z.object({
  format: z.literal("navkit-theme"),
  version: z.literal(1),
  name: z.string().min(1),
  tokens: z.object({
    bg: z.string(),
    surface: z.string(),
    surfaceStrong: z.string(),
    fg: z.string(),
    muted: z.string(),
    rule: z.string(),
    accent: z.string(),
    radius: z.number(),
    tileRadius: z.number(),
    blur: z.number(),
    shadow: z.string(),
  }),
});

export interface ImportResult {
  ok: boolean;
  profile?: Profile;
  images?: Record<string, string>;
  errors: string[];
}

/** 解析导入文件：先按导出格式校验，失败再尝试当作裸 Profile（兼容手写配置）。 */
export function parseImported(raw: string): ImportResult {
  let json: unknown;
  try {
    json = JSON.parse(raw);
  } catch (err) {
    return { ok: false, errors: [`不是合法的 JSON：${(err as Error).message}`] };
  }

  const wrapped = exportSchema.safeParse(json);
  if (wrapped.success) {
    return { ok: true, profile: normalize(wrapped.data.profile as Profile), images: wrapped.data.images, errors: [] };
  }

  const bare = profileSchema.safeParse(json);
  if (bare.success) {
    return { ok: true, profile: normalize(bare.data as Profile), errors: [] };
  }

  const errors = bare.error.issues.slice(0, 6).map((issue) => {
    const where = issue.path.join(".") || "(根)";
    return `${where}: ${issue.message}`;
  });
  return { ok: false, errors };
}

/** 补齐可能缺省的派生字段，避免导入后引用了不存在的 id。 */
function normalize(profile: Profile): Profile {
  const pageIds = new Set(profile.pages.map((p) => p.id));
  const activePageId = pageIds.has(profile.activePageId) ? profile.activePageId : profile.pages[0].id;

  const engineIds = new Set(profile.engines.map((e) => e.id));
  const defaultEngineId = engineIds.has(profile.defaultEngineId) ? profile.defaultEngineId : profile.engines[0].id;

  // 最近访问只记链接（组件没有「访问」语义）
  const linkIds = new Set(
    profile.pages.flatMap((p) =>
      p.groups.flatMap((g) => g.items.filter((i) => i.kind !== "widget").map((i) => i.id))
    )
  );

  return {
    ...profile,
    activePageId,
    defaultEngineId,
    recent: profile.recent.filter((r) => linkIds.has(r.id)).slice(0, 20),
  };
}
