// Package search 定义顶部搜索框可用的搜索引擎。
package search

import (
	"net/url"
	"strings"
)

// Engine 描述一个搜索引擎。
type Engine struct {
	ID          string // 稳定标识，出现在查询参数 engine 中
	Name        string // 下拉框展示名
	Icon        string // 前端图标名
	Template    string // 带 %s 占位符的查询地址模板
	Placeholder string // 搜索框提示语
	Internal    bool   // true 表示由本服务处理，而非跳转公网
	Hint        string // 下拉项说明
}

// URL 使用 query 构造最终地址；站内检索返回相对路径。
func (e Engine) URL(query string) string {
	q := strings.TrimSpace(query)
	if e.Template == "" || q == "" {
		return ""
	}
	return strings.Replace(e.Template, "%s", url.QueryEscape(q), 1)
}

var engines = []Engine{
	{
		ID:          "internal",
		Name:        "站内导航",
		Icon:        "compass",
		Template:    "/search?q=%s",
		Placeholder: "搜索站内系统与资源…",
		Internal:    true,
		Hint:        "在本地导航目录中检索",
	},
	{
		ID:          "baidu",
		Name:        "百度",
		Icon:        "search",
		Template:    "https://www.baidu.com/s?wd=%s",
		Placeholder: "百度一下…",
		Hint:        "全网中文检索",
	},
	{
		ID:          "bing",
		Name:        "必应",
		Icon:        "globe",
		Template:    "https://cn.bing.com/search?q=%s",
		Placeholder: "使用必应搜索…",
		Hint:        "国内可直连",
	},
	{
		ID:          "google",
		Name:        "Google",
		Icon:        "globe",
		Template:    "https://www.google.com/search?q=%s",
		Placeholder: "Search Google…",
		Hint:        "需外网访问",
	},
	{
		ID:          "cnki",
		Name:        "知网",
		Icon:        "book",
		Template:    "https://kns.cnki.net/kns8s/defaultresult/index?kw=%s",
		Placeholder: "检索知网文献…",
		Hint:        "中文文献库",
	},
	{
		ID:          "github",
		Name:        "GitHub",
		Icon:        "code",
		Template:    "https://github.com/search?q=%s",
		Placeholder: "Search GitHub…",
		Hint:        "代码与开源项目",
	},
}

// Engines 返回全部引擎，顺序即下拉框顺序。
func Engines() []Engine {
	out := make([]Engine, len(engines))
	copy(out, engines)
	return out
}

// Find 按 id 查找引擎（大小写不敏感）。
func Find(id string) (Engine, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, e := range engines {
		if e.ID == id {
			return e, true
		}
	}
	return Engine{}, false
}

// DefaultEngine 返回默认引擎（站内检索）。
func DefaultEngine() Engine { return engines[0] }

// External 返回所有需要跳转公网的引擎。
func External() []Engine {
	out := make([]Engine, 0, len(engines))
	for _, e := range engines {
		if !e.Internal {
			out = append(out, e)
		}
	}
	return out
}
