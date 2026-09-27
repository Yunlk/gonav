import { listImages, putImage } from "./db";
import type { ExportedProfile, Profile } from "../types";

const MAX_EMBED_BYTES = 1_500_000;
const MAX_TOTAL_EMBED = 4_000_000;

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error ?? new Error("读取图片失败"));
    reader.readAsDataURL(blob);
  });
}

/** 打包成可分享的 JSON。图片可选内嵌（体积允许时），否则只导出引用。 */
export async function buildExport(profile: Profile, embedImages: boolean): Promise<string> {
  const payload: ExportedProfile = {
    format: "navkit",
    version: 1,
    exportedAt: new Date().toISOString(),
    profile,
  };

  if (embedImages) {
    const images = await listImages();
    const embedded: Record<string, string> = {};
    let total = 0;
    for (const image of images) {
      if (image.blob.size > MAX_EMBED_BYTES) continue;
      if (total + image.blob.size > MAX_TOTAL_EMBED) break;
      embedded[image.id] = await blobToDataUrl(image.blob);
      total += image.blob.size;
    }
    if (Object.keys(embedded).length > 0) payload.images = embedded;
  }

  return JSON.stringify(payload, null, 2);
}

export function downloadText(filename: string, text: string): void {
  const blob = new Blob([text], { type: "application/json;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.rel = "noopener";
  document.body.appendChild(anchor);
  anchor.click();

  // 立刻移除 a 或过早 revoke blob 都会让浏览器取消下载（headless 尤其明显），
  // 所以延后清理：a 留 3 秒，blob 留 60 秒。
  window.setTimeout(() => anchor.remove(), 3000);
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

/** 把导出文件里内嵌的图片写回 IndexedDB，返回恢复数量。 */
export async function restoreImages(images: Record<string, string> | undefined): Promise<number> {
  if (!images) return 0;
  let count = 0;
  for (const [id, dataUrl] of Object.entries(images)) {
    try {
      const blob = await (await fetch(dataUrl)).blob();
      await putImage({ id, name: "imported", blob });
      count += 1;
    } catch {
      // 单张图片失败不影响整体导入
    }
  }
  return count;
}
