// Package shell 内嵌各 shell 的集成脚本。
//
// 脚本以本目录的文件为唯一事实来源，模板里的 __JCD_<KEY>__ 占位符由
// jcd init 在渲染时替换（目前只有日志路径）。一致性由 embed_test.go 保证。
package shell

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed jcd.zsh jcd.bash jcd.fish jcd.ps1 jcd.nu
var files embed.FS

// fileFor 是 shell 名到脚本文件的映射，含常用别名。
var fileFor = map[string]string{
	"zsh":        "jcd.zsh",
	"bash":       "jcd.bash",
	"fish":       "jcd.fish",
	"pwsh":       "jcd.ps1",
	"powershell": "jcd.ps1",
	"nu":         "jcd.nu",
	"nushell":    "jcd.nu",
}

// Supported 返回被接受的 shell 名（已排序）。
func Supported() []string {
	out := make([]string, 0, len(fileFor))
	for k := range fileFor {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Script 返回某个 shell 的集成脚本，并把 __JCD_<KEY>__ 占位符替换为 vars[KEY]。
func Script(name string, vars map[string]string) (string, error) {
	file, ok := fileFor[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", fmt.Errorf("shell: 不支持的 shell %q，可选：%s", name, strings.Join(Supported(), ", "))
	}

	b, err := files.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("shell: 读取内嵌脚本 %s: %w", file, err)
	}

	out := string(b)
	for k, v := range vars {
		out = strings.ReplaceAll(out, "__JCD_"+k+"__", v)
	}
	return out, nil
}
