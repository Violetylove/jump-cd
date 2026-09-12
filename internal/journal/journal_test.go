package journal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Violetylove/jump-cd/internal/journal"
)

func newJournal(t *testing.T) *journal.Journal {
	t.Helper()
	return journal.Open(filepath.Join(t.TempDir(), "journal"))
}

func TestDrainEmpty(t *testing.T) {
	j := newJournal(t)

	entries, err := j.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v", entries)
	}
	if err := j.Done(); err != nil {
		t.Fatalf("Done: %v", err)
	}
}

func TestAppendThenDrain(t *testing.T) {
	j := newJournal(t)

	if err := j.Append("/a/b", 1000); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := j.Append("/c/d", 2000); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err := j.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Path != "/a/b" || entries[0].Time != 1000 {
		t.Fatalf("entry 0 = %+v", entries[0])
	}
	if entries[1].Path != "/c/d" || entries[1].Time != 2000 {
		t.Fatalf("entry 1 = %+v", entries[1])
	}

	if err := j.Done(); err != nil {
		t.Fatalf("Done: %v", err)
	}
	again, err := j.Drain()
	if err != nil {
		t.Fatalf("Drain after Done: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("journal not cleared: %+v", again)
	}
}

// 折叠失败时不能丢数据：残留文件必须在下次 Drain 里被再次取出。
func TestUnfinishedBatchIsRetried(t *testing.T) {
	j := newJournal(t)

	if err := j.Append("/a", 1000); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := j.Drain(); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	// 刻意不调用 Done，模拟折叠过程中崩溃。

	if err := j.Append("/b", 2000); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err := j.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected the leftover batch to be retried, got %+v", entries)
	}
	if entries[0].Path != "/a" || entries[1].Path != "/b" {
		t.Fatalf("entries = %+v", entries)
	}
}

// shell 侧写的是裸路径（fish / nushell 拿不到 EPOCHSECONDS）。
func TestBarePathLinesAreAccepted(t *testing.T) {
	j := newJournal(t)

	if err := os.WriteFile(j.Path(), nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f, err := os.OpenFile(j.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString("/plain/path" + string(rune(10))); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	f.Close()

	entries, err := j.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "/plain/path" || entries[0].Time != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestPendingDoesNotConsume(t *testing.T) {
	j := newJournal(t)

	if err := j.Append("/a", 1000); err != nil {
		t.Fatalf("Append: %v", err)
	}

	pending, err := j.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending = %+v, %v", pending, err)
	}
	entries, err := j.Drain()
	if err != nil || len(entries) != 1 {
		t.Fatalf("Drain after Pending = %+v, %v", entries, err)
	}
}
