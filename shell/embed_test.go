package shell_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Violetylove/jump-cd/shell"
)

const journal = "C:/data/jump-cd/journal"

// hookMarker 是各 shell 脚本里「把热路径挂到目录变化上」的标志物。
// 不存在的 shell 会在这里露馅，而不是悄悄通过。
var hookMarker = map[string]string{
	"zsh":        "__jcd_hook",
	"bash":       "__jcd_hook",
	"fish":       "__jcd_hook",
	"pwsh":       "__jcd_hook",
	"powershell": "__jcd_hook",
	"nu":         "env_change.PWD",
	"nushell":    "env_change.PWD",
}

func render(t *testing.T, name string) string {
	t.Helper()
	got, err := shell.Script(name, map[string]string{"JOURNAL": journal})
	if err != nil {
		t.Fatalf("Script: %v", err)
	}
	return got
}

func TestScriptSubstitutesJournal(t *testing.T) {
	for _, name := range shell.Supported() {
		t.Run(name, func(t *testing.T) {
			got := render(t, name)
			if strings.Contains(got, "__JCD_") {
				t.Fatalf("未替换的占位符残留：%s", firstPlaceholder(got))
			}
			if !strings.Contains(got, journal) {
				t.Fatal("脚本里没有出现日志路径")
			}
		})
	}
}

// 两条铁律必须由脚本自证：
//  1. 热路径挂在目录变化上；
//  2. 热路径不调用 jcd add —— 那会为每个提示符启动一个进程。
func TestScriptsHonourInvariants(t *testing.T) {
	for _, name := range shell.Supported() {
		t.Run(name, func(t *testing.T) {
			got := render(t, name)

			marker, ok := hookMarker[name]
			if !ok {
				t.Fatalf("shell %q 没有登记 hook 标志物", name)
			}
			if !strings.Contains(got, marker) {
				t.Fatalf("缺少热路径 hook（期望 %q）", marker)
			}
			if strings.Contains(got, "jcd add") {
				t.Fatal("热路径不得调用 jcd add —— 那会为每个提示符启动一个进程")
			}
		})
	}
}

// PowerShell 脚本必须是纯 ASCII。
//
// Windows PowerShell 5.1 用控制台代码页解码外部程序的输出，非 ASCII 注释会被
// 解错，解出来的字符可能破坏语法 —— 表现出来就是「集成没定义出 jcd 函数」。
func TestPowerShellScriptIsASCII(t *testing.T) {
	got := render(t, "powershell")
	for i, r := range got {
		if r > 127 {
			t.Fatalf("jcd.ps1 必须纯 ASCII，第 %d 个字符是 %q", i, r)
		}
	}
}

// PowerShell 集成必须用 Get-Command 解析二进制，不能写死名字：
// Linux/macOS 上的可执行文件叫 jcd，没有 .exe 后缀。
func TestPowerShellResolvesTheBinary(t *testing.T) {
	got := render(t, "powershell")
	if !strings.Contains(got, "Get-Command") {
		t.Fatal("PowerShell 集成应当用 Get-Command 解析二进制，而不是写死名字")
	}
	if !strings.Contains(got, "CommandType Application") {
		t.Fatal("必须按 Application 类型过滤，否则 jcd 会解析到函数本身并无限递归")
	}
}

// 变量展开后面不能紧跟非 ASCII 字符。
//
// macOS 自带的是 bash 3.2：变量名后面直接跟一个多字节字符时，它会把那个字符的
// 字节当成变量名的一部分，于是变量名整个变了，配合 set -u 直接致命。
// Windows 和 Linux 上的 bash 5 完全不会报错 —— 典型的「只有 CI 才看得见」。
// 规矩很简单：变量名一律加花括号。
func TestShellScriptsAvoidVarFollowedByNonASCII(t *testing.T) {
	patterns := []string{
		filepath.Join("..", "scripts", "*"),
		filepath.Join("..", "shell", "jcd.*"),
	}

	checked := 0
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		for _, path := range matches {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("读取 %s: %v", path, err)
			}
			if bad := varThenNonASCII(b); bad != "" {
				t.Fatalf("%s 里有「变量后面紧跟非 ASCII 字符」：%s（给变量名加花括号）", path, bad)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("一个 shell 脚本都没扫到，路径写错了？")
	}
}

// varThenNonASCII 找出形如「美元符号 + 变量名 + 非 ASCII 字符」的片段，返回它用于报错。
func varThenNonASCII(b []byte) string {
	isNameStart := func(c byte) bool {
		return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	isNameByte := func(c byte) bool {
		return isNameStart(c) || (c >= '0' && c <= '9')
	}

	for i := 0; i < len(b); i++ {
		if b[i] != '$' || i+1 >= len(b) || !isNameStart(b[i+1]) {
			continue
		}
		j := i + 1
		for j < len(b) && isNameByte(b[j]) {
			j++
		}
		if j < len(b) && b[j] >= 0x80 {
			end := j + 1
			for end < len(b) && b[end]&0xC0 == 0x80 {
				end++
			}
			return string(b[i:end])
		}
	}
	return ""
}

// 给用户的 PowerShell 接入行必须是真能跑的那种。
//
// 这条踩过：Invoke-Expression 收到的是外部程序输出的字符串数组，不是单个字符串，
// 于是每个 PowerShell 用户的 profile 都会报 "Cannot convert System.Object[]"。
// 当时 e2e 自己用 -join 拼了一遍绕过去了，CI 全绿，用户全挂。
func TestPowerShellInitLineIsUsable(t *testing.T) {
	files := []string{
		filepath.Join("..", "README.md"),
		filepath.Join("..", "internal", "cli", "cli.go"),
		filepath.Join("..", "scripts", "install.ps1"),
		filepath.Join("..", "scripts", "e2e-inner.ps1"),
	}

	found := 0
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s: %v", path, err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if !strings.Contains(line, "Invoke-Expression") || !strings.Contains(line, "jcd init") {
				continue
			}
			// install.ps1 故意留着旧写法，用来识别并修好已经写坏的 profile。
			if strings.Contains(line, "$brokenLine") {
				continue
			}
			found++
			if !strings.Contains(line, "Out-String") {
				t.Fatalf("%s:%d 的 PowerShell 接入行少了 Out-String，会给用户一个跑不起来的 profile：%s",
					path, i+1, strings.TrimSpace(line))
			}
		}
	}
	if found == 0 {
		t.Fatal("一处 PowerShell 接入行都没扫到，路径写错了？")
	}
}

func TestUnknownShellIsRejected(t *testing.T) {
	if _, err := shell.Script("tcsh", nil); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}

func firstPlaceholder(s string) string {
	i := strings.Index(s, "__JCD_")
	if i < 0 {
		return ""
	}
	end := strings.Index(s[i:], "__")
	if end < 0 {
		return s[i:]
	}
	return s[i : i+end+2]
}
