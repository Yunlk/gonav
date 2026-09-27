package web

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"sync"
)

// renderer 管理按页面拆分的模板集合。
//
// 生产模式使用 go:embed 打包的模板；dev 模式下从磁盘读取并每次重新解析。
type renderer struct {
	dev  bool
	fsys fs.FS

	mu    sync.RWMutex
	pages map[string]*template.Template
}

func newRenderer(dev bool, devRoot string) (*renderer, error) {
	var root fs.FS
	if dev {
		if devRoot == "" {
			devRoot = "internal/web"
		}
		root = os.DirFS(devRoot)
	} else {
		root = embedded
	}

	r := &renderer{dev: dev, fsys: root, pages: make(map[string]*template.Template)}
	for _, page := range []string{"index", "search", "error"} {
		if err := r.parse(page); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *renderer) parse(page string) error {
	t, err := template.New("layout").Funcs(funcMap()).ParseFS(r.fsys,
		"templates/layout.html",
		"templates/partials.html",
		"templates/"+page+".html",
	)
	if err != nil {
		return fmt.Errorf("解析模板 %s: %w", page, err)
	}
	r.mu.Lock()
	r.pages[page] = t
	r.mu.Unlock()
	return nil
}

func (r *renderer) render(w io.Writer, page string, data any) error {
	if r.dev {
		if err := r.parse(page); err != nil {
			return err
		}
	}
	r.mu.RLock()
	t := r.pages[page]
	r.mu.RUnlock()
	if t == nil {
		return fmt.Errorf("模板 %s 未注册", page)
	}
	return t.ExecuteTemplate(w, "layout", data)
}
