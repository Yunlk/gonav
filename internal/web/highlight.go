package web

import (
	"html"
	"html/template"
	"strings"
)

// highlight 把 text 中命中 query 的片段包成 <mark class="hl">，其余部分做 HTML 转义。
// 仅做字面匹配；纯拼音命中时没有可高亮的字面片段，原文会原样返回。
func highlight(text, query string) template.HTML {
	terms := highlightTerms(query)
	if text == "" || len(terms) == 0 {
		return template.HTML(html.EscapeString(text)) //nolint:gosec // 已转义
	}

	lower := strings.ToLower(text)
	if len(lower) != len(text) {
		// 少数语言的 ToLower 会改变字节长度，此时放弃高亮以保证切片安全。
		return template.HTML(html.EscapeString(text)) //nolint:gosec // 已转义
	}

	var b strings.Builder
	b.Grow(len(text) + 32)
	for pos := 0; pos < len(text); {
		idx, length := nextMatch(lower, terms, pos)
		if idx < 0 {
			b.WriteString(html.EscapeString(text[pos:]))
			break
		}
		b.WriteString(html.EscapeString(text[pos:idx]))
		b.WriteString(`<mark class="hl">`)
		b.WriteString(html.EscapeString(text[idx : idx+length]))
		b.WriteString(`</mark>`)
		pos = idx + length
	}
	return template.HTML(b.String()) //nolint:gosec // 所有片段均已转义
}

// highlightTerms 拆分关键词：ASCII 词至少 2 个字符，避免单个字母刷满高亮。
func highlightTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if isASCII(f) && len(f) < 2 {
			continue
		}
		terms = append(terms, f)
	}
	return terms
}

// nextMatch 返回 from 之后最早出现的词及其字节长度。
func nextMatch(lower string, terms []string, from int) (int, int) {
	bestIndex, bestLength := -1, 0
	for _, term := range terms {
		offset := strings.Index(lower[from:], term)
		if offset < 0 {
			continue
		}
		index := from + offset
		if bestIndex < 0 || index < bestIndex || (index == bestIndex && len(term) > bestLength) {
			bestIndex, bestLength = index, len(term)
		}
	}
	return bestIndex, bestLength
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
