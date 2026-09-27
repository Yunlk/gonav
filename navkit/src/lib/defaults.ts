import type { Engine, Profile, ThemePreset, Wallpaper } from "../types";

export const THEMES: ThemePreset[] = [
  {
    id: "bone",
    name: "克制灰白",
    nameKey: "theme.bone.name",
    hint: "细线 · 小圆角 · 低调蓝",
    hintKey: "theme.bone.hint",
    tokens: {
      bg: "#f7f5f1",
      surface: "#ffffff",
      surfaceStrong: "#ffffff",
      fg: "#1c1c1e",
      muted: "#6b6b70",
      rule: "#e3dfd7",
      accent: "#2f6f9f",
      radius: 10,
      tileRadius: 10,
      blur: 0,
      shadow: "0 1px 2px rgba(28,28,30,.06)",
    },
  },
  {
    id: "ink",
    name: "墨黑",
    nameKey: "theme.ink.name",
    hint: "深色底 · 低对比",
    hintKey: "theme.ink.hint",
    tokens: {
      bg: "#131518",
      surface: "#1b1e22",
      surfaceStrong: "#22262b",
      fg: "#e8e8e6",
      muted: "#a2a6aa",
      rule: "#2c3138",
      accent: "#6fa8d6",
      radius: 10,
      tileRadius: 10,
      blur: 0,
      shadow: "0 1px 2px rgba(0,0,0,.35)",
    },
  },
  {
    id: "glass",
    name: "毛玻璃",
    nameKey: "theme.glass.name",
    hint: "壁纸打底 · 半透明卡片",
    hintKey: "theme.glass.hint",
    tokens: {
      bg: "#101317",
      surface: "rgba(255,255,255,.10)",
      surfaceStrong: "rgba(255,255,255,.16)",
      fg: "#f4f5f7",
      muted: "rgba(244,245,247,.72)",
      rule: "rgba(255,255,255,.18)",
      accent: "#8ec5ff",
      radius: 14,
      tileRadius: 16,
      blur: 18,
      shadow: "0 8px 30px rgba(0,0,0,.28)",
    },
  },
  {
    id: "paper",
    name: "纸张",
    nameKey: "theme.paper.name",
    hint: "米色底 · 更暖的字",
    hintKey: "theme.paper.hint",
    tokens: {
      bg: "#f3ead7",
      surface: "#fffaf0",
      surfaceStrong: "#fffdf8",
      fg: "#2a2418",
      muted: "#6f6553",
      rule: "#ded2b8",
      accent: "#a8632b",
      radius: 6,
      tileRadius: 6,
      blur: 0,
      shadow: "0 1px 2px rgba(42,36,24,.08)",
    },
  },
];

export const DEFAULT_WALLPAPERS: { id: string; name: string; css: string }[] = [
  { id: "sunset", name: "落日", css: "linear-gradient(160deg,#ff9a3c 0%,#ff5f6d 45%,#42275a 100%)" },
  { id: "ocean", name: "深海", css: "linear-gradient(200deg,#0f2027 0%,#203a43 45%,#2c5364 100%)" },
  { id: "mesh", name: "网格", css: "radial-gradient(circle at 18% 22%,#6ee7ff55,transparent 55%),radial-gradient(circle at 82% 30%,#ff7be555,transparent 55%),linear-gradient(#0b0b0b,#0b0b0b)" },
  { id: "mono", name: "素灰", css: "linear-gradient(140deg,#f1ede4,#cfc9be)" },
  { id: "forest", name: "林间", css: "linear-gradient(180deg,#0b3d2e,#1f6f43 60%,#9bd36a)" },
  { id: "dusk", name: "暮色", css: "linear-gradient(150deg,#2b1055,#7597de)" },
];

export const DEFAULT_ENGINES: Engine[] = [
  { id: "internal", name: "本站导航", urlTemplate: "/search?q=%s" },
  { id: "baidu", name: "百度", urlTemplate: "https://www.baidu.com/s?wd=%s" },
  { id: "bing", name: "必应", urlTemplate: "https://cn.bing.com/search?q=%s" },
  { id: "google", name: "Google", urlTemplate: "https://www.google.com/search?q=%s" },
  { id: "cnki", name: "知网", urlTemplate: "https://kns.cnki.net/kns8s/defaultresult/index?kw=%s" },
  { id: "github", name: "GitHub", urlTemplate: "https://github.com/search?q=%s" },
];

const DEFAULT_WALLPAPER: Wallpaper = {
  kind: "gradient",
  from: "#1f2a44",
  to: "#7f8fa6",
  angle: 150,
};

export function uid(prefix = "id"): string {
  return `${prefix}_${Math.random().toString(36).slice(2, 9)}`;
}

export function defaultProfile(): Profile {
  const main = {
    id: uid("group"),
    title: "常用",
    items: [
      { id: uid("link"), title: "图书馆官网", url: "https://library.neusoft.edu.cn/", icon: "📚" },
      { id: uid("link"), title: "教务系统", url: "https://teach.neusoft.edu.cn/", icon: "🗓" },
      { id: uid("link"), title: "选课系统", url: "https://xk.neusoft.edu.cn/xsxk/profile/index.html", icon: "✅" },
      { id: uid("link"), title: "智慧教育平台", url: "https://study.neusoft.edu.cn/login?redirect=/index", icon: "🎓" },
      { id: uid("link"), title: "中国知网", url: "https://www.cnki.net/", icon: "📄" },
    ],
  };
  const campus = {
    id: uid("group"),
    title: "校内",
    items: [
      { id: uid("link"), title: "综合服务门户", url: "https://qes.neusoft.edu.cn/qes/portal/index.do", icon: "🏛" },
      { id: uid("link"), title: "学生邮箱", url: "https://mail.dnui.edu.cn/", icon: "✉️" },
      { id: uid("link"), title: "DNUI 知识库", url: "https://dnuikb.neusoft.edu.cn/", icon: "💡" },
      { id: uid("link"), title: "学校官网", url: "https://www.neusoft.edu.cn/", icon: "🌐" },
    ],
  };
  const page = { id: uid("page"), title: "主页", groups: [main, campus] };

  return {
    version: 1,
    onboarded: false,
    themeId: "bone",
    themeOverrides: {},
    customCss: "",
    locale: "zh-CN",
    tileSize: 96,
    wallpaper: DEFAULT_WALLPAPER,
    engines: DEFAULT_ENGINES.map((e) => ({ ...e })),
    defaultEngineId: "internal",
    pages: [page],
    activePageId: page.id,
    recent: [],
    plugins: [],
  };
}
