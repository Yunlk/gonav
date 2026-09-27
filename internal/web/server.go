// Package web 提供导航页的 HTTP 层：页面渲染、JSON API 与静态资源。
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gonav/internal/catalog"
	"gonav/internal/gatewayclient"
	"gonav/internal/search"
	"gonav/internal/stats"
)

// Options 是构造 Server 所需的依赖。
type Options struct {
	Store   *catalog.Store       // 链接配置
	Stats   *stats.Store         // 点击统计，可为 nil
	Debug   bool                 // 开发模式：模板与资源每次重新读取
	DevRoot string               // 开发模式下的 web 目录
	Version string               // 展示在页脚的版本号
	Gateway gatewayclient.Client // 反向回调主网关，可为 nil
	// ScriptPath 是用户脚本落盘路径；为空表示关闭极客模式的用户脚本。
	ScriptPath string
}

// Server 是导航页的 HTTP 处理器集合。
type Server struct {
	store   *catalog.Store
	stats   *stats.Store
	render  *renderer
	static  fs.FS
	engines []search.Engine
	debug   bool
	version string
	mux     *http.ServeMux
	// navkit 前端产物（assets/navkit），未构建时为 nil
	navkit fs.FS
	// 反向回调主网关，可为 nil
	gateway gatewayclient.Client
	// 极客模式的沙箱用户脚本，可为 nil
	scripts *userScriptStore
}

// New 构造 Server。
func New(opts Options) (*Server, error) {
	if opts.Store == nil {
		return nil, errors.New("web: Options.Store 不能为空")
	}
	r, err := newRenderer(opts.Debug, opts.DevRoot)
	if err != nil {
		return nil, err
	}

	var static fs.FS
	if opts.Debug {
		root := opts.DevRoot
		if root == "" {
			root = "internal/web"
		}
		static = os.DirFS(filepath.Join(root, "assets"))
	} else {
		static, err = assetsFS()
		if err != nil {
			return nil, fmt.Errorf("web: 读取内置静态资源失败: %w", err)
		}
	}

	s := &Server{
		store:   opts.Store,
		stats:   opts.Stats,
		render:  r,
		static:  static,
		engines: search.Engines(),
		debug:   opts.Debug,
		version: opts.Version,
		gateway: opts.Gateway,
	}

	// navkit 是独立的单页应用，产物由 vite 直接构建到 assets/navkit 下。
	if navkit, subErr := fs.Sub(static, "navkit"); subErr == nil {
		s.navkit = navkit
	}

	if opts.ScriptPath != "" {
		scripts, scriptErr := openUserScriptStore(opts.ScriptPath)
		if scriptErr != nil {
			// 用户脚本不可用不该拦住建站
			slog.Warn("用户脚本存储初始化失败", "path", opts.ScriptPath, "err", scriptErr)
		} else {
			s.scripts = scripts
		}
	}

	s.routes()
	return s, nil
}

// Handler 返回带中间件的完整处理器。
func (s *Server) Handler() http.Handler {
	return recoverer(requestLogger(secureHeaders(s.mux)))
}

func (s *Server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /go/{id}", s.handleGo)
	mux.HandleFunc("GET /api/links", s.handleAPILinks)
	mux.HandleFunc("GET /api/search", s.handleAPISearch)
	mux.HandleFunc("GET /api/stats", s.handleAPIStats)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/userscripts", s.handleAPIUserScripts)
	mux.HandleFunc("PUT /api/userscripts/{id}", s.handleAPIUserScriptPut)
	mux.HandleFunc("DELETE /api/userscripts/{id}", s.handleAPIUserScriptDelete)
	mux.HandleFunc("GET /navkit-userscript/", s.handleUserScriptFile)
	mux.Handle("GET /static/", s.staticHandler())
	if s.navkit != nil {
		mux.HandleFunc("GET /navkit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/navkit/", http.StatusMovedPermanently)
		})
		mux.Handle("GET /navkit/", s.navkitHandler())
	}
	mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	mux.HandleFunc("GET /", s.handleNotFound)
	s.mux = mux
}

// ---------- 视图模型 ----------

type topLink struct {
	Link *catalog.Link
	Hits int64
}

type pageData struct {
	Site       catalog.Site
	Categories []*catalog.Category
	Pinned     []*catalog.Link

	Engines  []search.Engine
	Engine   search.Engine
	Query    string
	IsSearch bool
	Results  []catalog.Match

	Top       []topLink
	TotalHits int64
	TotalLink int
	TotalCat  int

	Version    string
	Year       int
	ConfigPath string
	ConfigTime time.Time
	ConfigErr  string
	Notice     string
	ErrorTitle string
	ErrorHint  string
}

func (s *Server) baseData() pageData {
	c := s.store.Current()
	total := int64(0)
	if s.stats != nil {
		total = s.stats.Total()
	}
	var cfgErr string
	if err := s.store.LastError(); err != nil {
		cfgErr = err.Error()
	}
	return pageData{
		Site:       c.Site,
		Categories: c.Categories,
		Pinned:     c.Pinned(),
		Engines:    s.engines,
		Engine:     search.DefaultEngine(),
		TotalLink:  c.LinkCount(),
		TotalCat:   c.CategoryCount(),
		Top:        s.topLinks(6),
		TotalHits:  total,
		Version:    s.version,
		Year:       time.Now().Year(),
		ConfigPath: c.SourcePath(),
		ConfigTime: c.UpdatedAt(),
		ConfigErr:  cfgErr,
	}
}

// topLinks 将点击统计映射为链接，忽略配置中已删除的入口。
func (s *Server) topLinks(n int) []topLink {
	if s.stats == nil {
		return nil
	}
	c := s.store.Current()
	entries := s.stats.Top(n * 2)
	out := make([]topLink, 0, n)
	for _, e := range entries {
		link, ok := c.Link(e.ID)
		if !ok {
			continue
		}
		out = append(out, topLink{Link: link, Hits: e.Hits})
		if len(out) == n {
			break
		}
	}
	return out
}

// ---------- 页面处理器 ----------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := s.baseData()
	s.writeHTML(w, r, "index", data, http.StatusOK)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	engineID := r.URL.Query().Get("engine")

	engine, ok := search.Find(engineID)
	if !ok {
		engine = search.DefaultEngine()
	}

	// 非站内引擎：直接把请求重定向到目标搜索引擎。
	if !engine.Internal {
		if q == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Redirect(w, r, engine.URL(q), http.StatusFound)
		return
	}

	data := s.baseData()
	data.IsSearch = true
	data.Query = q
	data.Engine = engine
	if q == "" {
		data.Notice = "请输入关键词后再搜索。"
	} else {
		data.Results = s.store.Current().Search(q, 50)
	}
	s.writeHTML(w, r, "search", data, http.StatusOK)
}

// handleGo 记录点击后跳转，统计不依赖前端 JS。
func (s *Server) handleGo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	link, ok := s.store.Current().Link(id)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if s.stats != nil {
		s.stats.Hit(id)
	}
	// 反向回调主网关（契约未定时是 no-op），失败不影响跳转
	if s.gateway != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.gateway.Emit(ctx, gatewayclient.Event{
				Kind:       gatewayclient.EventClickRecorded,
				LinkID:     id,
				OccurredAt: time.Now(),
			})
		}()
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	data := s.baseData()
	data.ErrorTitle = "页面不存在"
	data.ErrorHint = "链接可能已调整，请回到导航首页重新查找。"
	s.writeHTML(w, r, "error", data, http.StatusNotFound)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	f, err := s.static.Open("favicon.svg")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, f)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	c := s.store.Current()
	status := "ok"
	if s.store.LastError() != nil {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     status,
		"links":      c.LinkCount(),
		"categories": c.CategoryCount(),
		"updatedAt":  c.UpdatedAt(),
		"config":     c.SourcePath(),
		"version":    s.version,
	})
}

// ---------- JSON API ----------

func (s *Server) handleAPILinks(w http.ResponseWriter, r *http.Request) {
	c := s.store.Current()
	if catID := r.URL.Query().Get("category"); catID != "" {
		cat, ok := c.Category(catID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "分类不存在: " + catID})
			return
		}
		writeJSON(w, http.StatusOK, cat)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	matches := s.store.Current().Search(q, limit)
	if matches == nil {
		matches = []catalog.Match{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":   q,
		"count":   len(matches),
		"results": matches,
	})
}

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "top": []any{}})
		return
	}
	entries := s.stats.Top(20)
	type item struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
		URL  string `json:"url,omitempty"`
		Hits int64  `json:"hits"`
	}
	c := s.store.Current()
	items := make([]item, 0, len(entries))
	for _, e := range entries {
		it := item{ID: e.ID, Hits: e.Hits}
		if l, ok := c.Link(e.ID); ok {
			it.Name = l.Name
			it.URL = l.URL
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": s.stats.Total(), "top": items})
}

// ---------- 基础设施 ----------

func (s *Server) staticHandler() http.Handler {
	// 资源根目录即 assets/，因此需要剥掉 /static/ 前缀。
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(s.static)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.debug {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// navkitHandler 托管 navkit 单页应用：静态资源直出，未知路径回落到 index.html。
func (s *Server) navkitHandler() http.Handler {
	// FS 根就是 navkit 目录，所以必须先剥掉 URL 前缀（同 /static/ 的处理）。
	files := http.StripPrefix("/navkit/", http.FileServer(http.FS(s.navkit)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/navkit/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(s.navkit, name); err != nil {
			// 缺资源就老实 404，别把 HTML 当成 JS 喂给浏览器
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
			// SPA 路由：/navkit/anything 交给前端自己处理
			name = "index.html"
		}

		switch {
		case s.debug:
			w.Header().Set("Cache-Control", "no-store")
		case strings.HasPrefix(name, "assets/"):
			// vite 产物带内容哈希，可以长期缓存
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}

		// index.html 直接读文件：走 FileServer 会被它规范化重定向到 "./"
		if name == "index.html" {
			http.ServeFileFS(w, r, s.navkit, "index.html")
			return
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) writeHTML(w http.ResponseWriter, r *http.Request, page string, data pageData, status int) {
	var buf strings.Builder
	if err := s.render.render(&buf, page, data); err != nil {
		slog.Error("模板渲染失败", "page", page, "err", err)
		http.Error(w, "模板渲染失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(buf.String()))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

// secureHeaders 施加安全响应头，与前端「无内联脚本/样式」的实现保持一致。
//
// navkit 让用户自己填图标与壁纸，还会加载用户安装的插件：
//   - img-src 放开 data:/blob:（上传壁纸）与 https:（远程图标）
//   - frame-src 放开 https:（第三方插件在沙箱 iframe 里运行）
//   - connect-src 放开 https:（数据源插件要拉远端 JSON）
//
// 其余路径保持严格；插件本身仍然跑在不透明源的沙箱里，拿不到宿主数据。
func secureHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; " +
		"font-src 'self'; connect-src 'self'; form-action 'self' https:; base-uri 'self'; frame-ancestors 'self'"
	const navkitCSP = "default-src 'self'; img-src 'self' data: blob: https:; style-src 'self'; script-src 'self'; " +
		"font-src 'self'; connect-src 'self' https:; frame-src 'self' https:; form-action 'self' https:; " +
		"base-uri 'self'; frame-ancestors 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if r.URL.Path == "/navkit" || strings.HasPrefix(r.URL.Path, "/navkit/") {
			h.Set("Content-Security-Policy", navkitCSP)
		} else {
			h.Set("Content-Security-Policy", csp)
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"dur", time.Since(start).Round(time.Millisecond).String(),
		)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("请求处理 panic", "path", r.URL.Path, "panic", rec)
				http.Error(w, "服务内部错误", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
