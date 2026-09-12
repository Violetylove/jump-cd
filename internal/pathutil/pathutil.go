// Package pathutil 集中处理所有平台相关的路径差异。
//
// 设计约束（见 AGENTS.md 硬性约束 9）：业务代码只使用本包规范化后的路径，
// 不得在别处散落 runtime.GOOS 分支。
package pathutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	// ErrEmpty 表示路径为空。
	ErrEmpty = errors.New("pathutil: empty path")
	// ErrNewline 表示路径含换行符，不能作为协议输出。
	ErrNewline = errors.New("pathutil: path contains newline")
)

var driveLetter = regexp.MustCompile("^[A-Za-z]:$")

// IsWindows 报告当前是否运行在 Windows 上。
func IsWindows() bool { return runtime.GOOS == "windows" }

// ContainsNewline 报告字符串中是否含换行符。
// 换行符会破坏「一行一个路径」的协议，必须在输出前拦下。
func ContainsNewline(s string) bool {
	return strings.IndexByte(s, '\n') >= 0 || strings.IndexByte(s, '\r') >= 0
}

// Home 返回用户主目录（已规范化）。
func Home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pathutil: resolve home: %w", err)
	}
	return Normalize(h)
}

// Normalize 把用户输入或系统路径转成规范的存储形式：
// 展开 ~、转绝对路径、清理 . 与 ..、去掉末尾分隔符、Windows 盘符大写。
// 它不解析符号链接（那是 Canonical 的职责）。
func Normalize(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", ErrEmpty
	}
	if ContainsNewline(p) {
		return "", ErrNewline
	}

	p = expandHome(p)

	if IsWindows() && strings.HasPrefix(p, "/") {
		// git-bash / cygwin 里的 $PWD 是 /c/Users/x、/tmp/x 这类 MSYS 形式，
		// 而 jcd 是原生程序：直接 filepath.Abs 会得到 C:\c\Users\x 这种鬼东西。
		native, ok := msysToWindows(p)
		if !ok {
			return "", fmt.Errorf("pathutil: %q 是 MSYS 伪路径，无法解析成 Windows 路径", p)
		}
		p = native
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("pathutil: abs %q: %w", p, err)
	}
	abs = filepath.Clean(abs)

	if IsWindows() {
		abs = upperDrive(abs)
	}
	return abs, nil
}

// Canonical 在 Normalize 的基础上解析符号链接。
// 路径不存在时退回 Normalize 的结果，不报错。
func Canonical(p string) (string, error) {
	n, err := Normalize(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(n)
	if err != nil {
		return n, nil
	}
	return Normalize(resolved)
}

// Segments 把路径拆成有意义的段。
// Windows 盘符段（如 C:）会被丢弃，因为它不承载目录语义。
func Segments(p string) []string {
	s := strings.Trim(filepath.ToSlash(p), "/")
	if s == "" {
		return nil
	}
	raw := strings.Split(s, "/")
	out := make([]string, 0, len(raw))
	for _, seg := range raw {
		if seg == "" || seg == "." || seg == ".." || driveLetter.MatchString(seg) {
			continue
		}
		out = append(out, seg)
	}
	return out
}

// Base 返回路径的最后一段；根目录返回自身。
func Base(p string) string {
	segs := Segments(p)
	if len(segs) == 0 {
		return p
	}
	return segs[len(segs)-1]
}

// Parent 返回父目录；已是根目录时返回自身。
func Parent(p string) string {
	d := filepath.Dir(p)
	if Equal(d, p) {
		return p
	}
	return d
}

// Equal 按平台语义比较两条路径（Windows 大小写不敏感）。
func Equal(a, b string) bool {
	if IsWindows() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// SameParent 报告两条路径是否同父且互不相同。
func SameParent(a, b string) bool {
	if Equal(a, b) {
		return false
	}
	return Equal(Parent(a), Parent(b))
}

// IsUnder 报告 child 是否位于 parent 之下（含相等）。
func IsUnder(parent, child string) bool {
	parent = trimSeps(parent)
	child = trimSeps(child)
	if parent == "" {
		return false
	}
	if Equal(parent, child) {
		return true
	}

	pc, cc := fold(parent), fold(child)
	if !strings.HasPrefix(cc, pc) {
		return false
	}
	rest := cc[len(pc):]

	if strings.HasPrefix(rest, string(filepath.Separator)) {
		return true
	}
	// Windows 上 / 与反斜杠等价；POSIX 上反斜杠是合法文件名字符，不能当分隔符。
	return IsWindows() && strings.HasPrefix(rest, "/")
}

// ValidateOutput 校验路径可以作为协议输出。
func ValidateOutput(p string) error {
	if p == "" {
		return ErrEmpty
	}
	if ContainsNewline(p) {
		return ErrNewline
	}
	return nil
}

func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !(len(p) > 1 && p[0] == '~' && isSep(p[1])) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if len(p) == 1 {
		return home
	}
	return filepath.Join(home, p[2:])
}

// msysToWindows 把 MSYS / Cygwin 风格的路径转成原生 Windows 形式：
//
//	/c/Users/x     -> C:/Users/x       （盘符挂载，git-bash 下 cd 出来的就是这种）
//	/cygdrive/c/x  -> C:/Users/x
//	//server/share -> \\server\share   （UNC 的正斜杠写法）
//	/tmp/x         -> %TEMP%\x
//
// 第二项是关键：git-bash 把 /tmp 映射到 Windows 临时目录，但 $PWD 里写的是
// /tmp/...，原生程序不认。至于 /usr、/home 这类 MSYS 内部伪路径，返回 false
// 让调用方明确报错——猜一个盘符只会把用户带到不存在的目录。
func msysToWindows(p string) (string, bool) {
	if len(p) < 2 || p[0] != '/' {
		return "", false
	}

	if p[1] == '/' {
		return filepath.FromSlash(p), true
	}

	const cygdrive = "/cygdrive/"
	if strings.HasPrefix(p, cygdrive) {
		rest := p[len(cygdrive):]
		if len(rest) >= 2 && isDriveLetter(rest[0]) && rest[1] == '/' {
			return strings.ToUpper(rest[:1]) + ":" + rest[1:], true
		}
		return "", false
	}

	if isDriveLetter(p[1]) {
		if len(p) == 2 {
			return strings.ToUpper(p[1:2]) + ":" + string(filepath.Separator), true
		}
		if p[2] == '/' {
			return strings.ToUpper(p[1:2]) + ":" + p[2:], true
		}
	}

	if p == "/tmp" || strings.HasPrefix(p, "/tmp/") {
		tmp := os.TempDir()
		if p == "/tmp" {
			return tmp, true
		}
		return filepath.Join(tmp, p[len("/tmp/"):]), true
	}

	return "", false
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func upperDrive(p string) string {
	vol := filepath.VolumeName(p)
	if len(vol) == 2 && vol[1] == ':' {
		return strings.ToUpper(vol[:1]) + p[1:]
	}
	return p
}

func isSep(c byte) bool {
	return c == '/' || c == filepath.Separator
}

func trimSeps(p string) string {
	for len(p) > 0 && isSep(p[len(p)-1]) {
		p = p[:len(p)-1]
	}
	return p
}

func fold(p string) string {
	if IsWindows() {
		return strings.ToLower(p)
	}
	return p
}
