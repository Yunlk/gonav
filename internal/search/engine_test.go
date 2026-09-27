package search

import (
	"net/url"
	"strings"
	"testing"
)

func TestDefaultEngineIsInternal(t *testing.T) {
	def := DefaultEngine()
	if !def.Internal {
		t.Fatalf("默认引擎应为站内检索，实际 %+v", def)
	}
	if def.ID != "internal" {
		t.Fatalf("默认引擎 id = %q", def.ID)
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"baidu", "baidu", true},
		{"Baidu", "baidu", true},
		{"  google ", "google", true},
		{"internal", "internal", true},
		{"unknown-engine", "", false},
	}
	for _, tc := range tests {
		got, ok := Find(tc.in)
		if ok != tc.ok {
			t.Errorf("Find(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if got.ID != tc.want {
			t.Errorf("Find(%q) = %q, want %q", tc.in, got.ID, tc.want)
		}
	}
}

func TestEngineURL(t *testing.T) {
	baidu, _ := Find("baidu")
	got := baidu.URL("东软 内网")
	want := "https://www.baidu.com/s?wd=" + url.QueryEscape("东软 内网")
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}

	internal, _ := Find("internal")
	if got := internal.URL("图书馆"); got != "/search?q=%E5%9B%BE%E4%B9%A6%E9%A6%86" {
		t.Errorf("站内 URL = %q", got)
	}

	if got := baidu.URL("   "); got != "" {
		t.Errorf("空查询应返回空串，实际 %q", got)
	}
	if got := baidu.URL("a&b=c"); strings.Contains(got, "a&b=c") {
		t.Errorf("查询参数未被转义: %q", got)
	}
}

func TestEnginesShape(t *testing.T) {
	all := Engines()
	if len(all) < 3 {
		t.Fatalf("至少应有 3 个引擎，实际 %d", len(all))
	}
	seen := map[string]bool{}
	for _, e := range all {
		if e.ID == "" || e.Name == "" || e.Template == "" || e.Placeholder == "" {
			t.Errorf("引擎字段不完整: %+v", e)
		}
		if seen[e.ID] {
			t.Errorf("引擎 id 重复: %s", e.ID)
		}
		seen[e.ID] = true
	}

	// 修改返回值不应影响内部状态。
	all[0].Name = "被篡改"
	if again, _ := Find("internal"); again.Name == "被篡改" {
		t.Error("Engines() 返回了内部切片，调用方可意外修改")
	}

	external := External()
	if len(external) != len(Engines())-1 {
		t.Errorf("External 数量 = %d", len(external))
	}
	for _, e := range external {
		if e.Internal {
			t.Errorf("External 中包含站内引擎: %+v", e)
		}
	}
}
