package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrRevisionConflict 表示调用方声明的 expected_revision 与当前版本不一致。
var ErrRevisionConflict = errors.New("配置版本冲突")

// ErrCategoryNotFound 表示补丁指向了不存在的分类。
var ErrCategoryNotFound = errors.New("分类不存在")

// ErrLinkNotFound 表示补丁要删除的入口不存在。
var ErrLinkNotFound = errors.New("入口不存在")

// fileFormat 是落盘用的 JSON 形状：只包含文档里承诺的字段，
// 不会把 CategoryID/ContentHash 这类派生字段写进配置文件。
type fileFormat struct {
	Site       Site            `json:"site"`
	Categories []*fileCategory `json:"categories"`
}

type fileCategory struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Icon        string      `json:"icon,omitempty"`
	Accent      string      `json:"accent,omitempty"`
	Description string      `json:"description,omitempty"`
	Links       []*fileLink `json:"links"`
}

type fileLink struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Icon   string   `json:"icon,omitempty"`
	Desc   string   `json:"desc,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Pinned bool     `json:"pinned,omitempty"`
}

// Marshal 把配置序列化成配置文件格式的 JSON（已规范化，可直接落盘）。
func Marshal(c *Catalog) ([]byte, error) {
	out := fileFormat{Site: c.Site, Categories: make([]*fileCategory, 0, len(c.Categories))}
	for _, cat := range c.Categories {
		fc := &fileCategory{
			ID:          cat.ID,
			Name:        cat.Name,
			Icon:        cat.Icon,
			Accent:      cat.Accent,
			Description: cat.Description,
			Links:       make([]*fileLink, 0, len(cat.Links)),
		}
		for _, link := range cat.Links {
			fc.Links = append(fc.Links, &fileLink{
				ID:     link.ID,
				Name:   link.Name,
				URL:    link.URL,
				Icon:   link.Icon,
				Desc:   link.Desc,
				Tags:   link.Tags,
				Pinned: link.Pinned,
			})
		}
		out.Categories = append(out.Categories, fc)
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化配置: %w", err)
	}
	return append(raw, '\n'), nil
}

// ReplaceRaw 用一段配置替换当前配置。
//
// 它走的是与文件加载**完全相同**的校验链路（Decode → normalize），
// 因此网关下发与手工改文件的行为一致：非法配置一律拒绝，且不会破坏线上可用的旧配置。
// expectedRevision 非 0 时做乐观并发控制；dryRun 只校验不落盘。
func (s *Store) ReplaceRaw(raw []byte, source string, expectedRevision uint64, dryRun bool) (*Catalog, error) {
	c, err := Decode(raw, "网关下发")
	if err != nil {
		return nil, err
	}
	if source == "" {
		source = SourceGateway
	}
	c.source = source

	if expectedRevision != 0 {
		s.mu.RLock()
		current := s.revision
		s.mu.RUnlock()
		if current != expectedRevision {
			return nil, fmt.Errorf("%w：当前 %d，请求 %d", ErrRevisionConflict, current, expectedRevision)
		}
	}

	if dryRun {
		return c, nil
	}

	// 落盘的是规范化之后的形状，保证文件与内存里的一致
	out, err := Marshal(c)
	if err != nil {
		return nil, err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return nil, fmt.Errorf("写入临时配置 %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return nil, fmt.Errorf("替换配置 %s: %w", s.path, err)
	}

	s.mu.Lock()
	s.revision++
	c.revision = s.revision
	c.sourcePath = s.path
	s.cur = c
	s.err = nil
	// 刚才是我们自己写的文件，主动更新 mtime 免得热更新再触发一次重载
	if fi, statErr := os.Stat(s.path); statErr == nil {
		s.mod = fi.ModTime()
	}
	s.mu.Unlock()

	s.notify(c)
	return c, nil
}

// PutAction 是补丁动作。
type PutAction int

// 补丁动作。
const (
	ActionUpsert PutAction = iota
	ActionDelete
)

// PatchLink 在现有配置上增删改单条入口，然后走与全量下发相同的写入路径。
func (s *Store) PatchLink(action PutAction, categoryID string, link Link) (*Catalog, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, fmt.Errorf("%w：category_id 不能为空", ErrCategoryNotFound)
	}

	// 通过文件格式往返拿到一份可变副本，避免改动正在服务的那份配置
	raw, err := Marshal(s.Current())
	if err != nil {
		return nil, err
	}
	var draft fileFormat
	if err := json.Unmarshal(raw, &draft); err != nil {
		return nil, fmt.Errorf("复制配置: %w", err)
	}

	var target *fileCategory
	for _, cat := range draft.Categories {
		if cat.ID == categoryID {
			target = cat
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	switch action {
	case ActionDelete:
		id := strings.TrimSpace(link.ID)
		kept := make([]*fileLink, 0, len(target.Links))
		found := false
		for _, item := range target.Links {
			if item.ID == id {
				found = true
				continue
			}
			kept = append(kept, item)
		}
		if !found {
			return nil, fmt.Errorf("%w: %s", ErrLinkNotFound, id)
		}
		target.Links = kept

	case ActionUpsert:
		id := strings.TrimSpace(link.ID)
		if id == "" {
			id = slugify(link.Name)
		}
		if id == "" {
			return nil, fmt.Errorf("入口缺少可用的 id（name 里没有字母数字）")
		}
		next := &fileLink{
			ID:     id,
			Name:   strings.TrimSpace(link.Name),
			URL:    strings.TrimSpace(link.URL),
			Icon:   link.Icon,
			Desc:   link.Desc,
			Tags:   link.Tags,
			Pinned: link.Pinned,
		}
		replaced := false
		for i, item := range target.Links {
			if item.ID == id {
				target.Links[i] = next
				replaced = true
				break
			}
		}
		if !replaced {
			target.Links = append(target.Links, next)
		}

	default:
		return nil, fmt.Errorf("未知的补丁动作 %d", action)
	}

	out, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化补丁结果: %w", err)
	}
	return s.ReplaceRaw(append(out, '\n'), SourceGateway, 0, false)
}
