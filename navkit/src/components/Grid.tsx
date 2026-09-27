import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, rectSortingStrategy, sortableKeyboardCoordinates, useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import PluginFrame from "./PluginFrame";
import { useT } from "../lib/i18n";
import type { Group, InstalledPlugin, LinkItem, NavItem, ThemeTokens, WidgetItem } from "../types";
import { isWidget } from "../types";

function iconNode(item: LinkItem) {
  const icon = item.icon?.trim() ?? "";
  if (/^https?:\/\//i.test(icon) || icon.startsWith("data:")) {
    return <img className="tile__img" src={icon} alt="" loading="lazy" />;
  }
  return <span className="tile__glyph">{icon || item.title.trim().charAt(0) || "?"}</span>;
}

/** 所有格子单元共用的可排序外壳（链接与组件都走它）。 */
function SortableCell({
  id,
  sortable,
  span,
  accent,
  children,
}: {
  id: string;
  sortable: boolean;
  span?: number;
  accent?: string;
  children: React.ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled: !sortable,
  });

  return (
    <li
      ref={setNodeRef}
      className={`tile-cell${isDragging ? " is-dragging" : ""}`}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        ...(span && span > 1 ? { gridColumn: `span ${span}` } : {}),
        ...(accent ? ({ "--tile-accent": accent } as React.CSSProperties) : {}),
      }}
      data-tile={id}
      {...attributes}
      {...listeners}
    >
      {children}
    </li>
  );
}

interface Props {
  groups: Group[];
  recent: LinkItem[];
  plugins: InstalledPlugin[];
  tileSize: number;
  query: string;
  tokens: ThemeTokens;
  themeId: string;
  locale: string;
  hostVersion: string;
  /** 可视化布局编辑模式：显示分组排序、组件尺寸等内联控件 */
  editMode: boolean;
  onOpen: (item: LinkItem) => void;
  onEdit: (item: LinkItem, groupId: string) => void;
  onConfigureWidget: (item: WidgetItem, groupId: string) => void;
  onAdd: (groupId: string) => void;
  onAddWidget: (groupId: string) => void;
  onRenameGroup: (groupId: string, title: string) => void;
  onRemoveGroup: (groupId: string) => void;
  onMoveGroup: (groupId: string, delta: number) => void;
  onResizeWidget: (item: WidgetItem, groupId: string, dw: number, dh: number) => void;
  onClearRecent: () => void;
  onMove: (activeId: string, overId: string) => void;
}

export default function Grid({
  groups,
  recent,
  plugins,
  tileSize,
  query,
  tokens,
  themeId,
  locale,
  hostVersion,
  editMode,
  onOpen,
  onEdit,
  onConfigureWidget,
  onAdd,
  onAddWidget,
  onRenameGroup,
  onRemoveGroup,
  onMoveGroup,
  onResizeWidget,
  onClearRecent,
  onMove,
}: Props) {
  const t = useT();
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const needle = query.trim().toLowerCase();
  const filtering = needle.length > 0;

  function matches(item: NavItem) {
    if (!filtering) return true;
    const haystack = isWidget(item) ? item.title : `${item.title} ${item.url}`;
    return haystack.toLowerCase().includes(needle);
  }

  const visibleGroups = groups
    .map((group) => ({ ...group, items: group.items.filter(matches) }))
    .filter((group) => !filtering || group.items.length > 0);

  const visibleRecent = filtering ? [] : recent;
  const empty = filtering && visibleGroups.length === 0;

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (over && active.id !== over.id) onMove(String(active.id), String(over.id));
  }

  function renderItem(item: NavItem, groupId: string) {
    if (isWidget(item)) {
      const plugin = plugins.find((p) => p.id === item.pluginId && p.enabled);
      return (
        <SortableCell key={item.id} id={item.id} sortable={!filtering} span={item.w}>
          <div className="widget" style={{ height: item.h * 92 }} data-widget={item.id}>
            <div className="widget__bar">
              <span className="widget__name">
                {plugin ? plugin.manifest.name : t("grid.widget.missingShort", { id: item.pluginId })}
              </span>
              {editMode ? (
                <span className="widget__size" data-widget-size={item.id}>
                  <button type="button" className="widget__btn" title="−" onClick={() => onResizeWidget(item, groupId, -1, 0)}>
                    ◀
                  </button>
                  <span className="widget__dims">
                    {item.w}×{item.h}
                  </span>
                  <button type="button" className="widget__btn" title="+" onClick={() => onResizeWidget(item, groupId, 1, 0)}>
                    ▶
                  </button>
                  <button type="button" className="widget__btn" title="−" onClick={() => onResizeWidget(item, groupId, 0, -1)}>
                    ▲
                  </button>
                  <button type="button" className="widget__btn" title="+" onClick={() => onResizeWidget(item, groupId, 0, 1)}>
                    ▼
                  </button>
                </span>
              ) : null}
              <button
                type="button"
                className="widget__btn"
                title={t("grid.widget.configure")}
                data-widget-configure={item.id}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  onConfigureWidget(item, groupId);
                }}
              >
                ⚙
              </button>
            </div>
            {plugin ? (
              <PluginFrame
                plugin={plugin}
                height={item.h * 92}
                tokens={tokens}
                themeId={themeId}
                locale={locale}
                hostVersion={hostVersion}
              />
            ) : (
              <p className="widget__error">{t("grid.widget.missing", { id: item.pluginId })}</p>
            )}
          </div>
        </SortableCell>
      );
    }

    return (
      <SortableCell key={item.id} id={item.id} sortable={!filtering} accent={item.color}>
        <a
          className="tile"
          href={item.url}
          target="_blank"
          rel="noopener noreferrer"
          onClick={() => onOpen(item)}
          style={{ minHeight: tileSize }}
          data-tile-link={item.id}
        >
          <span className="tile__icon">{iconNode(item)}</span>
          <span className="tile__title">{item.title}</span>
        </a>
        <button
          type="button"
          className="tile__edit"
          aria-label={`${t("tile.edit")} ${item.title}`}
          title={t("tile.edit")}
          data-tile-edit={item.id}
          onClick={(event) => {
            event.preventDefault();
            event.stopPropagation();
            onEdit(item, groupId);
          }}
        >
          ✎
        </button>
      </SortableCell>
    );
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <div className="grid" data-grid>
        {visibleRecent.length > 0 ? (
          <section className="group group--recent" data-group="recent">
            <header className="group__head">
              <h2>{t("grid.recent")}</h2>
              <span className="group__count">{visibleRecent.length}</span>
              <button type="button" className="link-btn" onClick={onClearRecent} data-recent-clear>
                {t("grid.clear")}
              </button>
            </header>
            <ul className="tiles" style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${tileSize}px, 1fr))` }}>
              {visibleRecent.map((item) => (
                <li key={`recent-${item.id}`} className="tile-cell">
                  <a className="tile" href={item.url} target="_blank" rel="noopener noreferrer" onClick={() => onOpen(item)}>
                    <span className="tile__icon">{iconNode(item)}</span>
                    <span className="tile__title">{item.title}</span>
                  </a>
                </li>
              ))}
            </ul>
          </section>
        ) : null}

        {visibleGroups.map((group, index) => (
          <section className="group" key={group.id} data-group={group.id} data-edit-mode={editMode ? "on" : undefined}>
            <header className="group__head">
              <h2
                contentEditable={!filtering}
                suppressContentEditableWarning
                onBlur={(event) => {
                  const title = event.currentTarget.textContent?.trim() ?? group.title;
                  if (title !== group.title) onRenameGroup(group.id, title);
                }}
                title={filtering ? undefined : t("grid.renameHint")}
              >
                {group.title}
              </h2>
              <span className="group__count">{group.items.length}</span>
              {!filtering ? (
                <>
                  {editMode ? (
                    <>
                      <button
                        type="button"
                        className="group__btn"
                        onClick={() => onMoveGroup(group.id, -1)}
                        disabled={index === 0}
                        title={t("grid.moveUp")}
                        data-group-up={group.id}
                      >
                        ↑
                      </button>
                      <button
                        type="button"
                        className="group__btn"
                        onClick={() => onMoveGroup(group.id, 1)}
                        disabled={index === visibleGroups.length - 1}
                        title={t("grid.moveDown")}
                        data-group-down={group.id}
                      >
                        ↓
                      </button>
                      <button
                        type="button"
                        className="group__btn"
                        onClick={() => onAddWidget(group.id)}
                        title={t("app.action.addWidget")}
                        data-group-add-widget={group.id}
                      >
                        ▦
                      </button>
                    </>
                  ) : null}
                  <button type="button" className="group__btn" onClick={() => onAdd(group.id)} title={t("grid.addLink")}>
                    ＋
                  </button>
                  <button
                    type="button"
                    className="group__btn"
                    onClick={() => onRemoveGroup(group.id)}
                    title={t("grid.removeGroup")}
                  >
                    ×
                  </button>
                </>
              ) : null}
            </header>

            <SortableContext items={group.items.map((item) => item.id)} strategy={rectSortingStrategy}>
              <ul className="tiles" style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${tileSize}px, 1fr))` }}>
                {group.items.map((item) => renderItem(item, group.id))}
                {!filtering ? (
                  <li className="tile-cell tile-cell--add">
                    <button type="button" className="tile tile--add" onClick={() => onAdd(group.id)}>
                      <span className="tile__icon tile__glyph">＋</span>
                      <span className="tile__title">{t("grid.add")}</span>
                    </button>
                  </li>
                ) : null}
              </ul>
            </SortableContext>
          </section>
        ))}

        {empty ? (
          <p className="empty" data-empty>
            {t("grid.empty")}
          </p>
        ) : null}
      </div>
    </DndContext>
  );
}
