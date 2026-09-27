package web

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		query string
		want  string
	}{
		{"命中名称", "馆藏检索", "馆藏", `<mark class="hl">馆藏</mark>检索`},
		{"多处命中", "图书馆官网 图书馆", "图书馆", `<mark class="hl">图书馆</mark>官网 <mark class="hl">图书馆</mark>`},
		{"大小写不敏感", "GitHub 导航", "github", `<mark class="hl">GitHub</mark> 导航`},
		{"未命中保持原文", "教务系统", "知网", "教务系统"},
		{"纯拼音不高亮", "选课系统", "xkxt", "选课系统"},
		{"转义尖括号", "a<bb>c", "bb", "a&lt;<mark class=\"hl\">bb</mark>&gt;c"},
		{"引号被转义", `名称"XY"`, "xy", `名称&#34;<mark class="hl">XY</mark>&#34;`},
		{"单个 ASCII 字母不参与", "abc", "a", "abc"},
		{"空查询", "教务系统", "  ", "教务系统"},
		{"空文本", "", "教务", ""},
		{"多词取更早者", "abc def", "def abc", `<mark class="hl">abc</mark> <mark class="hl">def</mark>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(highlight(tc.text, tc.query)); got != tc.want {
				t.Errorf("highlight(%q, %q) = %q, want %q", tc.text, tc.query, got, tc.want)
			}
		})
	}
}

func TestHighlightNeverLeaksTags(t *testing.T) {
	got := string(highlight(`<script>alert(1)</script>`, "script"))
	if strings.Contains(got, "<script>") {
		t.Fatalf("未转义脚本标签: %s", got)
	}
	if !strings.Contains(got, "&lt;") {
		t.Fatalf("期望转义后的实体: %s", got)
	}
}
