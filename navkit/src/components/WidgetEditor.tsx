import { useState } from "react";
import { useT } from "../lib/i18n";
import type { InstalledPlugin, WidgetItem } from "../types";

interface Props {
  item: WidgetItem;
  plugin?: InstalledPlugin;
  onSave: (next: WidgetItem, settings: Record<string, string | number | boolean>) => void;
  onDelete: () => void;
  onClose: () => void;
}

export default function WidgetEditor({ item, plugin, onSave, onDelete, onClose }: Props) {
  const t = useT();
  const [title, setTitle] = useState(item.title);
  const [w, setW] = useState(item.w);
  const [h, setH] = useState(item.h);
  const [settings, setSettings] = useState<Record<string, string | number | boolean>>(plugin?.settings ?? {});

  const fields = plugin?.manifest.settings ?? [];

  function submit(event: React.FormEvent) {
    event.preventDefault();
    onSave({ ...item, title: title.trim() || item.title, w, h }, settings);
  }

  return (
    <div className="modal" role="dialog" aria-modal="true" aria-label={t("widget.edit.title")} data-widget-editor>
      <div className="modal__scrim" onClick={onClose} />
      <form className="modal__panel" onSubmit={submit}>
        <header className="modal__head">
          <h2>
            {t("widget.edit.title")} · {plugin?.manifest.name ?? item.pluginId}
          </h2>
          <button type="button" className="modal__close" onClick={onClose} aria-label={t("widget.close")}>
            ×
          </button>
        </header>

        <label className="field">
          <span>{t("widget.form.name")}</span>
          <input value={title} onChange={(event) => setTitle(event.target.value)} data-widget-title />
        </label>

        <div className="field-row">
          <label className="field">
            <span>{t("widget.form.w", { n: w })}</span>
            <input type="range" min={1} max={4} value={w} onChange={(event) => setW(Number(event.target.value))} data-widget-w />
          </label>
          <label className="field">
            <span>{t("widget.form.h", { n: h })}</span>
            <input type="range" min={1} max={4} value={h} onChange={(event) => setH(Number(event.target.value))} data-widget-h />
          </label>
        </div>

        {fields.length > 0 ? <h4 className="panel__sub">{t("widget.form.settings")}</h4> : null}
        {fields.map((field) => {
          const value = settings[field.key];
          if (field.type === "boolean") {
            return (
              <label className="field field--inline" key={field.key}>
                <input
                  type="checkbox"
                  checked={Boolean(value)}
                  onChange={(event) => setSettings({ ...settings, [field.key]: event.target.checked })}
                  data-widget-setting={field.key}
                />
                <span>{field.label}</span>
              </label>
            );
          }
          return (
            <label className="field" key={field.key}>
              <span>{field.label}</span>
              <input
                type={field.type === "number" ? "number" : "text"}
                value={String(value ?? "")}
                onChange={(event) =>
                  setSettings({
                    ...settings,
                    [field.key]: field.type === "number" ? Number(event.target.value) : event.target.value,
                  })
                }
                data-widget-setting={field.key}
              />
            </label>
          );
        })}

        <footer className="modal__foot">
          <button type="button" className="btn btn--danger" onClick={onDelete} data-widget-delete>
            {t("widget.remove")}
          </button>
          <span className="modal__spacer" />
          <button type="button" className="btn" onClick={onClose}>
            {t("tile.cancel")}
          </button>
          <button type="submit" className="btn btn--primary" data-widget-save>
            {t("tile.save")}
          </button>
        </footer>
      </form>
    </div>
  );
}
