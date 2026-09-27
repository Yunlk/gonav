import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { handlePluginRpc } from "../lib/plugins";
import type { InstalledPlugin, ThemeTokens } from "../types";

const NS = "navkit-plugin";

interface Props {
  plugin: InstalledPlugin;
  height: number;
  tokens: ThemeTokens;
  themeId: string;
  locale: string;
  hostVersion: string;
  onLog?: (level: string, message: string) => void;
}

/**
 * 插件的沙箱宿主。
 *
 * 沙箱策略：`sandbox="allow-scripts"`（**故意不给 allow-same-origin**）——
 * iframe 拿到的是不透明源，既读不到宿主 DOM，也拿不到宿主的 cookie / IndexedDB。
 * 插件要数据只能通过 postMessage 走 RPC，而每个能力都要先在清单里声明权限。
 */
export default function PluginFrame({ plugin, height, tokens, themeId, locale, hostVersion, onLog }: Props) {
  const frameRef = useRef<HTMLIFrameElement>(null);
  const readyRef = useRef(false);
  const [failed, setFailed] = useState("");

  // 主题/设置/尺寸变化都要推给插件，所以放进 ref 供 message 回调读取最新值
  const stateRef = useRef({ plugin, tokens, themeId, locale, hostVersion, height });
  stateRef.current = { plugin, tokens, themeId, locale, hostVersion, height };

  useLayoutEffect(() => {
    function post(payload: Record<string, unknown>) {
      frameRef.current?.contentWindow?.postMessage({ ns: NS, v: 1, ...payload }, "*");
    }

    function sendInit() {
      const current = stateRef.current;
      post({
        type: "init",
        plugin: { id: current.plugin.id, name: current.plugin.manifest.name, version: current.plugin.manifest.version },
        settings: current.plugin.settings,
        theme: current.tokens,
        themeId: current.themeId,
        locale: current.locale,
        hostVersion: current.hostVersion,
        size: { height: current.height },
      });
    }

    async function onMessage(event: MessageEvent) {
      // 只认自己这个 iframe 发来的消息；沙箱 iframe 的 origin 是 "null"，不能靠 origin 判断
      if (!frameRef.current || event.source !== frameRef.current.contentWindow) return;
      const data = event.data;
      if (!data || data.ns !== NS) return;

      const current = stateRef.current;

      switch (data.type) {
        case "ready":
          readyRef.current = true;
          setFailed("");
          sendInit();
          return;

        case "rpc": {
          const id = String(data.id ?? "");
          try {
            const result = await handlePluginRpc(
              current.plugin,
              { method: String(data.method ?? ""), params: data.params ?? {} },
              { pluginId: current.plugin.id, locale: current.locale, version: current.hostVersion }
            );
            post({ type: "rpc:result", id, result });
          } catch (err) {
            post({ type: "rpc:error", id, error: (err as Error).message });
          }
          return;
        }

        case "log":
          onLog?.(String(data.level ?? "info"), String(data.message ?? ""));
          return;

        case "error":
          setFailed(String(data.message ?? "插件报告了错误"));
          return;

        default:
          return;
      }
    }

    window.addEventListener("message", onMessage);
    // 插件是在脚本末尾就发 ready 的，可能早于宿主挂上监听器；
    // 所以这里再兜一次：没收到 ready 就主动 init，插件侧的 init 处理是幂等的。
    const timer = window.setTimeout(() => {
      if (!readyRef.current) sendInit();
    }, 700);

    return () => {
      window.removeEventListener("message", onMessage);
      window.clearTimeout(timer);
    };
  }, [onLog]);

  useEffect(() => {
    frameRef.current?.contentWindow?.postMessage(
      { ns: NS, v: 1, type: "settings", settings: plugin.settings },
      "*"
    );
  }, [plugin.settings]);

  useEffect(() => {
    frameRef.current?.contentWindow?.postMessage({ ns: NS, v: 1, type: "theme", theme: tokens, themeId }, "*");
  }, [tokens, themeId]);

  return (
    <div className="widget__frame" data-plugin-frame={plugin.id}>
      {failed ? <p className="widget__error">{failed}</p> : null}
      <iframe
        ref={frameRef}
        title={plugin.manifest.name}
        src={plugin.entryUrl}
        sandbox="allow-scripts"
        referrerPolicy="no-referrer"
        loading="lazy"
        onError={() => setFailed("插件入口加载失败")}
      />
    </div>
  );
}
