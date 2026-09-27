import { useEffect, useMemo, useRef, useState } from "react";
import { useT } from "../lib/i18n";

export interface Command {
  id: string;
  title: string;
  hint?: string;
  group: string;
  run: () => void;
}

/**
 * 命令面板：全键盘操作入口。
 * 打开后 ↑↓ 选择、Enter 执行、Esc 关闭，不需要碰鼠标。
 */
export default function CommandPalette({ commands, onClose }: { commands: Command[]; onClose: () => void }) {
  const t = useT();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return commands;
    const terms = needle.split(/\s+/);
    return commands.filter((command) => {
      const haystack = `${command.title} ${command.group} ${command.hint ?? ""}`.toLowerCase();
      return terms.every((term) => haystack.includes(term));
    });
  }, [commands, query]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    setCursor(0);
  }, [query]);

  useEffect(() => {
    listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: "nearest" });
  }, [cursor]);

  function run(index: number) {
    const command = filtered[index];
    if (!command) return;
    onClose();
    command.run();
  }

  return (
    <div className="palette" role="dialog" aria-modal="true" aria-label={t("app.foot.palette")} data-palette>
      <div className="modal__scrim" onClick={onClose} />
      <div className="palette__panel">
        <input
          ref={inputRef}
          className="palette__input"
          value={query}
          placeholder={t("palette.placeholder")}
          autoComplete="off"
          spellCheck={false}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setCursor((c) => (filtered.length ? (c + 1) % filtered.length : 0));
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              setCursor((c) => (filtered.length ? (c - 1 + filtered.length) % filtered.length : 0));
            } else if (event.key === "Enter") {
              event.preventDefault();
              run(cursor);
            } else if (event.key === "Escape") {
              event.preventDefault();
              onClose();
            }
          }}
          data-palette-input
        />

        <ul className="palette__list" ref={listRef} data-palette-list>
          {filtered.length === 0 ? <li className="palette__empty">{t("palette.empty")}</li> : null}
          {filtered.map((command, index) => (
            <li key={command.id}>
              <button
                type="button"
                className="palette__item"
                data-active={index === cursor}
                data-command={command.id}
                onMouseEnter={() => setCursor(index)}
                onClick={() => run(index)}
              >
                <span className="palette__group">{command.group}</span>
                <span className="palette__title">{command.title}</span>
                {command.hint ? <span className="palette__hint">{command.hint}</span> : null}
              </button>
            </li>
          ))}
        </ul>

        <p className="palette__foot">
          <span className="kbd">↑</span><span className="kbd">↓</span> {t("palette.foot")} · <span className="kbd">Enter</span> ·{" "}
          <span className="kbd">Esc</span>
        </p>
      </div>
    </div>
  );
}
