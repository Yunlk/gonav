// Package stats 记录导航入口的点击量并定期落盘。
// 统计不可用时不影响导航页本身。
package stats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type fileFormat struct {
	Counts    map[string]int64 `json:"counts"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Total     int64            `json:"total"`
}

// Entry 是一条聚合后的点击记录。
type Entry struct {
	ID   string `json:"id"`
	Hits int64  `json:"hits"`
}

// Store 是并发安全的点击计数器。
type Store struct {
	mu      sync.Mutex
	path    string
	counts  map[string]int64
	total   int64
	dirty   bool
	updated time.Time
}

// Open 打开（或创建）统计文件。文件不存在时返回空统计；
// 文件损坏时返回空统计并附带错误，调用方可以继续运行。
func Open(path string) (*Store, error) {
	s := &Store{path: path, counts: make(map[string]int64)}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, fmt.Errorf("读取统计文件 %s: %w", path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		return s, fmt.Errorf("解析统计文件 %s: %w", path, err)
	}
	if f.Counts != nil {
		s.counts = f.Counts
	}
	s.total = f.Total
	s.updated = f.UpdatedAt
	return s, nil
}

// Path 返回统计文件路径。
func (s *Store) Path() string { return s.path }

// Hit 记录一次点击。
func (s *Store) Hit(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[id]++
	s.total++
	s.dirty = true
	s.updated = time.Now()
}

// Hits 返回某个入口的点击量。
func (s *Store) Hits(id string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[id]
}

// Total 返回累计点击量。
func (s *Store) Total() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// Top 返回点击量最高的 n 个入口，降序排列。
func (s *Store) Top(n int) []Entry {
	s.mu.Lock()
	out := make([]Entry, 0, len(s.counts))
	for id, hits := range s.counts {
		out = append(out, Entry{ID: id, Hits: hits})
	}
	s.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		return out[i].ID < out[j].ID
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// Flush 在有待写入数据时落盘。
func (s *Store) Flush() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	payload := fileFormat{
		Counts:    make(map[string]int64, len(s.counts)),
		UpdatedAt: s.updated,
		Total:     s.total,
	}
	for k, v := range s.counts {
		payload.Counts[k] = v
	}
	path := s.path
	s.dirty = false
	s.mu.Unlock()

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			s.markDirty()
			return fmt.Errorf("创建统计目录 %s: %w", dir, err)
		}
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		s.markDirty()
		return fmt.Errorf("序列化统计: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		s.markDirty()
		return fmt.Errorf("写入统计 %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		s.markDirty()
		return fmt.Errorf("替换统计文件 %s: %w", path, err)
	}
	return nil
}

func (s *Store) markDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

// Run 周期性落盘，直到 ctx 结束；结束时再做一次收尾写入。
func (s *Store) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := s.Flush(); err != nil {
				slog.Warn("收尾写入统计失败", "err", err)
			}
			return
		case <-ticker.C:
			if err := s.Flush(); err != nil {
				slog.Warn("写入统计失败", "err", err)
			}
		}
	}
}
