import { useState } from "react";
import { useT } from "../lib/i18n";
import type { Page } from "../types";

interface Props {
  pages: Page[];
  activePageId: string;
  onSelect: (pageId: string) => void;
  onAdd: (title: string) => void;
  onRename: (pageId: string, title: string) => void;
  onRemove: (pageId: string) => void;
}

export default function PagesBar({ pages, activePageId, onSelect, onAdd, onRename, onRemove }: Props) {
  const t = useT();
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  return (
    <div className="pages" data-pages>
      {pages.map((page) =>
        editing === page.id ? (
          <input
            key={page.id}
            className="pages__input"
            value={draft}
            autoFocus
            onChange={(event) => setDraft(event.target.value)}
            onBlur={() => {
              onRename(page.id, draft.trim() || page.title);
              setEditing(null);
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter") event.currentTarget.blur();
              if (event.key === "Escape") setEditing(null);
            }}
          />
        ) : (
          <button
            key={page.id}
            type="button"
            className={`pages__tab${page.id === activePageId ? " is-active" : ""}`}
            onClick={() => onSelect(page.id)}
            onDoubleClick={() => {
              setEditing(page.id);
              setDraft(page.title);
            }}
            title={t("pages.renameHint")}
            data-page-tab={page.id}
          >
            {page.title}
            {pages.length > 1 && page.id === activePageId ? (
              <span
                className="pages__close"
                role="button"
                tabIndex={-1}
                aria-label={t("pages.remove", { name: page.title })}
                onClick={(event) => {
                  event.stopPropagation();
                  onRemove(page.id);
                }}
              >
                ×
              </span>
            ) : null}
          </button>
        )
      )}

      <button type="button" className="pages__add" onClick={() => onAdd("")} title={t("pages.add")} data-page-add>
        ＋
      </button>
    </div>
  );
}
