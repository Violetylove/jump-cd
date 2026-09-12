package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Violetylove/jump-cd/internal/store"
)

const now = int64(1_700_000_000)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	return store.Open(filepath.Join(t.TempDir(), "dirs.json"))
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	d, err := newStore(t).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Len() != 0 {
		t.Fatalf("新库应当为空，实际 %d 条", d.Len())
	}
}

func TestTouchAndRoundTrip(t *testing.T) {
	s := newStore(t)

	if err := s.Update(func(d *store.Data) error {
		d.Touch("/a/b", now)
		d.Touch("/a/b", now+60)
		d.Touch("/c", now)
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	d, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Len() != 2 {
		t.Fatalf("Len = %d want 2", d.Len())
	}
	e := d.Dirs["/a/b"]
	if e.Visits != 2 || e.First != now || e.Last != now+60 {
		t.Fatalf("entry = %+v", e)
	}
}

// 数据文件是给人看的：键名要能读懂，内容要带缩进换行。
func TestFileIsHumanReadable(t *testing.T) {
	s := newStore(t)
	if err := s.Update(func(d *store.Data) error {
		d.Touch("/home/u/code", now)
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	b, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(b)
	for _, want := range []string{"\"dirs\"", "\"visits\"", "/home/u/code", "\n", "  "} {
		if !strings.Contains(text, want) {
			t.Fatalf("文件里应当出现 %q，实际：\n%s", want, text)
		}
	}
}

// 并发写不能丢更新：这是不用数据库也能保证正确性的关键。
func TestConcurrentUpdateDoesNotLoseWrites(t *testing.T) {
	s := newStore(t)

	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join("/p", string(rune('a'+i)))
			if err := s.Update(func(d *store.Data) error {
				d.Touch(path, now)
				return nil
			}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发 Update: %v", err)
	}

	d, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Len() != workers {
		t.Fatalf("丢了更新：Len = %d want %d", d.Len(), workers)
	}
}

// 文件损坏时不能静默丢数据：留一份 .bad，然后从头开始。
func TestCorruptFileIsBackedUpNotDropped(t *testing.T) {
	s := newStore(t)
	if err := os.MkdirAll(filepath.Dir(s.Path()), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(s.Path(), []byte("{ 这不是 JSON"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	d, err := s.Load()
	if err == nil {
		t.Fatal("期望报错，好让调用方提示用户")
	}
	if d == nil || d.Len() != 0 {
		t.Fatalf("损坏后应当返回空数据，实际 %+v", d)
	}
	if _, statErr := os.Stat(s.Path() + ".bad"); statErr != nil {
		t.Fatalf("原始文件应当被留证为 .bad：%v", statErr)
	}
}

func TestPrune(t *testing.T) {
	d := store.NewData()
	d.Touch("/alive", now)
	d.Touch("/dead", now)

	if removed := d.Prune(func(p string) bool { return p == "/alive" }); removed != 1 {
		t.Fatalf("removed = %d want 1", removed)
	}
	if d.Len() != 1 {
		t.Fatalf("Len = %d want 1", d.Len())
	}
}

func TestPathsAreSorted(t *testing.T) {
	d := store.NewData()
	for _, p := range []string{"/c", "/a", "/b"} {
		d.Touch(p, now)
	}
	got := d.Paths()
	want := []string{"/a", "/b", "/c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Paths = %v want %v", got, want)
		}
	}
}
