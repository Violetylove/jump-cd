// Package store 用一个 JSON 文件记住你去过哪些目录。
//
// 为什么不是 SQLite：那要背 10MB 静态库和十几毫秒的包级初始化，换来事务与并发安全。
// 对一个「记住我去过哪里」的小工具来说不划算。这里用「文件锁 + 原子改名」，
// 全部代码不到 200 行，而且出问题时你可以直接用眼睛看、用手改。
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

// schemaVersion 写进文件，将来改格式时用于识别。
const schemaVersion = 1

// newline 与 backslash 写成数字常量，是为了在源码里彻底避开转义 ——
// 这类字面量在生成器/模板里极易被吃掉。
const (
	newline   = 10
	backslash = 92
)

// 文件锁参数。锁只是一个空文件，靠 O_EXCL 抢占。
const (
	lockSuffix = ".lock"
	lockStale  = 10 * time.Second
	lockWait   = 3 * time.Second
	lockPoll   = 5 * time.Millisecond
)

// Entry 是一个目录的访问记录。
type Entry struct {
	Visits int64 `json:"visits"`
	Last   int64 `json:"last"`
	First  int64 `json:"first"`
}

// Data 是磁盘上的全部内容。JSON 键用小写，方便你直接读这个文件。
type Data struct {
	Version int              `json:"version"`
	Dirs    map[string]Entry `json:"dirs"`
}

// NewData 返回一份空数据。
func NewData() *Data {
	return &Data{Version: schemaVersion, Dirs: map[string]Entry{}}
}

// Touch 记录一次访问。
func (d *Data) Touch(path string, now int64) {
	e := d.Dirs[path]
	if e.First == 0 {
		e.First = now
	}
	e.Visits++
	e.Last = now
	d.Dirs[path] = e
}

// Prune 删掉已经不存在的目录，返回删除条数。exists 由调用方提供，便于测试。
func (d *Data) Prune(exists func(string) bool) int {
	removed := 0
	for path := range d.Dirs {
		if !exists(path) {
			delete(d.Dirs, path)
			removed++
		}
	}
	return removed
}

// Len 返回记住的目录数。
func (d *Data) Len() int { return len(d.Dirs) }

// TotalVisits 返回累计访问次数。
func (d *Data) TotalVisits() int64 {
	var sum int64
	for _, e := range d.Dirs {
		sum += e.Visits
	}
	return sum
}

// Paths 返回全部路径，已排序，便于稳定输出。
func (d *Data) Paths() []string {
	out := make([]string, 0, len(d.Dirs))
	for p := range d.Dirs {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Store 是磁盘上的目录历史。
type Store struct {
	path string
}

// Open 返回指向某个文件的 Store，不会创建文件。
func Open(path string) *Store { return &Store{path: path} }

// Path 返回数据文件路径。
func (s *Store) Path() string { return s.path }

// Load 只读地读取数据。
//
// 不需要加锁：写是「写临时文件再原子改名」，读者永远看不到半个文件。
// 文件损坏时会把它改名成 .bad 留证，然后返回一份空数据 —— 同时返回一个非 nil 的错误。
// 调用方应当把错误打到 stderr 上继续跑，而不是罢工。
func (s *Store) Load() (*Data, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewData(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: 读取 %s: %w", s.path, err)
	}

	d := NewData()
	if err := json.Unmarshal(b, d); err != nil {
		backup := s.path + ".bad"
		_ = os.Rename(s.path, backup)
		return NewData(), fmt.Errorf("store: %s 解析失败，已备份为 %s 并重新开始: %w", s.path, backup, err)
	}
	if d.Dirs == nil {
		d.Dirs = map[string]Entry{}
	}
	if d.Version == 0 {
		d.Version = schemaVersion
	}
	return d, nil
}

// Update 在文件锁保护下读改写。
//
// fn 拿到的永远是最新数据，所以并发调用不会互相覆盖 —— 这是不用数据库
// 也能保证正确性的关键。锁是空文件，抢不到就等；持有者崩溃留下的陈旧锁会被接管。
func (s *Store) Update(fn func(*Data) error) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	d, err := s.Load()
	if err != nil && d == nil {
		return err
	}
	if err := fn(d); err != nil {
		return err
	}
	return s.save(d)
}

// save 先写临时文件再原子改名，读者与写者因此永远不会看到半截内容。
func (s *Store) save(d *Data) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("store: 序列化: %w", err)
	}
	b = append(b, newline)

	if err := os.MkdirAll(dirOf(s.path), 0o755); err != nil {
		return fmt.Errorf("store: 创建数据目录: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("store: 写临时文件: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store: 生效新数据: %w", err)
	}
	return nil
}

func (s *Store) lock() (func(), error) {
	lockPath := s.path + lockSuffix
	deadline := time.Now().Add(lockWait)

	var lastErr error
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		lastErr = err

		// 抢不到是常态。除了「已存在」，Windows 上还有另一种：
		// 另一个进程正在创建或删除这个名字时，O_EXCL 返回的是「拒绝访问」。
		// 两者都当作「先等等」，真有问题会在超时处带着原始错误报出来。
		if !errors.Is(err, os.ErrExist) && !os.IsPermission(err) {
			return nil, fmt.Errorf("store: 加锁 %s: %w", lockPath, err)
		}

		// 持有者可能已经崩溃，陈旧锁要能接管，否则工具会永久卡死。
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > lockStale {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("store: 等待 %s 超时（另一个进程卡住了？）: %w", lockPath, lastErr)
		}
		time.Sleep(lockPoll)
	}
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == backslash {
			if i == 0 {
				return path[:1]
			}
			return path[:i]
		}
	}
	return "."
}
