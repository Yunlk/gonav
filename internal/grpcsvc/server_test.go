package grpcsvc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	navkitv1 "gonav/gen/navkit/v1"
	"gonav/internal/catalog"
	"gonav/internal/gatewayclient"
	"gonav/internal/stats"
)

const testConfig = `{
  "site": {"title": "单元测试导航", "subtitle": "副标题", "logo": "UT", "announcement": "公告", "footer": "页脚"},
  "categories": [
    {
      "id": "library",
      "name": "图书资源",
      "icon": "book",
      "accent": "violet",
      "description": "馆藏与电子资源",
      "links": [
        {"id": "library", "name": "图书馆官网", "url": "https://library.example.edu.cn/", "desc": "馆藏检索", "tags": ["常用"], "pinned": true},
        {"id": "cnki", "name": "中国知网", "url": "https://www.cnki.net/", "icon": "book", "desc": "中文文献"}
      ]
    },
    {
      "id": "teaching",
      "name": "教学系统",
      "icon": "graduation-cap",
      "accent": "sky",
      "description": "选课与作业",
      "links": [
        {"id": "teach", "name": "教务系统", "url": "https://teach.example.edu.cn/", "desc": "课表与成绩", "tags": ["常用"], "pinned": true},
        {"id": "xk", "name": "选课系统", "url": "https://xk.example.edu.cn/", "desc": "选课"}
      ]
    }
  ]
}`

type fixture struct {
	client navkitv1.NavkitServiceClient
	store  *catalog.Store
	stats  *stats.Store
	path   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixtureWith(t, nil)
}

// recordingGateway 记录反向回调事件，用于验证回调接线。
type recordingGateway struct {
	mu     sync.Mutex
	events []gatewayclient.Event
}

func (g *recordingGateway) Emit(_ context.Context, event gatewayclient.Event) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.events = append(g.events, event)
	return nil
}

func (g *recordingGateway) Describe() string { return "test-recorder" }

func (g *recordingGateway) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.events)
}

func newFixtureWith(t *testing.T, gateway gatewayclient.Client) *fixture {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "links.json")
	if err := os.WriteFile(path, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := catalog.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	st, err := stats.Open(filepath.Join(dir, "stats.json"))
	if err != nil {
		t.Fatalf("stats.Open: %v", err)
	}

	srv := New(Options{
		Store:     store,
		Stats:     st,
		Version:   "test",
		HTTPAddr:  ":0",
		GRPCAddr:  "bufnet",
		StartedAt: time.Now(),
		Gateway:   gateway,
	})

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	navkitv1.RegisterNavkitServiceServer(grpcServer, srv)
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return &fixture{client: navkitv1.NewNavkitServiceClient(conn), store: store, stats: st, path: path}
}

func TestGetCatalogReturnsSnapshotWithMeta(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	resp, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{IncludeLinks: true})
	if err != nil {
		t.Fatalf("GetCatalog: %v", err)
	}
	cat := resp.GetCatalog()
	if cat == nil {
		t.Fatal("catalog 为空")
	}
	if resp.GetNotModified() {
		t.Error("首次拉取不应是 not_modified")
	}

	meta := cat.GetMeta()
	if meta.GetRevision() == 0 {
		t.Error("revision 未赋值")
	}
	if len(meta.GetContentHash()) != 64 {
		t.Errorf("content_hash 不是 sha256: %q", meta.GetContentHash())
	}
	if meta.GetSource() != catalog.SourceFile {
		t.Errorf("source = %q, want %q", meta.GetSource(), catalog.SourceFile)
	}
	if meta.GetUpdatedAt() == nil {
		t.Error("updated_at 为空")
	}

	if got := len(cat.GetCategories()); got != 2 {
		t.Fatalf("分类数 = %d, want 2", got)
	}
	if got := len(cat.GetCategories()[0].GetLinks()); got != 2 {
		t.Errorf("第一个分类入口数 = %d, want 2", got)
	}

	site := cat.GetSite()
	if site.GetTitle() != "单元测试导航" || site.GetLogo() != "UT" || site.GetFooter() != "页脚" {
		t.Errorf("site 映射不完整: %+v", site)
	}

	link := cat.GetCategories()[1].GetLinks()[0]
	if link.GetId() != "teach" || link.GetCategoryName() != "教学系统" || !link.GetPinned() {
		t.Errorf("link 映射不完整: %+v", link)
	}
	if len(link.GetTags()) != 1 || link.GetTags()[0] != "常用" {
		t.Errorf("tags 映射异常: %v", link.GetTags())
	}
}

func TestGetCatalogNotModified(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	first, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{IncludeLinks: true})
	if err != nil {
		t.Fatal(err)
	}
	rev := first.GetCatalog().GetMeta().GetRevision()

	same, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{KnownRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	if !same.GetNotModified() || same.GetCatalog() != nil {
		t.Errorf("revision 相同时应只回 not_modified: %+v", same)
	}

	// revision 变化后必须重新下发
	f.store.Reload()
	after, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{KnownRevision: rev})
	if err != nil {
		t.Fatal(err)
	}
	if after.GetNotModified() || after.GetCatalog() == nil {
		t.Error("revision 变化后应重新下发")
	}
	if after.GetCatalog().GetMeta().GetRevision() <= rev {
		t.Errorf("revision 未递增: %d → %d", rev, after.GetCatalog().GetMeta().GetRevision())
	}
}

func TestGetCatalogCategoryFilter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	resp, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{CategoryId: "library", IncludeLinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.GetCatalog().GetCategories()); got != 1 {
		t.Fatalf("分类数 = %d, want 1", got)
	}
	if resp.GetCatalog().GetCategories()[0].GetId() != "library" {
		t.Error("过滤到了错误的分类")
	}

	_, err = f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{CategoryId: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("未知分类应返回 NotFound，实际 %v", status.Code(err))
	}
}

func TestGetCatalogWithoutLinks(t *testing.T) {
	f := newFixture(t)
	resp, err := f.client.GetCatalog(context.Background(), &navkitv1.GetCatalogRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, cat := range resp.GetCatalog().GetCategories() {
		if len(cat.GetLinks()) != 0 {
			t.Errorf("include_links=false 时不应带入口: %s", cat.GetId())
		}
	}
}

func TestListCategoriesReturnsSkeleton(t *testing.T) {
	f := newFixture(t)
	resp, err := f.client.ListCategories(context.Background(), &navkitv1.ListCategoriesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.GetCategories()); got != 2 {
		t.Fatalf("分类数 = %d", got)
	}
	if len(resp.GetCategories()[0].GetLinks()) != 0 {
		t.Error("ListCategories 不应带入口明细")
	}
	if resp.GetMeta().GetRevision() == 0 {
		t.Error("meta 缺失")
	}
}

func TestSearchReusesWeightedPinyinSearch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// 中文
	resp, err := f.client.Search(ctx, &navkitv1.SearchRequest{Query: "教务"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetMatches()) == 0 || resp.GetMatches()[0].GetLink().GetId() != "teach" {
		t.Fatalf("中文检索失败: %+v", resp.GetMatches())
	}
	if resp.GetMatches()[0].GetScore() <= 0 {
		t.Error("score 未返回")
	}
	if resp.GetRevision() == 0 {
		t.Error("revision 未返回")
	}

	// 拼音首字母
	resp, err = f.client.Search(ctx, &navkitv1.SearchRequest{Query: "jwxt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetMatches()) != 1 || resp.GetMatches()[0].GetLink().GetId() != "teach" {
		t.Errorf("首字母检索失败: %+v", resp.GetMatches())
	}

	// 分类限定
	resp, err = f.client.Search(ctx, &navkitv1.SearchRequest{Query: "系统", CategoryId: "library"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range resp.GetMatches() {
		if m.GetLink().GetCategoryId() != "library" {
			t.Errorf("分类限定失效: %s", m.GetLink().GetId())
		}
	}
}

func TestSearchLimitValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	resp, err := f.client.Search(ctx, &navkitv1.SearchRequest{Query: "系统", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetMatches()) != 1 {
		t.Errorf("limit=1 时返回 %d 条", len(resp.GetMatches()))
	}

	if _, err := f.client.Search(ctx, &navkitv1.SearchRequest{Query: "系统", Limit: 201}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("limit>200 应返回 InvalidArgument，实际 %v", status.Code(err))
	}
}

func TestHealthAndServerInfo(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	health, err := f.client.Health(ctx, &navkitv1.HealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if health.GetStatus() != "ok" {
		t.Errorf("status = %q", health.GetStatus())
	}
	if health.GetLinkCount() != 4 || health.GetCategoryCount() != 2 {
		t.Errorf("计数异常: links=%d categories=%d", health.GetLinkCount(), health.GetCategoryCount())
	}

	info, err := f.client.GetServerInfo(ctx, &navkitv1.GetServerInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetVersion() != "test" {
		t.Errorf("version = %q", info.GetVersion())
	}

	// 能力清单只能包含已实现的方法：网关据此避免踩 Unimplemented
	live := map[string]bool{}
	for _, c := range info.GetCapabilities() {
		live[c] = true
	}
	for _, want := range []string{
		"navkit.v1.NavkitService/Health",
		"navkit.v1.NavkitService/GetServerInfo",
		"navkit.v1.NavkitService/GetCatalog",
		"navkit.v1.NavkitService/ListCategories",
		"navkit.v1.NavkitService/Search",
		"navkit.v1.NavkitService/GetStats",
		"navkit.v1.NavkitService/PutCatalog",
		"navkit.v1.NavkitService/PatchLink",
		"navkit.v1.NavkitService/WatchCatalog",
	} {
		if !live[want] {
			t.Errorf("能力清单缺少 %s", want)
		}
	}
	if len(live) != len(capabilities) {
		t.Errorf("能力清单数量 = %d, want %d", len(live), len(capabilities))
	}
}

func TestHealthReportsDegradedAfterBadConfig(t *testing.T) {
	f := newFixture(t)

	if err := os.WriteFile(f.path, []byte(`{"categories":[{"id":"a","links":[{"name":"x","url":"nope"}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Reload(); err == nil {
		t.Fatal("非法配置应报错")
	}

	health, err := f.client.Health(context.Background(), &navkitv1.HealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if health.GetStatus() != "degraded" {
		t.Errorf("status = %q, want degraded", health.GetStatus())
	}
	if health.GetLastError() == "" {
		t.Error("degraded 时应带 last_error")
	}
	// 降级时仍在服务上一版可用配置
	if health.GetLinkCount() != 4 {
		t.Errorf("降级后入口数 = %d, want 4（保留旧配置）", health.GetLinkCount())
	}
}

func TestPutCatalogWritesAndBumpsRevision(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	payload := &navkitv1.PutCatalogRequest{
		Site: &navkitv1.Site{Title: "网关下发导航", Logo: "GW", Subtitle: "由主网关统一下发"},
		Categories: []*navkitv1.Category{
			{
				Id: "gw", Name: "网关分类", Icon: "cloud", Accent: "sky", Description: "来自主网关",
				Links: []*navkitv1.Link{
					{Id: "a", Name: "入口 A", Url: "https://a.example.com/", Tags: []string{"常用"}, Pinned: true},
					{Id: "b", Name: "入口 B", Url: "https://b.example.com/"},
				},
			},
		},
	}

	resp, err := f.client.PutCatalog(ctx, payload)
	if err != nil {
		t.Fatalf("PutCatalog: %v", err)
	}
	if resp.GetRevision() != 2 {
		t.Errorf("revision = %d, want 2（初始为 1）", resp.GetRevision())
	}
	if resp.GetLinkCount() != 2 || resp.GetCategoryCount() != 1 {
		t.Errorf("计数异常: links=%d cats=%d", resp.GetLinkCount(), resp.GetCategoryCount())
	}
	if len(resp.GetContentHash()) != 64 {
		t.Errorf("content_hash 异常: %q", resp.GetContentHash())
	}

	// 内存里已生效
	got, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{IncludeLinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetCatalog().GetSite().GetTitle() != "网关下发导航" {
		t.Error("新配置未生效")
	}
	if got.GetCatalog().GetMeta().GetSource() != catalog.SourceGateway {
		t.Errorf("source = %q, want %q", got.GetCatalog().GetMeta().GetSource(), catalog.SourceGateway)
	}

	// 落盘了，而且是规范化后的形状（不含派生字段）
	raw, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "网关下发导航") || !strings.Contains(text, "入口 A") {
		t.Errorf("配置文件未更新: %s", text[:min(120, len(text))])
	}
	if strings.Contains(text, "categoryId") || strings.Contains(text, "contentHash") {
		t.Error("配置文件里混进了派生字段")
	}

	// Health 同步反映新配置
	health, err := f.client.Health(ctx, &navkitv1.HealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if health.GetRevision() != 2 || health.GetLinkCount() != 2 || health.GetCategoryCount() != 1 {
		t.Errorf("Health 未同步: %+v", health)
	}
}

func TestPutCatalogRejectsInvalidAndKeepsOldConfig(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	before, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{IncludeLinks: true})
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := before.GetCatalog().GetMeta().GetRevision()

	_, err = f.client.PutCatalog(ctx, &navkitv1.PutCatalogRequest{
		Site: &navkitv1.Site{Title: "坏的"},
		Categories: []*navkitv1.Category{
			{Id: "x", Name: "坏分类", Links: []*navkitv1.Link{{Id: "bad", Name: "坏入口", Url: "ftp://nope"}}},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("非法 URL 应返回 InvalidArgument，实际 %v (%v)", status.Code(err), err)
	}
	if !strings.Contains(err.Error(), "http") {
		t.Errorf("错误信息应指出 URL 问题: %v", err)
	}

	// 旧配置与版本号都不能被破坏
	after, err := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{IncludeLinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if after.GetCatalog().GetMeta().GetRevision() != beforeRevision {
		t.Error("校验失败不应递增 revision")
	}
	if after.GetCatalog().GetSite().GetTitle() != "单元测试导航" {
		t.Error("校验失败不应替换掉可用配置")
	}

	// 空分类列表也要拦住
	if _, err := f.client.PutCatalog(ctx, &navkitv1.PutCatalogRequest{Site: &navkitv1.Site{Title: "空"}}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("空 categories 应返回 InvalidArgument，实际 %v", status.Code(err))
	}
}

func TestPutCatalogDryRunAndRevisionConflict(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	payload := &navkitv1.PutCatalogRequest{
		Site: &navkitv1.Site{Title: "dry run"},
		Categories: []*navkitv1.Category{
			{Id: "d", Name: "D", Links: []*navkitv1.Link{{Id: "d1", Name: "D1", Url: "https://d.example.com/"}}},
		},
		DryRun: true,
	}
	if _, err := f.client.PutCatalog(ctx, payload); err != nil {
		t.Fatalf("dry run 应成功: %v", err)
	}
	after, _ := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{})
	if after.GetCatalog().GetMeta().GetRevision() != 1 || after.GetCatalog().GetSite().GetTitle() != "单元测试导航" {
		t.Error("dry run 不应改动任何状态")
	}

	// 乐观并发：声明一个错误的 revision
	payload.DryRun = false
	payload.ExpectedRevision = 999
	if _, err := f.client.PutCatalog(ctx, payload); status.Code(err) != codes.Aborted {
		t.Errorf("revision 冲突应返回 Aborted，实际 %v", status.Code(err))
	}

	// 声明正确的 revision
	payload.ExpectedRevision = 1
	if _, err := f.client.PutCatalog(ctx, payload); err != nil {
		t.Errorf("revision 匹配时应成功: %v", err)
	}
}

func TestPatchLinkUpsertAndDelete(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// 新增
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action:     navkitv1.PatchLinkRequest_ACTION_UPSERT,
		CategoryId: "library",
		Link:       &navkitv1.Link{Id: "wanfang", Name: "万方数据", Url: "https://www.wanfangdata.com.cn/", Tags: []string{"文献"}},
	}); err != nil {
		t.Fatalf("PatchLink upsert: %v", err)
	}
	got, _ := f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{CategoryId: "library", IncludeLinks: true})
	if len(got.GetCatalog().GetCategories()[0].GetLinks()) != 3 {
		t.Errorf("新增后入口数 = %d, want 3", len(got.GetCatalog().GetCategories()[0].GetLinks()))
	}

	// 更新同 id
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action:     navkitv1.PatchLinkRequest_ACTION_UPSERT,
		CategoryId: "library",
		Link:       &navkitv1.Link{Id: "wanfang", Name: "万方（改名）", Url: "https://www.wanfangdata.com.cn/"},
	}); err != nil {
		t.Fatalf("PatchLink 更新: %v", err)
	}
	got, _ = f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{CategoryId: "library", IncludeLinks: true})
	links := got.GetCatalog().GetCategories()[0].GetLinks()
	if len(links) != 3 {
		t.Fatalf("更新不应新增条目，实际 %d 条", len(links))
	}
	found := false
	for _, link := range links {
		if link.GetId() == "wanfang" && link.GetName() == "万方（改名）" {
			found = true
		}
	}
	if !found {
		t.Error("更新未生效")
	}

	// 删除
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action:     navkitv1.PatchLinkRequest_ACTION_DELETE,
		CategoryId: "library",
		Link:       &navkitv1.Link{Id: "wanfang"},
	}); err != nil {
		t.Fatalf("PatchLink delete: %v", err)
	}
	got, _ = f.client.GetCatalog(ctx, &navkitv1.GetCatalogRequest{CategoryId: "library", IncludeLinks: true})
	if len(got.GetCatalog().GetCategories()[0].GetLinks()) != 2 {
		t.Error("删除未生效")
	}

	// 错误分支
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action: navkitv1.PatchLinkRequest_ACTION_DELETE, CategoryId: "library", Link: &navkitv1.Link{Id: "nope"},
	}); status.Code(err) != codes.NotFound {
		t.Errorf("删除不存在条目应返回 NotFound，实际 %v", status.Code(err))
	}
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action: navkitv1.PatchLinkRequest_ACTION_UPSERT, CategoryId: "nope", Link: &navkitv1.Link{Id: "x", Name: "X", Url: "https://x.example.com/"},
	}); status.Code(err) != codes.NotFound {
		t.Errorf("未知分类应返回 NotFound，实际 %v", status.Code(err))
	}
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{CategoryId: "library"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("缺 action 应返回 InvalidArgument，实际 %v", status.Code(err))
	}
}

func TestWatchCatalogStreamsChanges(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// 先改一次，制造「客户端落后」的场景
	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action:     navkitv1.PatchLinkRequest_ACTION_UPSERT,
		CategoryId: "library",
		Link:       &navkitv1.Link{Id: "patch1", Name: "补丁一", Url: "https://p1.example.com/"},
	}); err != nil {
		t.Fatal(err)
	}

	// from_revision 落后 → 立刻补推一份当前快照
	stream, err := f.client.WatchCatalog(ctx, &navkitv1.WatchCatalogRequest{FromRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("补推失败: %v", err)
	}
	if first.GetHeartbeat() {
		t.Error("补推不应是心跳")
	}
	if first.GetRevision() != 2 || first.GetCatalog() == nil {
		t.Errorf("补推内容异常: rev=%d catalog=%v", first.GetRevision(), first.GetCatalog() != nil)
	}
	if len(first.GetContentHash()) != 64 {
		t.Error("补推缺少 content_hash")
	}

	// 之后继续变更 → 同一个流上收到推送
	events := make(chan *navkitv1.WatchCatalogResponse, 4)
	go func() {
		for {
			event, err := stream.Recv()
			if err != nil {
				return
			}
			events <- event
		}
	}()

	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action:     navkitv1.PatchLinkRequest_ACTION_UPSERT,
		CategoryId: "teaching",
		Link:       &navkitv1.Link{Id: "patch2", Name: "补丁二", Url: "https://p2.example.com/"},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-events:
		if event.GetHeartbeat() {
			t.Error("变更推送不应标记为心跳")
		}
		if event.GetRevision() != 3 {
			t.Errorf("推送 revision = %d, want 3", event.GetRevision())
		}
		found := false
		for _, cat := range event.GetCatalog().GetCategories() {
			for _, link := range cat.GetLinks() {
				if link.GetId() == "patch2" {
					found = true
				}
			}
		}
		if !found {
			t.Error("推送的快照里没有新入口")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("4 秒内未收到变更推送")
	}
}

func TestGetStatsWorks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	f.stats.Hit("teach")
	resp, err := f.client.GetStats(ctx, &navkitv1.GetStatsRequest{Top: 5})
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if resp.GetTotal() != 1 || len(resp.GetTop()) != 1 {
		t.Errorf("统计异常: %+v", resp)
	}
	if resp.GetTop()[0].GetName() != "教务系统" {
		t.Errorf("统计未回填名称: %+v", resp.GetTop()[0])
	}
}

func TestGatewayCallbackFiresOnWrite(t *testing.T) {
	gateway := &recordingGateway{}
	f := newFixtureWith(t, gateway)
	ctx := context.Background()

	if _, err := f.client.PatchLink(ctx, &navkitv1.PatchLinkRequest{
		Action:     navkitv1.PatchLinkRequest_ACTION_UPSERT,
		CategoryId: "library",
		Link:       &navkitv1.Link{Id: "cb", Name: "回调测试", Url: "https://cb.example.com/"},
	}); err != nil {
		t.Fatal(err)
	}

	// 回调是异步的，给它一点时间
	deadline := time.Now().Add(2 * time.Second)
	for gateway.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if gateway.count() != 1 {
		t.Fatalf("回调次数 = %d, want 1", gateway.count())
	}

	event := gateway.events[0]
	if event.Kind != gatewayclient.EventCatalogChanged {
		t.Errorf("事件类型 = %q", event.Kind)
	}
	if event.Revision != 2 {
		t.Errorf("事件 revision = %d, want 2", event.Revision)
	}
	if event.Source != catalog.SourceGateway || len(event.ContentHash) != 64 {
		t.Errorf("事件载荷异常: %+v", event)
	}

	// dry run 不应触发回调
	before := gateway.count()
	if _, err := f.client.PutCatalog(ctx, &navkitv1.PutCatalogRequest{
		Site:   &navkitv1.Site{Title: "dry"},
		DryRun: true,
		Categories: []*navkitv1.Category{
			{Id: "d", Name: "D", Links: []*navkitv1.Link{{Id: "d1", Name: "D1", Url: "https://d.example.com/"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if gateway.count() != before {
		t.Error("dry run 不应触发网关回调")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
