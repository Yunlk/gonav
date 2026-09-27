import { useState } from "react";
import { useT } from "../lib/i18n";
import type { Group, LinkItem } from "../types";

export interface TileDraft {
  title: string;
  url: string;
  icon: string;
  color: string;
  groupId: string;
}

interface Props {
  mode: "add" | "edit";
  groups: Group[];
  item?: LinkItem;
  groupId: string;
  onSave: (draft: TileDraft) => void;
  onDelete?: () => void;
  onClose: () => void;
}

function normalizeUrl(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return trimmed;
  return `https://${trimmed}`;
}

export default function TileEditor({ mode, groups, item, groupId, onSave, onDelete, onClose }: Props) {
  const t = useT();
  const [title, setTitle] = useState(item?.title ?? "");
  const [url, setUrl] = useState(item?.url ?? "");
  const [icon, setIcon] = useState(item?.icon ?? "");
  const [color, setColor] = useState(item?.color ?? "");
  const [targetGroup, setTargetGroup] = useState(groupId);
  const [error, setError] = useState("");

  function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!title.trim()) {
      setError(t("tile.error.title"));
      return;
    }
    const finalUrl = normalizeUrl(url);
    if (!finalUrl) {
      setError(t("tile.error.url"));
      return;
    }
    try {
      new URL(finalUrl);
    } catch {
      setError(t("tile.error.urlInvalid"));
      return;
    }
    onSave({ title: title.trim(), url: finalUrl, icon: icon.trim(), color: color.trim(), groupId: targetGroup });
  }

  return (
    <div className="modal" role="dialog" aria-modal="true" aria-label={t("tile.add.title")} data-tile-editor>
      <div className="modal__scrim" onClick={onClose} />
      <form className="modal__panel" onSubmit={submit}>
        <header className="modal__head">
          <h2>{mode === "add" ? t("tile.add.title") : t("tile.edit")}</h2>
          <button type="button" className="modal__close" onClick={onClose} aria-label={t("widget.close")}>
            ×
          </button>
        </header>

        <label className="field">
          <span>{t("tile.form.title")}</span>
          <input
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder={t("tile.form.titlePlaceholder")}
            autoFocus
            data-field-title
          />
        </label>

        <label className="field">
          <span>{t("tile.form.url")}</span>
          <input
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            placeholder={t("tile.form.urlPlaceholder")}
            data-field-url
          />
        </label>

        <label className="field">
          <span>{t("tile.form.icon")}</span>
          <input
            value={icon}
            onChange={(event) => setIcon(event.target.value)}
            placeholder={t("tile.form.iconHint")}
            data-field-icon
          />
        </label>

        <label className="field">
          <span>{t("tile.form.color")}</span>
          <input value={color} onChange={(event) => setColor(event.target.value)} placeholder="#2f6f9f" data-field-color />
        </label>

        {groups.length > 1 ? (
          <label className="field">
            <span>{t("tile.form.group")}</span>
            <select value={targetGroup} onChange={(event) => setTargetGroup(event.target.value)} data-field-group>
              {groups.map((group) => (
                <option key={group.id} value={group.id}>
                  {group.title}
                </option>
              ))}
            </select>
          </label>
        ) : null}

        {error ? <p className="field__error">{error}</p> : null}

        <footer className="modal__foot">
          {mode === "edit" && onDelete ? (
            <button type="button" className="btn btn--danger" onClick={onDelete} data-tile-delete>
              {t("tile.delete")}
            </button>
          ) : null}
          <span className="modal__spacer" />
          <button type="button" className="btn" onClick={onClose}>
            {t("tile.cancel")}
          </button>
          <button type="submit" className="btn btn--primary" data-tile-save>
            {t("tile.save")}
          </button>
        </footer>
      </form>
    </div>
  );
}
