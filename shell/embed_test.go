package shell_test

import (
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
