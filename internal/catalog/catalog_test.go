package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleJSON = `{
  "site": {"title": "测试导航", "subtitle": "副标题", "logo": "T"},
  "categories": [
    {
      "id": "library",
      "name": "图书资源",
      "icon": "book",
      "accent": "violet",
      "links": [
        {"id": "opac", "name": "馆藏检索", "url": "https://lib.example.edu.cn/opac", "desc": "纸本图书借阅", "tags": ["常用"], "pinned": true},
        {"name": "电子期刊", "url": "https://lib.example.edu.cn/ejournal", "icon": "file-text"}
      ]
    },
    {
      "id": "teaching",
      "name": "教学系统",
      "links": [
        {"id": "lms", "name": "网络教学平台", "url": "https://lms.example.edu.cn", "desc": "课程资料与作业", "tags": ["LMS"]}
      ]
    }
  ]
}`

func TestParseBuildsIndexes(t *testing.T) {
	c, err := Parse([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	if got := c.LinkCount(); got != 3 {
		t.Errorf("LinkCount = %d, want 3", got)
	}
	if got := c.CategoryCount(); got != 2 {
		t.Errorf("CategoryCount = %d, want 2", got)
	}
	if got := c.Site.Title; got != "测试导航" {
		t.Errorf("Site.Title = %q", got)
	}

	link, ok := c.Link("opac")
	if !ok {
		t.Fatal("按 id 查找 opac 失败")
	}
	if link.CategoryName != "图书资源" || link.Accent != "violet" {
		t.Errorf("分类信息未回填: %+v", link)
	}
	if got := link.Host(); got != "lib.example.edu.cn" {
		t.Errorf("Host = %q", got)
	}

	// 未提供 id 的链接应自动生成 id。
	if _, ok := c.Link("lib-example-edu-cn-ejournal"); !ok {
		// slug 基于名称，中文名会退化为 分类 id + 序号，两种结果都允许。
		if _, ok := c.Link("library-2"); !ok {
			ids := make([]string, 0, len(c.Links()))
			for _, l := range c.Links() {
				ids = append(ids, l.ID)
			}
			t.Fatalf("未生成链接 id, 现有 id: %v", ids)
		}
	}

	// 分类未指定 accent 时应回落到默认值。
	teaching, _ := c.Category("teaching")
	if teaching.Accent != defaultAccent {
		t.Errorf("accent 默认值 = %q, want %q", teaching.Accent, defaultAccent)
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "非法 URL 协议",
			raw:  `{"categories":[{"id":"a","links":[{"name":"x","url":"ftp://example.com"}]}]}`,
			want: "http://",
		},
		{
			name: "缺少链接名称",
			raw:  `{"categories":[{"id":"a","links":[{"url":"https://example.com"}]}]}`,
			want: "缺少 name",
		},
		{
			name: "重复链接 id",
			raw: `{"categories":[{"id":"a","links":[
				{"id":"dup","name":"x","url":"https://a.example.com"},
				{"id":"dup","name":"y","url":"https://b.example.com"}]}]}`,
			want: "重复",
		},
		{
			name: "空分类列表",
			raw:  `{"categories":[]}`,
			want: "至少需要一个分类",
		},
		{
			name: "分类无链接",
			raw:  `{"categories":[{"id":"a","links":[]}]}`,
			want: "没有任何链接",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			if err == nil {
				t.Fatalf("期望报错，实际通过")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("错误信息 %q 未包含 %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	c, err := Parse([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	tests := []struct {
		query string
		want  string // 期望排名第一的链接 id
	}{
		{"opac", "opac"},
		{"馆藏", "opac"},
		{"借阅", "opac"},
		{"LMS", "lms"},
		{"教学", "lms"},
		{"lib.example", "opac"},
		{"lib.example.edu.cn lib", "opac"},
		// 拼音与首字母
		{"gcjs", "opac"},
		{"guancang", "opac"},
		{"guancangjiansuo", "opac"},
		{"wljxpt", "lms"},
		{"wangluojiaoxue", "lms"},
		{"dzqk", "library-2"},
	}
	for _, tc := range tests {
		got := c.Search(tc.query, 5)
		if len(got) == 0 {
			t.Errorf("Search(%q) 无结果", tc.query)
			continue
		}
		if got[0].Link.ID != tc.want {
			t.Errorf("Search(%q) 首位 = %s, want %s", tc.query, got[0].Link.ID, tc.want)
		}
	}

	// 单字拼音首字母命中分类：教学系统的链接都能被 "jx" 找到。
	if got := c.Search("jx", 5); len(got) != 1 || got[0].Link.ID != "lms" {
		t.Errorf("Search(\"jx\") = %+v", got)
	}

	if got := c.Search("不存在的关键词", 5); len(got) != 0 {
		t.Errorf("无关关键词应无结果，实际 %d 条", len(got))
	}
	if got := c.Search("   ", 5); got != nil {
		t.Errorf("空关键词应返回 nil，实际 %v", got)
	}
	if got := c.Search("图书 借阅", 5); len(got) == 0 {
		t.Error("多词“与”检索应有结果")
	}
	if got := c.Search("图书 不存在的词", 5); len(got) != 0 {
		t.Error("多词检索中任一未命中应整体不命中")
	}

	pinned := c.Pinned()
	if len(pinned) != 1 || pinned[0].ID != "opac" {
		t.Errorf("Pinned = %+v", pinned)
	}

	limited := c.Search("lib", 2)
	if len(limited) > 2 {
		t.Errorf("limit 未生效: %d", len(limited))
	}
}

func TestStoreReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "links.json")
	if err := os.WriteFile(path, []byte(sampleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore 失败: %v", err)
	}
	if got := store.Current().LinkCount(); got != 3 {
		t.Fatalf("初始链接数 = %d", got)
	}

	// mtime 未变化时不重载。
	changed, err := store.ReloadIfChanged()
	if err != nil || changed {
		t.Fatalf("ReloadIfChanged = (%v, %v), want (false, nil)", changed, err)
	}

	// 写入非法配置：保留旧配置并记录错误。
	if err := os.WriteFile(path, []byte(`{"categories":[{"id":"a","links":[{"name":"x","url":"nope"}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMtime(t, path)
	if _, err := store.Reload(); err == nil {
		t.Fatal("非法配置应当报错")
	}
	if store.LastError() == nil {
		t.Fatal("LastError 应记录最近一次失败")
	}
	if got := store.Current().LinkCount(); got != 3 {
		t.Fatalf("失败后应保留旧配置，实际 %d", got)
	}

	// 修复配置后应恢复。
	if err := os.WriteFile(path, []byte(sampleJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMtime(t, path)
	if _, err := store.Reload(); err != nil {
		t.Fatalf("修复后重载失败: %v", err)
	}
	if store.LastError() != nil {
		t.Fatalf("成功后 LastError 应为 nil, 实际 %v", store.LastError())
	}

	// Watch 在文件变化后触发回调。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go store.Watch(ctx, 20*time.Millisecond, func(c *Catalog) {
		select {
		case done <- c.LinkCount():
		default:
		}
	})

	patched := strings.Replace(sampleJSON, `"id": "lms"`, `"id": "lms2"`, 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMtime(t, path)

	select {
	case n := <-done:
		if n != 3 {
			t.Errorf("Watch 回调链接数 = %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch 未在 2s 内触发回调")
	}
	cancel()
}

// bumpMtime 把文件时间戳向前推进，避免同秒写入导致 mtime 判定失效。
func bumpMtime(t *testing.T, path string) {
	t.Helper()
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}
