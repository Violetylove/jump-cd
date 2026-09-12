// Package journal 实现由 shell 直接追加的访问日志。
//
// 为什么不让 shell 调用 jcd add：
// Windows 上启动一个 Go 二进制实测约 14ms（原生 PowerShell，30 次均值），
// 而每个提示符都要付这笔钱。改成 shell 内建追加后，热路径降到微秒级；
// 二进制只在用户真正敲 jcd 时启动，那时 30ms 完全无感。
//
// 日志是「先追加、后折叠」：每次 cd 写一行，下次 jcd 启动时批量折进数据库。
// 条目自带时间戳，因此折叠时仍能按真实时间做衰减。
package journal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// drainingSuffix 是「正在处理」的中间文件名。
const drainingSuffix = ".draining"

// Entry 是一条待折叠的访问记录。
type Entry struct {
	Path string
	Time int64 // 0 表示 shell 未能提供时间戳，折叠时用当前时间
}

// Journal 是磁盘上的追加日志。
type Journal struct {
	path string
}

// Open 返回指向 path 的日志句柄；不会创建文件。
func Open(path string) *Journal { return &Journal{path: path} }

// Path 返回日志文件路径。
func (j *Journal) Path() string { return j.path }

// Append 追加一条记录。shell 侧的追加走内建重定向，不经过这里；
// 本方法供 jcd add（手工调用或无法内建追加的 shell）使用。
func (j *Journal) Append(path string, now int64) error {
	if err := os.MkdirAll(filepath.Dir(j.path), 0o755); err != nil {
		return fmt.Errorf("journal: 创建目录: %w", err)
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("journal: 打开日志: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%d\t%s\n", now, path); err != nil {
		return fmt.Errorf("journal: 追加: %w", err)
	}
	return nil
}

// Drain 取出全部待处理条目，并把日志从活动路径上摘掉。
//
// 用「改名再读」而不是「读后清空」：改名是原子的，不会与正在追加的 shell
// 抢同一个文件。若后续折叠失败，.draining 会留在磁盘上，下次 Drain 会连同它
// 一起取出——代价是可能重复计一次访问，这比丢掉一次访问更无害。
func (j *Journal) Drain() ([]Entry, error) {
	var out []Entry

	// 先捡起上次没处理完的残留。
	if leftover, err := readFile(j.path + drainingSuffix); err == nil {
		out = append(out, leftover...)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.Rename(j.path, j.path+drainingSuffix); err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("journal: 轮转日志: %w", err)
	}

	fresh, err := readFile(j.path + drainingSuffix)
	if err != nil {
		return out, err
	}
	return append(out, fresh...), nil
}

// Done 在条目成功落库后调用，清掉待处理文件。
func (j *Journal) Done() error {
	if err := os.Remove(j.path + drainingSuffix); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Pending 只读取待处理条目，不改动任何文件。供 jcd doctor 展示。
func (j *Journal) Pending() ([]Entry, error) {
	var out []Entry
	if leftover, err := readFile(j.path + drainingSuffix); err == nil {
		out = append(out, leftover...)
	}
	fresh, err := readFile(j.path)
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	return append(out, fresh...), nil
}

func readFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 32*1024), 1<<20)
	for sc.Scan() {
		if e, ok := parseLine(sc.Text()); ok {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// parseLine 接受两种形态：<epoch>\t<path> 与裸 <path>。
func parseLine(line string) (Entry, bool) {
	if line == "" {
		return Entry{}, false
	}
	if i := strings.IndexByte(line, '\t'); i > 0 && i+1 < len(line) {
		if ts, err := strconv.ParseInt(line[:i], 10, 64); err == nil && ts >= 0 {
			return Entry{Time: ts, Path: line[i+1:]}, true
		}
	}
	return Entry{Path: line}, true
}
