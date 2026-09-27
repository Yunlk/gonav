package stats

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHitTopAndFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "stats.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	if s.Total() != 0 {
		t.Fatalf("初始 total = %d", s.Total())
	}

	for i := 0; i < 5; i++ {
		s.Hit("lms")
	}
	for i := 0; i < 2; i++ {
		s.Hit("opac")
	}
	s.Hit("") // 空 id 应被忽略
	s.Hit("hpc")

	if got := s.Total(); got != 8 {
		t.Errorf("Total = %d, want 8", got)
	}
	if got := s.Hits("lms"); got != 5 {
		t.Errorf("Hits(lms) = %d", got)
	}

	top := s.Top(2)
	if len(top) != 2 {
		t.Fatalf("Top(2) 长度 = %d", len(top))
	}
	if top[0].ID != "lms" || top[0].Hits != 5 {
		t.Errorf("Top[0] = %+v", top[0])
	}
	if top[1].ID != "opac" {
		t.Errorf("Top[1] = %+v", top[1])
	}

	if err := s.Flush(); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("统计文件未生成: %v", err)
	}

	// 重新打开应恢复计数。
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("重新 Open 失败: %v", err)
	}
	if got := reopened.Total(); got != 8 {
		t.Errorf("恢复后 Total = %d, want 8", got)
	}
	if got := reopened.Hits("lms"); got != 5 {
		t.Errorf("恢复后 Hits(lms) = %d", got)
	}
}

func TestOpenToleratesMissingAndCorruptFiles(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "none.json")
	s, err := Open(missing)
	if err != nil {
		t.Fatalf("文件缺失不应报错: %v", err)
	}
	if s.Total() != 0 {
		t.Errorf("total = %d", s.Total())
	}
	// 没有数据时 Flush 不应创建文件。
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Error("无数据时不应写出文件")
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(corrupt)
	if err == nil {
		t.Error("损坏文件应返回错误以便告警")
	}
	if s2 == nil || s2.Total() != 0 {
		t.Error("损坏文件时应回退为空统计")
	}
}

func TestRunFlushesOnCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stats.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx, 10*time.Millisecond)
		close(done)
	}()

	s.Hit("opac")
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 未随 ctx 退出")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := reopened.Hits("opac"); got != 1 {
		t.Errorf("退出时未收尾落盘, Hits(opac) = %d", got)
	}
}
