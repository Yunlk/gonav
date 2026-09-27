package pinyin

import "testing"

func TestFullAndInitials(t *testing.T) {
	cases := []struct {
		in       string
		full     string
		initials string
	}{
		{"教务系统", "jiaowuxitong", "jwxt"},
		{"图书馆官网", "tushuguanguanwang", "tsggw"},
		{"选课系统", "xuankexitong", "xkxt"},
		{"知网 CNKI", "zhiwangcnki", "zwc"},
		{"东软 Go!", "dongruango", "drg"},
		{"ITEST 测试平台", "itestceshipingtai", "icspt"},
		{"网络教学平台", "wangluojiaoxuepingtai", "wljxpt"},
		{"", "", ""},
		{"hello-world_2", "helloworld2", "hw2"},
		{"人工智能学院", "rengongzhinengxueyuan", "rgznxy"},
	}

	for _, tc := range cases {
		if got := Full(tc.in); got != tc.full {
			t.Errorf("Full(%q) = %q, want %q", tc.in, got, tc.full)
		}
		if got := Initials(tc.in); got != tc.initials {
			t.Errorf("Initials(%q) = %q, want %q", tc.in, got, tc.initials)
		}
	}
}

func TestSyllable(t *testing.T) {
	for r, want := range map[rune]string{'图': "tu", '书': "shu", '馆': "guan", '教': "jiao"} {
		got, ok := Syllable(r)
		if !ok || got != want {
			t.Errorf("Syllable(%q) = (%q, %v), want %q", r, got, ok, want)
		}
	}
	if _, ok := Syllable('A'); ok {
		t.Error("非汉字不应返回读音")
	}
	if _, ok := Syllable('𝄞'); ok {
		t.Error("表外字符不应返回读音")
	}
}

func TestTableLoaded(t *testing.T) {
	d := load()
	if len(d.runes) < 20000 {
		t.Fatalf("内嵌读音表过小: %d 个汉字", len(d.runes))
	}
	if len(d.syl) < 300 {
		t.Fatalf("音节表过小: %d", len(d.syl))
	}
}
