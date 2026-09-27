package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"gonav/internal/catalog"
	"gonav/internal/stats"
)

const testConfig = `{
  "site": {"title": "单元测试导航", "subtitle": "测试副标题", "logo": "UT", "announcement": "测试公告", "footer": "测试页脚"},
  "categories": [
    {
      "id": "library",
      "name": "图书资源",
      "icon": "book",
      "accent": "violet",
      "description": "馆藏与电子资源",
      "links": [
        {"id": "opac", "name": "馆藏检索", "url": "https://lib.example.edu.cn/opac", "desc": "纸本图书借阅", "tags": ["常用"], "pinned": true}
      ]
    },
    {
      "id": "teaching",
      "name": "教学系统",
      "icon": "graduation-cap",
      "accent": "sky",
      "description": "选课与作业",
      "links": [
        {"id": "lms", "name": "网络教学平台", "url": "https://lms.example.edu.cn", "desc": "课程资料与作业", "tags": ["LMS"]}
      ]
    }
  ]
}`

func newTestServer(t *testing.T) (*httptest.Server, *stats.Store) {
	t.Helper()

	dir := t.TempDir()
	cfg := filepath.Join(dir, "links.json")
	if err := os.WriteFile(cfg, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := catalog.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	st, err := stats.Open(filepath.Join(dir, "stats.json"))
	if err != nil {
		t.Fatalf("stats.Open: %v", err)
	}

	srv, err := New(Options{
		Store:      store,
		Stats:      st,
		Version:    "test",
		ScriptPath: filepath.Join(dir, "userscripts.json"),
	})
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

// get 发起请求并禁止自动跟随重定向，便于断言 302 目标。
func get(t *testing.T, ts *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	resp, err := client.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体 %s: %v", path, err)
	}
	return resp, string(body)
}

func TestIndexPage(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, body := get(t, ts, "/")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("缺少 CSP 头: %q", csp)
	}

	for _, want := range []string{
		"单元测试导航",
		"图书资源",
		"教学系统",
		"馆藏检索",
		"网络教学平台",
		`action="/search"`,
		`/go/opac`,
		`data-search-form`,
		"测试公告",
		"测试页脚",
		"站内导航",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("首页缺少内容: %q", want)
		}
	}
}

func TestSearchPageInternal(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, body := get(t, ts, "/search?q=%E9%A6%86%E8%97%8F")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d", resp.StatusCode)
	}
	if !strings.Contains(body, "馆藏检索") {
		t.Error("站内检索未命中「馆藏检索」")
	}
	if !strings.Contains(body, `<mark class="hl">馆藏</mark>`) {
		t.Error("结果页未高亮命中的关键词")
	}
	if !strings.Contains(body, "命中") {
		t.Error("结果页缺少命中数提示")
	}

	// 空查询应给出提示而不是报错。
	_, empty := get(t, ts, "/search?q=")
	if !strings.Contains(empty, "请输入关键词") {
		t.Error("空查询缺少提示语")
	}
}

func TestPinyinSearch(t *testing.T) {
	ts, _ := newTestServer(t)

	// 首字母 / 全拼都能命中；纯拼音命中时没有可高亮的字面片段。
	for _, query := range []string{"gcjs", "guancang", "wljxpt", "wangluojiaoxue"} {
		resp, body := get(t, ts, "/search?q="+query)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s 状态码 = %d", query, resp.StatusCode)
		}
		if !strings.Contains(body, "命中 <strong>1</strong> 条") {
			t.Errorf("%s 未命中唯一结果", query)
		}
		if strings.Contains(body, `<mark class="hl">`) {
			t.Errorf("%s 是拼音命中，不应有字面高亮", query)
		}
	}

	// 首页的实时筛选数据里带上拼音串。
	_, index := get(t, ts, "/")
	if !strings.Contains(index, "wangluojiaoxuepingtai") || !strings.Contains(index, "gcjs") {
		t.Error("首页 data-search 未包含拼音")
	}
	if !strings.Contains(index, "jwxt") {
		t.Error("首页未提示拼音检索用法")
	}
}

func TestFavoritesHooks(t *testing.T) {
	ts, _ := newTestServer(t)

	_, index := get(t, ts, "/")
	for _, want := range []string{
		`data-fav-strip`,
		`data-fav-body`,
		`data-fav="opac"`,
		`data-fav="lms"`,
		`aria-pressed="false"`,
		`我的收藏`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("首页缺少收藏夹元素: %q", want)
		}
	}

	_, results := get(t, ts, "/search?q=lms")
	if !strings.Contains(results, `data-fav="lms"`) {
		t.Error("结果页缺少收藏按钮")
	}
}

func TestSearchRedirectsToExternalEngine(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, _ := get(t, ts, "/search?engine=baidu&q=golang")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码 = %d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://www.baidu.com/s?wd=golang") {
		t.Errorf("跳转地址 = %q", loc)
	}

	// 未知引擎回落到站内检索。
	resp2, body2 := get(t, ts, "/search?engine=not-exist&q=lms")
	if resp2.StatusCode != http.StatusOK || !strings.Contains(body2, "网络教学平台") {
		t.Errorf("未知引擎应回落站内检索, 状态码 %d", resp2.StatusCode)
	}
}

func TestGoTracksAndRedirects(t *testing.T) {
	ts, st := newTestServer(t)

	resp, _ := get(t, ts, "/go/opac")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码 = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://lib.example.edu.cn/opac" {
		t.Errorf("跳转地址 = %q", loc)
	}
	if got := st.Hits("opac"); got != 1 {
		t.Errorf("点击未被统计, Hits(opac) = %d", got)
	}

	// 存在的 id 跳转首页。
	resp2, _ := get(t, ts, "/go/not-exist")
	if resp2.StatusCode != http.StatusFound || resp2.Header.Get("Location") != "/" {
		t.Errorf("未知 id 应跳回首页, 状态码 %d, Location %q", resp2.StatusCode, resp2.Header.Get("Location"))
	}
}

func TestJSONAPIs(t *testing.T) {
	ts, st := newTestServer(t)
	st.Hit("lms")
	st.Hit("lms")

	resp, body := get(t, ts, "/api/search?q=LMS")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d", resp.StatusCode)
	}
	var searchResp struct {
		Query   string `json:"query"`
		Count   int    `json:"count"`
		Results []struct {
			Link struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"link"`
			Score int `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &searchResp); err != nil {
		t.Fatalf("解析 /api/search 失败: %v (%s)", err, body)
	}
	if searchResp.Count != 1 || searchResp.Results[0].Link.ID != "lms" {
		t.Errorf("/api/search 结果异常: %+v", searchResp)
	}

	// 无结果时 results 应为空数组而非 null。
	_, emptyBody := get(t, ts, "/api/search?q=zzz-nothing")
	if !strings.Contains(emptyBody, `"results": []`) {
		t.Errorf("空结果的 JSON 形态不理想: %s", emptyBody)
	}

	_, linksBody := get(t, ts, "/api/links")
	if !strings.Contains(linksBody, `"图书资源"`) || !strings.Contains(linksBody, `"opac"`) {
		t.Error("/api/links 内容不完整")
	}

	_, catBody := get(t, ts, "/api/links?category=teaching")
	if !strings.Contains(catBody, `"网络教学平台"`) {
		t.Error("/api/links?category= 未按分类过滤")
	}

	resp404, _ := get(t, ts, "/api/links?category=missing")
	if resp404.StatusCode != http.StatusNotFound {
		t.Errorf("未知分类状态码 = %d", resp404.StatusCode)
	}

	_, statsBody := get(t, ts, "/api/stats")
	var statsResp struct {
		Total int64 `json:"total"`
		Top   []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Hits int64  `json:"hits"`
		} `json:"top"`
	}
	if err := json.Unmarshal([]byte(statsBody), &statsResp); err != nil {
		t.Fatalf("解析 /api/stats 失败: %v", err)
	}
	if statsResp.Total != 2 || len(statsResp.Top) != 1 || statsResp.Top[0].Name != "网络教学平台" {
		t.Errorf("/api/stats 结果异常: %+v", statsResp)
	}

	_, health := get(t, ts, "/healthz")
	if !strings.Contains(health, `"status": "ok"`) {
		t.Errorf("/healthz = %s", health)
	}
}

func TestStaticAssetsAnd404(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, css := get(t, ts, "/static/css/app.css")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CSS 状态码 = %d", resp.StatusCode)
	}
	if !strings.Contains(css, "--surface") || !strings.Contains(css, "@font-face") {
		t.Error("CSS 内容异常")
	}

	respJS, js := get(t, ts, "/static/js/app.js")
	if respJS.StatusCode != http.StatusOK ||
		!strings.Contains(js, "GONAV_THEME") || !strings.Contains(js, "data-search-input") {
		t.Errorf("JS 资源异常: %d", respJS.StatusCode)
	}

	respIcon, _ := get(t, ts, "/static/favicon.svg")
	if respIcon.StatusCode != http.StatusOK || !strings.Contains(respIcon.Header.Get("Content-Type"), "svg") {
		t.Errorf("favicon 异常: %d %s", respIcon.StatusCode, respIcon.Header.Get("Content-Type"))
	}

	resp404, body404 := get(t, ts, "/this-page-does-not-exist")
	if resp404.StatusCode != http.StatusNotFound {
		t.Errorf("404 状态码 = %d", resp404.StatusCode)
	}
	if !strings.Contains(body404, "页面不存在") {
		t.Error("404 页面未渲染友好提示")
	}
}

func TestTopPinnedRendering(t *testing.T) {
	ts, st := newTestServer(t)
	st.Hit("lms")
	st.Hit("lms")
	st.Hit("lms")

	_, body := get(t, ts, "/")
	if !strings.Contains(body, "常用推荐") {
		t.Error("缺少常用推荐区块")
	}
	if !strings.Contains(body, "热门入口") {
		t.Error("缺少热门入口区块")
	}
	if !strings.Contains(body, "3 次访问") {
		t.Error("热门入口未反映点击量")
	}
}

// TestNavkitServed 覆盖 navkit 单页应用的托管方式与它专用的 CSP。
func TestNavkitServed(t *testing.T) {
	ts, _ := newTestServer(t)

	resp, body := get(t, ts, "/navkit/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/navkit/ 状态码 = %d", resp.StatusCode)
	}
	if !strings.Contains(body, `id="root"`) {
		t.Error("/navkit/ 未返回前端挂载点")
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "img-src 'self' data: blob: https:") {
		t.Errorf("navkit 的 CSP 未放开图片域名: %q", csp)
	}

	// 带内容哈希的产物可以长期缓存
	asset := regexp.MustCompile(`/navkit/(assets/[^"]+\.js)`).FindStringSubmatch(body)
	if len(asset) != 2 {
		t.Fatalf("未能从 index.html 里解析出资源路径: %s", body)
	}
	assetResp, _ := get(t, ts, "/navkit/"+asset[1])
	if assetResp.StatusCode != http.StatusOK {
		t.Fatalf("资源 %s 状态码 = %d", asset[1], assetResp.StatusCode)
	}
	if cache := assetResp.Header.Get("Cache-Control"); !strings.Contains(cache, "immutable") {
		t.Errorf("带哈希的资源应长期缓存，实际 %q", cache)
	}

	// SPA 回落：未知子路径交给前端路由，且不能被长期缓存
	spaResp, spaBody := get(t, ts, "/navkit/some/deep/route")
	if spaResp.StatusCode != http.StatusOK || !strings.Contains(spaBody, `id="root"`) {
		t.Errorf("SPA 回落失败: %d", spaResp.StatusCode)
	}
	if cache := spaResp.Header.Get("Cache-Control"); cache != "no-cache" {
		t.Errorf("index.html 不应被长期缓存，实际 %q", cache)
	}

	// 无尾斜杠要重定向
	redirect, _ := get(t, ts, "/navkit")
	if redirect.StatusCode != http.StatusMovedPermanently {
		t.Errorf("/navkit 状态码 = %d, want 301", redirect.StatusCode)
	}

	// 放宽只作用于 navkit，gonav 自己的页面仍然严格
	home, _ := get(t, ts, "/")
	if csp := home.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "img-src 'self' data:;") {
		t.Errorf("首页 CSP 被意外放宽: %q", csp)
	}

	// 缺失的静态资源不能回落成 HTML，否则浏览器会把 HTML 当 JS 解析
	missing, _ := get(t, ts, "/navkit/assets/does-not-exist.js")
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("缺失资源状态码 = %d, want 404", missing.StatusCode)
	}
}

// TestUserScriptAPI 覆盖极客模式的沙箱用户脚本：
// 脚本由宿主托管成「包装页 + 外链脚本」，因此不需要放松 CSP。
func TestUserScriptAPI(t *testing.T) {
	ts, _ := newTestServer(t)

	send := func(method, path, body string) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp, string(raw)
	}

	// 初始为空
	list, body := send(http.MethodGet, "/api/userscripts", "")
	if list.StatusCode != http.StatusOK || !strings.Contains(body, `"scripts": []`) {
		t.Fatalf("初始列表异常: %d %s", list.StatusCode, body)
	}

	// 保存一个脚本
	code := "parent.postMessage({ns:'navkit-plugin',v:1,type:'ready'},'*');"
	put, putBody := send(http.MethodPut, "/api/userscripts/demo", `{"name":"演示 <脚本>","code":`+strconv.Quote(code)+`,"permissions":["storage"]}`)
	if put.StatusCode != http.StatusOK {
		t.Fatalf("保存失败: %d %s", put.StatusCode, putBody)
	}
	if !strings.Contains(putBody, "/navkit-userscript/demo.html") {
		t.Errorf("响应缺少包装页地址: %s", putBody)
	}

	// 包装页：外链引入脚本，且标题做了转义
	wrap, wrapBody := send(http.MethodGet, "/navkit-userscript/demo.html", "")
	if wrap.StatusCode != http.StatusOK {
		t.Fatalf("包装页状态码 = %d", wrap.StatusCode)
	}
	if !strings.Contains(wrapBody, `<script src="/navkit-userscript/demo.js"></script>`) {
		t.Errorf("包装页未外链脚本: %s", wrapBody)
	}
	if strings.Contains(wrapBody, "<script>") {
		t.Error("包装页不应包含内联脚本（会被 CSP 拦下）")
	}
	if strings.Contains(wrapBody, "<style>") {
		t.Error("包装页不应包含内联样式（会被 style-src 'self' 拦下）")
	}
	if !strings.Contains(wrapBody, `<link rel="stylesheet" href="/navkit-userscript/base.css">`) {
		t.Errorf("包装页未外链基础样式: %s", wrapBody)
	}

	// 基础样式本身可访问
	css, cssBody := send(http.MethodGet, "/navkit-userscript/base.css", "")
	if css.StatusCode != http.StatusOK || !strings.Contains(cssBody, "font:") {
		t.Errorf("基础样式异常: %d", css.StatusCode)
	}
	if !strings.Contains(wrapBody, "演示 &lt;脚本&gt;") {
		t.Errorf("标题未转义: %s", wrapBody)
	}

	// 脚本本体
	js, jsBody := send(http.MethodGet, "/navkit-userscript/demo.js", "")
	if js.StatusCode != http.StatusOK || !strings.Contains(jsBody, "navkit-plugin") {
		t.Fatalf("脚本本体异常: %d", js.StatusCode)
	}
	if ct := js.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("脚本 Content-Type = %q", ct)
	}

	// 未知脚本 404
	if missing, _ := send(http.MethodGet, "/navkit-userscript/nope.html", ""); missing.StatusCode != http.StatusNotFound {
		t.Errorf("未知脚本状态码 = %d, want 404", missing.StatusCode)
	}

	// 非法 id 与超大脚本都要挡住
	if bad, _ := send(http.MethodPut, "/api/userscripts/BAD ID", `{"name":"x","code":"1"}`); bad.StatusCode != http.StatusBadRequest {
		t.Errorf("非法 id 状态码 = %d, want 400", bad.StatusCode)
	}
	huge := strings.Repeat("a", maxUserScriptBytes+1)
	if big, _ := send(http.MethodPut, "/api/userscripts/huge", `{"name":"x","code":`+strconv.Quote(huge)+`}`); big.StatusCode != http.StatusBadRequest {
		t.Errorf("超大脚本状态码 = %d, want 400", big.StatusCode)
	}

	// 删除后不可访问
	if del, _ := send(http.MethodDelete, "/api/userscripts/demo", ""); del.StatusCode != http.StatusOK {
		t.Errorf("删除状态码 = %d", del.StatusCode)
	}
	if gone, _ := send(http.MethodGet, "/navkit-userscript/demo.html", ""); gone.StatusCode != http.StatusNotFound {
		t.Errorf("删除后状态码 = %d, want 404", gone.StatusCode)
	}

}

// TestRestrainedShell 锁定「克制的日常工具风」：结构要简单，且不得回流此前的炫技元素。
// 同时确保模板不产生内联样式（服务端下发了严格 CSP：style-src 'self'）。
func TestRestrainedShell(t *testing.T) {
	ts, _ := newTestServer(t)

	_, index := get(t, ts, "/")
	for _, want := range []string{
		`class="topbar"`, `class="wordmark"`, `class="hero__title"`,
		`data-search-form`, `data-search-input`, `data-example="jwxt"`,
		`data-chips`, `class="chip`, `data-grid`, `class="cat"`,
		`data-fav`, `data-clock`, `data-theme-toggle`, `data-toast`,
		`class="footer"`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("首页缺少结构: %q", want)
		}
	}

	// 炫技元素不得回流
	for _, unwanted := range []string{
		`data-intro`, `data-cursor`, `data-progress`, `data-marquee`,
		`data-cycler`, `data-kinetic`, `class="grain"`, `footer__wordmark`, `class="ticker"`,
	} {
		if strings.Contains(index, unwanted) {
			t.Errorf("首页不应再出现装饰性元素: %q", unwanted)
		}
	}

	if strings.Contains(index, ` style="`) {
		t.Error("模板中出现内联 style 属性，会被严格 CSP 拦截")
	}

	_, css := get(t, ts, "/static/css/app.css")
	// 荧光酸绿、混色光标、无限跑马灯都应收起
	for _, unwanted := range []string{"#d8ff00", "mix-blend-mode", "@keyframes"} {
		if strings.Contains(strings.ToLower(css), unwanted) {
			t.Errorf("样式表仍包含炫技实现: %q", unwanted)
		}
	}

	// 自托管字体：CSS 引用 + 资源可访问
	if !strings.Contains(css, `url("../fonts/archivo-600.woff2")`) {
		t.Error("CSS 未引用自托管字体")
	}
	fontResp, _ := get(t, ts, "/static/fonts/archivo-600.woff2")
	if fontResp.StatusCode != http.StatusOK {
		t.Errorf("字体资源状态码 = %d", fontResp.StatusCode)
	}
}
