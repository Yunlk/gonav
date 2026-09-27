import { THEMES } from "./defaults";
import type { Profile, ThemePreset, ThemeTokens } from "../types";

export function themeById(id: string): ThemePreset {
  return THEMES.find((t) => t.id === id) ?? THEMES[0];
}

const NUMERIC_KEYS = new Set<keyof ThemeTokens>(["radius", "tileRadius", "blur"]);

/** 主题令牌 = 预设 + 用户覆盖（高级模式改的就是这一层）。 */
export function themeTokens(profile: Profile): ThemeTokens {
  const base = { ...themeById(profile.themeId).tokens };
  for (const [key, value] of Object.entries(profile.themeOverrides ?? {})) {
    if (!(key in base)) continue;
    const typed = key as keyof ThemeTokens;
    if (NUMERIC_KEYS.has(typed)) {
      const numeric = Number(value);
      if (!Number.isNaN(numeric)) (base as Record<string, unknown>)[typed] = numeric;
    } else {
      (base as Record<string, unknown>)[typed] = value;
    }
  }
  return base;
}

/** 把主题令牌写到 :root 的 CSS 变量上，全站样式只依赖变量。 */
export function applyTheme(tokens: ThemeTokens, themeId: string): void {
  const root = document.documentElement;
  root.dataset.theme = themeId;
  root.style.setProperty("--nk-bg", tokens.bg);
  root.style.setProperty("--nk-surface", tokens.surface);
  root.style.setProperty("--nk-surface-strong", tokens.surfaceStrong);
  root.style.setProperty("--nk-fg", tokens.fg);
  root.style.setProperty("--nk-muted", tokens.muted);
  root.style.setProperty("--nk-rule", tokens.rule);
  root.style.setProperty("--nk-accent", tokens.accent);
  root.style.setProperty("--nk-radius", `${tokens.radius}px`);
  root.style.setProperty("--nk-tile-radius", `${tokens.tileRadius}px`);
  root.style.setProperty("--nk-blur", `${tokens.blur}px`);
  root.style.setProperty("--nk-shadow", tokens.shadow);
}

/** 主题令牌里可被用户在面板上直接改的项。 */
export const EDITABLE_TOKENS: { key: keyof ThemeTokens; label: string; type: "color" | "number" }[] = [
  { key: "bg", label: "背景", type: "color" },
  { key: "surface", label: "面板", type: "color" },
  { key: "surfaceStrong", label: "卡片", type: "color" },
  { key: "fg", label: "文字", type: "color" },
  { key: "muted", label: "次要文字", type: "color" },
  { key: "rule", label: "描边", type: "color" },
  { key: "accent", label: "强调色", type: "color" },
  { key: "radius", label: "圆角", type: "number" },
  { key: "tileRadius", label: "图标圆角", type: "number" },
  { key: "blur", label: "毛玻璃模糊", type: "number" },
];
