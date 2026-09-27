package web

import (
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// userScript 是用户在「极客模式」里粘贴的一段 JS。
//
// 它**不会**在主页面里执行：宿主为每个脚本生成一个包装页
// （/navkit-userscript/<id>.html），里面用外链 <script> 引入脚本本体，
// 再由 navkit 以 sandbox="allow-scripts" 的 iframe 加载。
// 这样既满足「自定义 JS 以沙箱用户脚本方式运行」，又不用放松页面 CSP。
type userScript struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Permissions []string  `json:"permissions"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const (
	maxUserScriptBytes = 256 * 1024
	maxUserScripts     = 64
)

var userScriptIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-_.]{0,63}$`)

type userScriptStore struct {
	mu      sync.RWMutex
	path    string
	scripts map[string]userScript
}

func openUserScriptStore(path string) (*userScriptStore, error) {
	s := &userScriptStore{path: path, scripts: map[string]userScript{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("读取用户脚本 %s: %w", path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	var list []userScript
	if err := json.Unmarshal(raw, &list); err != nil {
		return s, fmt.Errorf("解析用户脚本 %s: %w", path, err)
	}
	for _, script := range list {
		s.scripts[script.ID] = script
	}
	return s, nil
}

func (s *userScriptStore) list() []userScript {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]userScript, 0, len(s.scripts))
	for _, script := range s.scripts {
		out = append(out, script)
	}
	return out
}

func (s *userScriptStore) get(id string) (userScript, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	script, ok := s.scripts[id]
	return script, ok
}

func (s *userScriptStore) put(script userScript) error {
	s.mu.Lock()
	if _, exists := s.scripts[script.ID]; !exists && len(s.scripts) >= maxUserScripts {
		s.mu.Unlock()
		return fmt.Errorf("用户脚本数量上限为 %d", maxUserScripts)
	}
	script.UpdatedAt = time.Now()
	s.scripts[script.ID] = script
	payload := s.listLocked()
	s.mu.Unlock()
	return s.flush(payload)
}

func (s *userScriptStore) remove(id string) error {
	s.mu.Lock()
	delete(s.scripts, id)
	payload := s.listLocked()
	s.mu.Unlock()
	return s.flush(payload)
}

func (s *userScriptStore) listLocked() []userScript {
	out := make([]userScript, 0, len(s.scripts))
	for _, script := range s.scripts {
		out = append(out, script)
	}
	return out
}

func (s *userScriptStore) flush(list []userScript) error {
	if s.path == "" {
		return nil
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ---------------------------------------------------------------- HTTP 层

func (s *Server) handleAPIUserScripts(w http.ResponseWriter, r *http.Request) {
	if s.scripts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "用户脚本未启用"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scripts": s.scripts.list()})
}

func (s *Server) handleAPIUserScriptPut(w http.ResponseWriter, r *http.Request) {
	if s.scripts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "用户脚本未启用"})
		return
	}

	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	if !userScriptIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 只能是小写字母、数字、- _ .（1-64 位）"})
		return
	}

	var payload struct {
		Name        string   `json:"name"`
		Code        string   `json:"code"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUserScriptBytes+4096)).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体不是合法 JSON: " + err.Error()})
		return
	}
	if len(payload.Code) > maxUserScriptBytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "脚本过大（上限 256KB）"})
		return
	}

	script := userScript{
		ID:          id,
		Name:        strings.TrimSpace(payload.Name),
		Code:        payload.Code,
		Permissions: payload.Permissions,
	}
	if script.Name == "" {
		script.Name = id
	}
	if err := s.scripts.put(script); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("用户脚本已保存", "id", id, "permissions", script.Permissions, "bytes", len(script.Code))
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        script.ID,
		"wrapUrl":   "/navkit-userscript/" + script.ID + ".html",
		"scriptUrl": "/navkit-userscript/" + script.ID + ".js",
	})
}

func (s *Server) handleAPIUserScriptDelete(w http.ResponseWriter, r *http.Request) {
	if s.scripts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "用户脚本未启用"})
		return
	}
	if err := s.scripts.remove(strings.ToLower(strings.TrimSpace(r.PathValue("id")))); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// userScriptBaseCSS 是包装页的默认样式。
//
// 必须走**外链**：包装页受严格 CSP（style-src 'self'）管辖，
// 内联 <style> 会被浏览器拒掉，还会在控制台留下违规记录。
const userScriptBaseCSS = `html, body {
  margin: 0;
  height: 100%;
  font: 13px/1.6 system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
  color: var(--nk-fg, #1c1c1e);
  background: transparent;
}
`

// handleUserScriptFile 同时提供包装页、脚本本体与基础样式。
func (s *Server) handleUserScriptFile(w http.ResponseWriter, r *http.Request) {
	if s.scripts == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/navkit-userscript/")

	if name == "base.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(userScriptBaseCSS))
		return
	}

	ext := filepath.Ext(name)
	id := strings.ToLower(strings.TrimSuffix(name, ext))

	script, ok := s.scripts.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	switch ext {
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(script.Code))
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(userScriptWrapper(script)))
	default:
		http.NotFound(w, r)
	}
}

// userScriptWrapper 生成包装页：样式与脚本都走外链，因此包装页不需要任何 CSP 例外。
func userScriptWrapper(script userScript) string {
	return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>` + html.EscapeString(script.Name) + `</title>
<link rel="stylesheet" href="/navkit-userscript/base.css">
</head>
<body>
<div id="root"></div>
<script src="/navkit-userscript/` + script.ID + `.js"></script>
</body>
</html>
`
}
