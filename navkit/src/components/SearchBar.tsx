import { useEffect, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import type { Engine } from "../types";

interface Props {
  engines: Engine[];
  defaultEngineId: string;
  onQueryChange: (query: string) => void;
  onEngineChange: (engineId: string) => void;
}

export default function SearchBar({ engines, defaultEngineId, onQueryChange, onEngineChange }: Props) {
  const t = useT();
  const [engineId, setEngineId] = useState(defaultEngineId);
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (engines.some((e) => e.id === defaultEngineId)) setEngineId(defaultEngineId);
  }, [defaultEngineId, engines]);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      const typing = target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA");
      if (event.key === "/" && !typing) {
        event.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
      }
      if (event.key === "Escape" && document.activeElement === inputRef.current) {
        setQuery("");
        onQueryChange("");
        inputRef.current?.blur();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onQueryChange]);

  const engine = engines.find((e) => e.id === engineId) ?? engines[0];

  function update(value: string) {
    setQuery(value);
    onQueryChange(value);
  }

  function submit(event: React.FormEvent) {
    event.preventDefault();
    const value = query.trim();
    if (!value) {
      inputRef.current?.focus();
      return;
    }
    window.location.href = engine.urlTemplate.replace("%s", encodeURIComponent(value));
  }

  return (
    <form className="search" onSubmit={submit} role="search" data-search-form>
      <select
        className="search__engine"
        value={engineId}
        onChange={(event) => {
          setEngineId(event.target.value);
          onEngineChange(event.target.value);
        }}
        aria-label={t("search.engineLabel")}
        data-engine
      >
        {engines.map((item) => (
          <option key={item.id} value={item.id}>
            {item.name}
          </option>
        ))}
      </select>

      <input
        ref={inputRef}
        className="search__input"
        type="search"
        value={query}
        placeholder={t("search.placeholder", { engine: engine.name })}
        aria-label={t("search.queryLabel")}
        autoComplete="off"
        spellCheck={false}
        onChange={(event) => update(event.target.value)}
        data-search-input
      />

      <button className="btn btn--primary" type="submit">
        {t("search.submit")}
      </button>
    </form>
  );
}
