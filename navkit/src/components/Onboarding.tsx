import { useState } from "react";
import { THEMES } from "../lib/defaults";
import { useT } from "../lib/i18n";

interface Props {
  onDone: (themeId: string) => void;
}

export default function Onboarding({ onDone }: Props) {
  const t = useT();
  const [selected, setSelected] = useState(THEMES[0].id);

  return (
    <div className="onboarding" data-onboarding>
      <div className="onboarding__inner">
        <p className="onboarding__eyebrow">{t("onboarding.eyebrow")}</p>
        <h1 className="onboarding__title">{t("onboarding.title")}</h1>
        <p className="onboarding__sub">{t("onboarding.sub")}</p>

        <ul className="themes" data-onboarding-themes>
          {THEMES.map((theme) => (
            <li key={theme.id}>
              <button
                type="button"
                className={`theme-card${selected === theme.id ? " is-selected" : ""}`}
                onClick={() => setSelected(theme.id)}
                data-theme-option={theme.id}
                aria-pressed={selected === theme.id}
              >
                <span
                  className="theme-card__preview"
                  style={{ background: theme.tokens.bg, borderColor: theme.tokens.rule }}
                >
                  <span className="theme-card__row" style={{ background: theme.tokens.surface, borderColor: theme.tokens.rule }} />
                  <span
                    className="theme-card__row theme-card__row--short"
                    style={{ background: theme.tokens.surface, borderColor: theme.tokens.rule }}
                  />
                  <span className="theme-card__dot" style={{ background: theme.tokens.accent }} />
                </span>
                <span className="theme-card__name">{t(theme.nameKey)}</span>
                <span className="theme-card__hint">{t(theme.hintKey)}</span>
              </button>
            </li>
          ))}
        </ul>

        <button className="btn btn--primary btn--lg" type="button" onClick={() => onDone(selected)} data-onboarding-start>
          {t("onboarding.start")}
        </button>
      </div>
    </div>
  );
}
