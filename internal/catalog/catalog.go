// Package catalog 负责加载、校验、热更新并检索导航链接配置。
package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gonav/internal/pinyin"
)

// Link 描述一条导航入口。
type Link struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Icon   string   `json:"icon"`
	Desc   string   `json:"desc"`
	Tags   []string `json:"tags"`
	Pinned bool     `json:"pinned"`

	// 由所属分类回填。
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	Accent       string `json:"accent"`

	pyFull    string // 名称全拼
	pyInit    string // 名称拼音首字母
	tagPyFull string // 标签全拼（空格分隔）
	tagPyInit string // 标签拼音首字母（空格分隔）
	pyCatFull string // 所属分类全拼
	pyCatInit string // 所属分类拼音首字母
}

// Py 返回名称与标签的拼音检索串，供前端实时筛选使用。
func (l *Link) Py() string {
	return strings.TrimSpace(l.pyFull + " " + l.pyInit + " " + l.tagPyFull + " " + l.tagPyInit)
}

// Host 返回链接主机名。
func (l *Link) Host() string {
	u, err := url.Parse(l.URL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// Category 是业务域分组。
type Category struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Icon        string  `json:"icon"`
	Accent      string  `json:"accent"`
	Description string  `json:"description"`
	Links       []*Link `json:"links"`

	pyFull string
	pyInit string
}

// Site 是站点级文案配置。
type Site struct {
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	Logo         string `json:"logo"`
	Announcement string `json:"announcement"`
	Footer       string `json:"footer"`
}

// Catalog 是一份完整的导航配置。
type Catalog struct {
	Site       Site        `json:"site"`
	Categories []*Category `json:"categories"`

	links      []*Link
	byLinkID   map[string]*Link
	byCatID    map[string]*Category
	updatedAt  time.Time
	sourcePath string

	// 同步语义：每份快照带单调递增版本号与内容哈希，供 gRPC 网关判断过期。
	revision    uint64
	contentHash string
	source      string
}

// SourceFile / SourceGateway 标记这份配置是本地文件热更新来的，还是网关下发的。
const (
	SourceFile    = "file"
	SourceGateway = "gateway"
)

// Accents 是允许的主题强调色，取值为 rare-ui 调色板的语义名。
var Accents = []string{"violet", "sky", "emerald", "amber", "rose", "cyan", "fuchsia", "lime"}

var accentSet = func() map[string]bool {
	m := make(map[string]bool, len(Accents))
	for _, a := range Accents {
		m[a] = true
	}
	return m
}()

const defaultAccent = "violet"

// Load 从 path 读取并校验配置。
func Load(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	c, err := decode(raw, path)
	if err != nil {
		return nil, err
	}
	c.sourcePath = path
	c.source = SourceFile
	return c, nil
}

// Decode 解析并校验一段配置字节，origin 只用于错误信息与来源标记。
// gRPC 的 PutCatalog 与文件加载共用这一条校验链路，保证两条写入路径行为一致。
func Decode(raw []byte, origin string) (*Catalog, error) {
	return decode(raw, origin)
}

func decode(raw []byte, origin string) (*Catalog, error) {
	var c Catalog
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("解析配置 %s: %w", origin, err)
	}
	if err := c.normalize(); err != nil {
		return nil, fmt.Errorf("校验配置 %s: %w", origin, err)
	}
	c.updatedAt = time.Now()
	c.contentHash = hashOf(raw)
	return &c, nil
}

// Parse 从内存字节解析配置，主要供测试使用。
func Parse(raw []byte) (*Catalog, error) {
	c, err := decode(raw, "配置")
	if err != nil {
		return nil, err
	}
	return c, nil
}

// hashOf 返回配置内容摘要，用于让网关判断两份配置是否等价。
func hashOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (c *Catalog) normalize() error {
	if strings.TrimSpace(c.Site.Title) == "" {
		c.Site.Title = "内部网址导航"
	}
	if strings.TrimSpace(c.Site.Logo) == "" {
		c.Site.Logo = "GN"
	}
	if len(c.Categories) == 0 {
		return fmt.Errorf("至少需要一个分类")
	}

	c.byLinkID = make(map[string]*Link)
	c.byCatID = make(map[string]*Category)
	c.links = make([]*Link, 0, 64)

	for ci, cat := range c.Categories {
		if cat == nil {
			return fmt.Errorf("第 %d 个分类为空", ci+1)
		}
		cat.ID = strings.TrimSpace(cat.ID)
		if cat.ID == "" {
			return fmt.Errorf("第 %d 个分类缺少 id", ci+1)
		}
		if _, dup := c.byCatID[cat.ID]; dup {
			return fmt.Errorf("分类 id 重复: %s", cat.ID)
		}
		if strings.TrimSpace(cat.Name) == "" {
			cat.Name = cat.ID
		}
		if cat.Icon == "" {
			cat.Icon = "folder"
		}
		if !accentSet[cat.Accent] {
			cat.Accent = defaultAccent
		}
		cat.pyFull = pinyin.Full(cat.Name)
		cat.pyInit = pinyin.Initials(cat.Name)
		for li, link := range cat.Links {
			if link == nil {
				return fmt.Errorf("分类 %s 第 %d 条链接为空", cat.ID, li+1)
			}
			if err := c.prepareLink(cat, link, li); err != nil {
				return err
			}
		}
		if len(cat.Links) == 0 {
			return fmt.Errorf("分类 %s（%s）没有任何链接", cat.ID, cat.Name)
		}
		c.byCatID[cat.ID] = cat
	}
	return nil
}

func (c *Catalog) prepareLink(cat *Category, link *Link, idx int) error {
	where := fmt.Sprintf("分类 %s 第 %d 条链接", cat.ID, idx+1)
	link.Name = strings.TrimSpace(link.Name)
	if link.Name == "" {
		return fmt.Errorf("%s 缺少 name", where)
	}
	link.URL = strings.TrimSpace(link.URL)
	u, err := url.Parse(link.URL)
	if err != nil {
		return fmt.Errorf("%s（%s）URL 非法: %w", where, link.Name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s（%s）URL 必须以 http:// 或 https:// 开头，当前为 %q", where, link.Name, link.URL)
	}
	if u.Host == "" {
		return fmt.Errorf("%s（%s）URL 缺少主机名", where, link.Name)
	}
	if link.ID == "" {
		link.ID = slugify(link.Name)
	}
	if link.ID == "" {
		link.ID = cat.ID + "-" + strconv.Itoa(idx+1)
	}
	if _, dup := c.byLinkID[link.ID]; dup {
		return fmt.Errorf("链接 id 重复: %s（%s）", link.ID, link.Name)
	}
	if link.Icon == "" {
		link.Icon = "link"
	}
	link.CategoryID = cat.ID
	link.CategoryName = cat.Name
	link.Accent = cat.Accent
	link.pyFull = pinyin.Full(link.Name)
	link.pyInit = pinyin.Initials(link.Name)
	link.pyCatFull = cat.pyFull
	link.pyCatInit = cat.pyInit
	link.tagPyFull = pinyin.Join(link.Tags, pinyin.Full)
	link.tagPyInit = pinyin.Join(link.Tags, pinyin.Initials)

	c.byLinkID[link.ID] = link
	c.links = append(c.links, link)
	return nil
}

func slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// UpdatedAt 返回本次配置的加载时间。
func (c *Catalog) UpdatedAt() time.Time { return c.updatedAt }

// Revision 返回这份快照的版本号（单调递增，仅由 Store 赋值）。
func (c *Catalog) Revision() uint64 { return c.revision }

// ContentHash 返回配置内容摘要（sha256 十六进制）。
func (c *Catalog) ContentHash() string { return c.contentHash }

// Source 返回这份配置的来源：file | gateway。
func (c *Catalog) Source() string {
	if c.source == "" {
		return SourceFile
	}
	return c.source
}

// SourcePath 返回配置来源路径。
func (c *Catalog) SourcePath() string { return c.sourcePath }

// Links 返回按分类顺序展开的全部链接。
func (c *Catalog) Links() []*Link { return c.links }

// LinkCount 返回链接总数。
func (c *Catalog) LinkCount() int { return len(c.links) }

// CategoryCount 返回分类总数。
func (c *Catalog) CategoryCount() int { return len(c.Categories) }

// Category 按 id 查找分类。
func (c *Catalog) Category(id string) (*Category, bool) {
	cat, ok := c.byCatID[id]
	return cat, ok
}

// Link 按 id 查找链接。
func (c *Catalog) Link(id string) (*Link, bool) {
	l, ok := c.byLinkID[id]
	return l, ok
}

// Pinned 返回所有「常用推荐」链接，按分类顺序排列。
func (c *Catalog) Pinned() []*Link {
	out := make([]*Link, 0, 8)
	for _, l := range c.links {
		if l.Pinned {
			out = append(out, l)
		}
	}
	return out
}

// Match 是一条检索命中结果。
type Match struct {
	Link  *Link `json:"link"`
	Score int   `json:"score"`
}

// Search 按关键词检索链接。多个空格分隔的词之间是「与」关系，
// 每个词在名称 / 标签 / 描述 / 分类 / URL 上命中都会累计分数。
// limit <= 0 表示不限制条数。
func (c *Catalog) Search(query string, limit int) []Match {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(terms) == 0 {
		return nil
	}

	out := make([]Match, 0, 16)
	for _, l := range c.links {
		if score := l.matchScore(terms); score > 0 {
			out = append(out, Match{Link: l, Score: score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Link.Name < out[j].Link.Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (l *Link) matchScore(terms []string) int {
	name := strings.ToLower(l.Name)
	desc := strings.ToLower(l.Desc)
	cat := strings.ToLower(l.CategoryName)
	tags := strings.ToLower(strings.Join(l.Tags, " "))
	urlText := strings.ToLower(l.URL)

	total := 0
	for _, t := range terms {
		score := 0
		switch {
		case name == t:
			score += 200
		case strings.HasPrefix(name, t):
			score += 120
		case strings.Contains(name, t):
			score += 80
		}
		if strings.Contains(tags, t) {
			score += 50
		}
		if strings.Contains(cat, t) {
			score += 30
		}
		if strings.Contains(desc, t) {
			score += 20
		}
		if strings.Contains(urlText, t) {
			score += 10
		}
		score += l.matchPinyin(t)
		if score == 0 {
			return 0 // 任一词未命中即整体不命中
		}
		total += score
	}
	if l.Pinned {
		total += 5
	}
	return total
}

// matchPinyin 处理纯字母关键词：支持全拼、首字母，以及标签拼音。
func (l *Link) matchPinyin(term string) int {
	if !isASCIILetters(term) {
		return 0
	}
	score := 0
	switch {
	case l.pyInit == term:
		score += 110
	case strings.HasPrefix(l.pyInit, term):
		score += 90
	case strings.Contains(l.pyInit, term):
		score += 55
	}
	if strings.Contains(l.pyFull, term) {
		score += 45
	}
	if strings.Contains(l.tagPyInit, term) {
		score += 30
	}
	if strings.Contains(l.tagPyFull, term) {
		score += 20
	}
	if strings.Contains(l.pyCatInit, term) {
		score += 25
	}
	if strings.Contains(l.pyCatFull, term) {
		score += 15
	}
	return score
}

func isASCIILetters(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// Store 持有当前配置，并支持按文件 mtime 热更新。
type Store struct {
	path string

	mu       sync.RWMutex
	cur      *Catalog
	mod      time.Time
	err      error
	revision uint64
	subs     map[int]func(*Catalog)
	nextSub  int
}

// NewStore 加载配置并返回可热更新的 Store。配置非法时直接返回错误（快速失败）。
func NewStore(path string) (*Store, error) {
	c, err := Load(path)
	if err != nil {
		return nil, err
	}
	c.revision = 1
	s := &Store{path: path, cur: c, revision: 1, subs: make(map[int]func(*Catalog))}
	if fi, statErr := os.Stat(path); statErr == nil {
		s.mod = fi.ModTime()
	}
	return s, nil
}

// Current 返回当前生效的配置（只读，调用方不得修改）。
func (s *Store) Current() *Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Revision 返回当前配置版本号。
func (s *Store) Revision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}

// Subscribe 注册配置变更回调（供 gRPC WatchCatalog 使用），返回取消函数。
func (s *Store) Subscribe(fn func(*Catalog)) func() {
	s.mu.Lock()
	if s.subs == nil {
		s.subs = make(map[int]func(*Catalog))
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = fn
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

// notifyLocked 必须在持有写锁时调用，回调在锁外异步执行避免阻塞读。
func (s *Store) notify(snapshot *Catalog) {
	s.mu.RLock()
	subs := make([]func(*Catalog), 0, len(s.subs))
	for _, fn := range s.subs {
		subs = append(subs, fn)
	}
	s.mu.RUnlock()

	for _, fn := range subs {
		go fn(snapshot)
	}
}

// Path 返回配置文件路径。
func (s *Store) Path() string { return s.path }

// LastError 返回最近一次热更新失败的原因（成功时为 nil）。
func (s *Store) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

// Reload 强制重新加载配置。失败时保留旧配置并记录错误。
func (s *Store) Reload() (*Catalog, error) {
	c, err := Load(s.path)

	s.mu.Lock()
	if err != nil {
		s.err = err
		s.mu.Unlock()
		return s.cur, err
	}
	s.err = nil
	s.revision++
	c.revision = s.revision
	s.cur = c
	if fi, statErr := os.Stat(s.path); statErr == nil {
		s.mod = fi.ModTime()
	}
	s.mu.Unlock()

	s.notify(c)
	return c, nil
}

// ReloadIfChanged 在文件 mtime 变化时重载，返回是否发生了重载。
func (s *Store) ReloadIfChanged() (bool, error) {
	fi, err := os.Stat(s.path)
	if err != nil {
		return false, fmt.Errorf("stat 配置 %s: %w", s.path, err)
	}
	s.mu.RLock()
	unchanged := fi.ModTime().Equal(s.mod)
	s.mu.RUnlock()
	if unchanged {
		return false, nil
	}
	_, err = s.Reload()
	return err == nil, err
}

// Watch 周期性检查配置文件变化，变化且校验通过时回调 onChange。
// 该校验失败不会中断进程，只会保留旧配置并打印告警。
func (s *Store) Watch(ctx context.Context, interval time.Duration, onChange func(*Catalog)) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := s.ReloadIfChanged()
			if err != nil {
				slog.Warn("链接配置热更新失败，继续使用旧配置", "path", s.path, "err", err)
				continue
			}
			if changed {
				slog.Info("链接配置已热更新",
					"path", s.path,
					"categories", s.Current().CategoryCount(),
					"links", s.Current().LinkCount())
				if onChange != nil {
					onChange(s.Current())
				}
			}
		}
	}
}
