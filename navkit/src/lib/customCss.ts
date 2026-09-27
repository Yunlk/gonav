/**
 * 极客模式：自定义 CSS。
 *
 * 严格 CSP 下 `style-src 'self'` 会拦掉 `<style>` 标签与内联 style 属性，
 * 但 **CSSOM 不受 CSP 限制**，所以用 adoptedStyleSheets 注入：
 * 既不放松 CSP，又能让用户完全接管样式。
 *
 * 自定义 JS 不走这条路——它会被当成「用户脚本插件」丢进沙箱 iframe 里跑。
 */
let sheet: CSSStyleSheet | null = null;

export function applyCustomCss(css: string): { ok: boolean; error?: string } {
  if (!("adoptedStyleSheets" in document)) {
    return { ok: false, error: "当前浏览器不支持 adoptedStyleSheets，无法注入自定义样式" };
  }
  if (!sheet) {
    sheet = new CSSStyleSheet();
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  }
  try {
    sheet.replaceSync(css);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: (err as Error).message };
  }
}
