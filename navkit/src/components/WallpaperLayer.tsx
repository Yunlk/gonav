import { useEffect, useState } from "react";
import { DEFAULT_WALLPAPERS } from "../lib/defaults";
import { getImage } from "../lib/db";
import type { ThemeTokens, Wallpaper } from "../types";

export function wallpaperCss(wallpaper: Wallpaper): string {
  switch (wallpaper.kind) {
    case "solid":
      return wallpaper.value;
    case "gradient":
      return `linear-gradient(${wallpaper.angle}deg, ${wallpaper.from}, ${wallpaper.to})`;
    case "preset":
      return DEFAULT_WALLPAPERS.find((p) => p.id === wallpaper.value)?.css ?? "#111";
    case "image":
      return "#0e1114";
  }
}

/** 背景层：CSS 壁纸与上传图片两种形态，上传图带模糊与压暗。 */
export default function WallpaperLayer({ wallpaper, tokens }: { wallpaper: Wallpaper; tokens: ThemeTokens }) {
  const [url, setUrl] = useState<string | null>(null);

  useEffect(() => {
    if (wallpaper.kind !== "image") {
      setUrl(null);
      return;
    }
    let revoked: string | null = null;
    let cancelled = false;

    void (async () => {
      const stored = await getImage(wallpaper.imageId);
      if (!stored || cancelled) return;
      const objectUrl = URL.createObjectURL(stored.blob);
      revoked = objectUrl;
      setUrl(objectUrl);
    })();

    return () => {
      cancelled = true;
      if (revoked) URL.revokeObjectURL(revoked);
      setUrl(null);
    };
  }, [wallpaper]);

  if (wallpaper.kind === "image") {
    return (
      <div className="wallpaper" aria-hidden="true">
        {url ? (
          <img
            className="wallpaper__img"
            src={url}
            alt=""
            style={{
              filter: `blur(${wallpaper.blur}px)`,
              transform: wallpaper.blur > 0 ? "scale(1.06)" : undefined,
            }}
          />
        ) : (
          <div className="wallpaper__img wallpaper__img--missing" style={{ background: tokens.bg }} />
        )}
        <div className="wallpaper__dim" style={{ opacity: wallpaper.dim / 100 }} />
      </div>
    );
  }

  return <div className="wallpaper" aria-hidden="true" style={{ background: wallpaperCss(wallpaper) }} />;
}
