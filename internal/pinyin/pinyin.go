// Package pinyin 提供汉字到无声调全拼与拼音首字母的转换。
// 读音表在编译期内嵌，无外部依赖与网络请求。
package pinyin

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

type dict struct {
	runes []rune
	idx   []uint16
	syl   []string
}

var (
	once  sync.Once
	table *dict
)

func load() *dict {
	once.Do(func() {
		d, err := decode(tableGzipBase64)
		if err != nil {
			// 内嵌数据损坏时退化为「无拼音」，不影响导航页可用。
			d = &dict{}
		}
		table = d
	})
	return table
}

func decode(encoded string) (*dict, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 解码: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("gzip 解压: %w", err)
	}
	defer zr.Close()
	plain, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("读取解压数据: %w", err)
	}

	buf := bytes.NewReader(plain)
	count, err := binary.ReadUvarint(buf)
	if err != nil {
		return nil, err
	}
	d := &dict{
		runes: make([]rune, 0, count),
		idx:   make([]uint16, 0, count),
	}
	var prev rune
	for i := uint64(0); i < count; i++ {
		delta, err := binary.ReadUvarint(buf)
		if err != nil {
			return nil, err
		}
		syllable, err := binary.ReadUvarint(buf)
		if err != nil {
			return nil, err
		}
		prev += rune(delta)
		d.runes = append(d.runes, prev)
		d.idx = append(d.idx, uint16(syllable))
	}

	syllableCount, err := binary.ReadUvarint(buf)
	if err != nil {
		return nil, err
	}
	d.syl = make([]string, 0, syllableCount)
	for i := uint64(0); i < syllableCount; i++ {
		n, err := binary.ReadUvarint(buf)
		if err != nil {
			return nil, err
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(buf, b); err != nil {
			return nil, err
		}
		d.syl = append(d.syl, string(b))
	}
	return d, nil
}

func (d *dict) syllable(r rune) (string, bool) {
	i := sort.Search(len(d.runes), func(i int) bool { return d.runes[i] >= r })
	if i < len(d.runes) && d.runes[i] == r {
		return d.syl[d.idx[i]], true
	}
	return "", false
}

// Syllable 返回单个汉字的无声调拼音。
func Syllable(r rune) (string, bool) {
	return load().syllable(r)
}

// Full 返回无声调全拼；字母与数字原样保留（小写），其他字符作为分词边界丢弃。
func Full(s string) string {
	d := load()
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		if syllable, ok := d.syllable(r); ok {
			b.WriteString(syllable)
			continue
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
		}
	}
	return b.String()
}

// Initials 返回首字母串：汉字取音节首字母，连续的字母数字串取首字母。
func Initials(s string) string {
	d := load()
	var b strings.Builder
	b.Grow(len(s))
	inWord := false
	for _, r := range s {
		if syllable, ok := d.syllable(r); ok {
			b.WriteByte(syllable[0])
			inWord = false
			continue
		}
		switch {
		case r >= 'a' && r <= 'z':
			if !inWord {
				b.WriteRune(r)
			}
			inWord = true
		case r >= 'A' && r <= 'Z':
			if !inWord {
				b.WriteRune(r - 'A' + 'a')
			}
			inWord = true
		case r >= '0' && r <= '9':
			if !inWord {
				b.WriteRune(r)
			}
			inWord = true
		default:
			inWord = false
		}
	}
	return b.String()
}

// Join 用 f 逐个转换 items 并以空格连接，空结果会被跳过。
func Join(items []string, f func(string) string) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if converted := f(item); converted != "" {
			parts = append(parts, converted)
		}
	}
	return strings.Join(parts, " ")
}
