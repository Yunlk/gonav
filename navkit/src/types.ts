export interface LinkItem {
  /** 可选：老配置里没有这个字段，缺失即视为链接 */
  kind?: "link";
  id: string;
  title: string;
  url: string;
  /** emoji、单个汉字或图片 URL；留空时取标题首字 */
  icon: string;
  /** 覆盖主题强调色 */
  color?: string;
}

/** 组件实例：占格子的插件宿主，参与拖拽排序。 */
export interface WidgetItem {
  kind: "widget";
  id: string;
  pluginId: string;
  title: string;
  /** 占几列 / 几行 */
  w: number;
  h: number;
}

export type NavItem = LinkItem | WidgetItem;

export function isWidget(item: NavItem): item is WidgetItem {
  return item.kind === "widget";
}

export interface Group {
  id: string;
  title: string;
  items: NavItem[];
}

export interface Page {
  id: string;
  title: string;
  groups: Group[];
}

export interface Engine {
  id: string;
  name: string;
  /** 查询地址模板，%s 会被替换成转义后的关键词 */
  urlTemplate: string;
}

export type Wallpaper =
  | { kind: "solid"; value: string }
  | { kind: "gradient"; from: string; to: string; angle: number }
  | { kind: "preset"; value: string }
  | { kind: "image"; imageId: string; blur: number; dim: number };

export interface RecentEntry {
  id: string;
  at: number;
}

/* ------------------------------------------------------------------ 插件 */

export type PluginKind = "widget" | "source";

export interface PluginSetting {
  key: string;
  label: string;
  type: "string" | "number" | "boolean";
  default: string | number | boolean;
}

export interface PluginManifest {
  id: string;
  name: string;
  version: string;
  apiVersion: 1;
  kind: PluginKind;
  description?: string;
  author?: string;
  entry: string;
  /** storage / theme:read / search:query / network:<origin> */
  permissions: string[];
  settings?: PluginSetting[];
  defaultSize?: { w: number; h: number };
}

export interface InstalledPlugin {
  id: string;
  manifest: PluginManifest;
  manifestUrl: string;
  /** 已解析成绝对地址的入口 */
  entryUrl: string;
  enabled: boolean;
  settings: Record<string, string | number | boolean>;
  installedAt: number;
  source: "builtin" | "url";
}

export interface Profile {
  /** 配置格式版本，导入时用于兼容判断 */
  version: 1;
  onboarded: boolean;
  themeId: string;
  /** CSS 变量覆盖（高级模式） */
  themeOverrides: Record<string, string>;
  /** 极客模式：自定义 CSS（走 CSSOM 注入） */
  customCss: string;
  locale: "zh-CN" | "en";
  tileSize: number;
  wallpaper: Wallpaper;
  engines: Engine[];
  defaultEngineId: string;
  pages: Page[];
  activePageId: string;
  recent: RecentEntry[];
  plugins: InstalledPlugin[];
}

export interface ThemeTokens {
  bg: string;
  surface: string;
  surfaceStrong: string;
  fg: string;
  muted: string;
  rule: string;
  accent: string;
  radius: number;
  tileRadius: number;
  blur: number;
  shadow: string;
}

export interface ThemePreset {
  id: string;
  name: string;
  /** 界面文案的字典 key（name/hint 只作为兜底） */
  nameKey: string;
  hint: string;
  hintKey: string;
  tokens: ThemeTokens;
}

export interface ThemePack {
  format: "navkit-theme";
  version: 1;
  name: string;
  tokens: ThemeTokens;
}

export interface ExportedProfile {
  format: "navkit";
  version: 1;
  exportedAt: string;
  profile: Profile;
  /** 仅在体积允许时内嵌的上传壁纸 */
  images?: Record<string, string>;
}
