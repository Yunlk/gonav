import { themePackSchema } from "./schema";
import type { ThemePack, ThemeTokens } from "../types";

/** 把当前令牌（含用户覆盖）打包成可分享的主题包。 */
export function buildThemePack(name: string, tokens: ThemeTokens): string {
  const pack: ThemePack = { format: "navkit-theme", version: 1, name, tokens };
  return JSON.stringify(pack, null, 2);
}

export function parseThemePack(text: string): { pack?: ThemePack; error?: string } {
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch (err) {
    return { error: `不是合法的 JSON：${(err as Error).message}` };
  }
  const parsed = themePackSchema.safeParse(json);
  if (!parsed.success) {
    return { error: parsed.error.issues.map((i) => `${i.path.join(".") || "(根)"}: ${i.message}`).join("；") };
  }
  return { pack: parsed.data as ThemePack };
}

/** 主题包 → CSS 变量覆盖表（导入后仍然可以继续手改）。 */
export function packToOverrides(pack: ThemePack): Record<string, string> {
  const overrides: Record<string, string> = {};
  for (const [key, value] of Object.entries(pack.tokens)) {
    overrides[key] = typeof value === "number" ? String(value) : String(value);
  }
  return overrides;
}
