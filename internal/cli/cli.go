// Package cli 是 jcd 的命令行界面。
//
// 刻意不用 CLI 框架：这里只有五个子命令和几个开关，标准库的 flag 就够了。
// 少一个依赖，二进制也小一截。
//
// 输出约定：stdout 是协议通道，成功时只有一行目标路径 —— shell 函数拿它的内容去 cd。
// 其余一切（提示、错误、候选列表）都走 stderr。
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Violetylove/jump-cd/internal/config"
	"github.com/Violetylove/jump-cd/internal/journal"
	"github.com/Violetylove/jump-cd/internal/jump"
	"github.com/Violetylove/jump-cd/internal/pathutil"
	"github.com/Violetylove/jump-cd/internal/store"
	"github.com/Violetylove/jump-cd/internal/version"
	"github.com/Violetylove/jump-cd/shell"
)

// 进程退出码。
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

// listLimit 是 -l 最多列出的候选数。
const listLimit = 50

var usageLines = []string{
	"jcd —— 记住你去过的目录",
	"",
	"用法:",
	"  jcd                   回到用户主目录",
	"  jcd <路径>            进入这个目录，并记住它",
	"  jcd <关键词...>       跳回之前去过的匹配目录",
	"",
	"查询开关:",
	"  -l                    列出候选，不跳转",
	"  -f                    有多个候选时直接取最高分（默认不猜，交给你）",
	"",
	"其它子命令:",
	"  jcd list              列出记住的全部目录（-prune 顺便清理失效的）",
	"  jcd add [路径]        只记录不跳转（shell hook 用，平时不用管）",
	"  jcd init <shell>      输出 shell 集成脚本",
	"  jcd version           版本",
	"",
	"环境变量:",
	"  JCD_DATA_DIR          数据目录",
	"  JCD_HALF_LIFE_DAYS    记忆半衰期，默认 7 天",
	"  JCD_AMBIGUOUS_TAU     歧义阈值，默认 1.25",
}

// Run 执行一次命令行调用，返回进程退出码。
func Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return cmdQuery(ctx, nil)
	}

	switch args[0] {
	case "query":
		return cmdQuery(ctx, args[1:])
	case "add":
		return cmdAdd(ctx, args[1:])
	case "list":
		return cmdList(ctx, args[1:])
	case "init":
		return cmdInit(args[1:])
	case "version", "--version", "-V":
		fmt.Printf("jcd %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
		return exitOK
	case "help", "--help", "-h":
		fmt.Println(strings.Join(usageLines, "\n"))
		return exitOK
	default:
		// 不是已知子命令，就当成查询的一部分：
		// 直接跑二进制时 jcd foo 与 jcd query foo 应该是一回事。
		return cmdQuery(ctx, args)
	}
}

// ------------------------------------------------------------------ query

func cmdQuery(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	list := fs.Bool("l", false, "列出候选")
	force := fs.Bool("f", false, "直接取最高分")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()

	cfg := config.Load()
	now := time.Now().Unix()

	// 直接路径优先，而且刻意不碰历史文件：
	// jcd ~/a/b/c 的语义就是 cd ~/a/b/c，不该依赖历史是否完好。
	// 不带参数时它返回主目录，所以 jcd 单独一个就是回家。
	if !*list {
		if p, ok := directPath(rest); ok {
			return emitPath(p)
		}
	}

	keywords := jump.Keywords(rest)
	if len(keywords) == 0 {
		fmt.Fprintln(os.Stderr, "jcd: 没给出可用的关键词")
		return exitUsage
	}

	st, err := openStore()
	if err != nil {
		return fail(err)
	}

	// 先把手敲命令期间的访问日志折进历史，否则刚去过的目录还看不到。
	if err := foldJournal(st, now); err != nil {
		fmt.Fprintln(os.Stderr, "jcd: 折叠访问日志失败:", err)
	}

	data, warn := st.Load()
	if warn != nil {
		fmt.Fprintln(os.Stderr, "jcd:", warn)
	}

	cands := dropMissing(st, jump.Search(data, keywords, now, cfg.HalfLifeDays))

	if *list {
		printCandidates(os.Stdout, cands)
		return exitOK
	}

	switch d := jump.Decide(cands, cfg.AmbiguousTau, 10); d.Outcome {
	case jump.OutcomeNone:
		fmt.Fprintf(os.Stderr, "jcd: 没有记住过匹配 %q 的目录\n", strings.Join(keywords, " "))
		return exitFail
	case jump.OutcomeUnique:
		return emitPath(d.Best.Path)
	default:
		if *force {
			return emitPath(d.Best.Path)
		}
		// 猜错比让你再打两个字贵得多，所以这里不猜。
		fmt.Fprintf(os.Stderr, "jcd: 有 %d 个候选不相上下，多给它一点信息，或用 -l 看全部（-f 直接取第一个）:\n", len(d.Top))
		printCandidates(os.Stderr, d.Top)
		return exitFail
	}
}

// directPath 判断这次调用是不是「直接走进一个真实目录」。
//
// 不带参数：回家，跟 cd 一样。
// 只有一个参数而且它确实是个存在的目录：直接用它。
//
// 两个都不是，才退回去查访问历史。代价是「当前目录下正好有个同名目录」时，
// 直接路径会压过历史记录 —— 这与 cd 的直觉一致，是刻意的。
func directPath(args []string) (string, bool) {
	if len(args) == 0 {
		home, err := pathutil.Home()
		if err != nil {
			return "", false
		}
		return home, true
	}
	if len(args) != 1 {
		return "", false
	}

	arg := strings.TrimSpace(args[0])
	if arg == "" || strings.HasPrefix(arg, "-") {
		return "", false
	}

	p, err := pathutil.Normalize(arg)
	if err != nil {
		return "", false
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	return p, true
}

// dropMissing 丢掉已经不存在的目录，并顺手把它们从历史里删掉。
// 只检查候选，不做全量扫描 —— 全量 stat 太慢，不该出现在查询路径上。
func dropMissing(st *store.Store, cands []jump.Candidate) []jump.Candidate {
	kept := make([]jump.Candidate, 0, len(cands))
	var gone []string

	for _, c := range cands {
		if fi, err := os.Stat(c.Path); err == nil && fi.IsDir() {
			kept = append(kept, c)
			continue
		}
		gone = append(gone, c.Path)
	}

	if len(gone) > 0 {
		_ = st.Update(func(d *store.Data) error {
			for _, p := range gone {
				delete(d.Dirs, p)
			}
			return nil
		})
	}
	return kept
}

func printCandidates(w *os.File, cands []jump.Candidate) {
	limit := len(cands)
	if limit > listLimit {
		limit = listLimit
	}
	for i := 0; i < limit; i++ {
		fmt.Fprintf(w, "  %2d) %8.1f  %s\n", i+1, cands[i].Score, cands[i].Path)
	}
	if len(cands) > limit {
		fmt.Fprintf(w, "  ... 还有 %d 条\n", len(cands)-limit)
	}
}

// ------------------------------------------------------------------ add

func cmdAdd(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	quiet := fs.Bool("quiet", false, "不输出")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	target := ""
	if rest := fs.Args(); len(rest) > 0 {
		target = rest[0]
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return fail(err)
		}
		target = wd
	}

	p, err := pathutil.Normalize(target)
	if err != nil {
		return exitFail
	}

	// 目录不存在就当作什么也没发生：shell 启动时的过期 PWD 不该报错。
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		return exitOK
	}

	// 只写日志，不碰历史文件 —— 少一次加锁，也少一个写入路径。
	jp, err := config.JournalPath()
	if err != nil {
		return fail(err)
	}
	if err := journal.Open(jp).Append(p, time.Now().Unix()); err != nil {
		return fail(err)
	}
	if !*quiet {
		fmt.Fprintln(os.Stderr, p)
	}
	return exitOK
}

// ------------------------------------------------------------------ list

func cmdList(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	prune := fs.Bool("prune", false, "顺便删掉已经不存在的目录")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	st, err := openStore()
	if err != nil {
		return fail(err)
	}
	now := time.Now().Unix()
	if err := foldJournal(st, now); err != nil {
		fmt.Fprintln(os.Stderr, "jcd: 折叠访问日志失败:", err)
	}

	if *prune {
		if err := st.Update(func(d *store.Data) error {
			removed := d.Prune(isDir)
			fmt.Fprintf(os.Stderr, "清理了 %d 条失效记录\n", removed)
			return nil
		}); err != nil {
			return fail(err)
		}
	}

	data, warn := st.Load()
	if warn != nil {
		fmt.Fprintln(os.Stderr, "jcd:", warn)
	}

	fmt.Printf("数据文件  %s\n", st.Path())
	fmt.Printf("记住 %d 个目录，累计 %d 次访问\n\n", data.Len(), data.TotalVisits())

	for _, p := range data.Paths() {
		e := data.Dirs[p]
		fmt.Printf("  %3d 次  %-12s  %s\n", e.Visits, humanAge(e.Last, now), p)
	}
	return exitOK
}

// ------------------------------------------------------------------ init

func cmdInit(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "jcd: 需要指定 shell，可选：%s\n", strings.Join(shell.Supported(), ", "))
		return exitUsage
	}

	jp, err := config.JournalPath()
	if err != nil {
		return fail(err)
	}
	// shell 侧的追加重定向不会创建目录，init 顺手建好。
	if err := os.MkdirAll(filepath.Dir(jp), 0o755); err != nil {
		return fail(err)
	}

	// 脚本里的路径一律用正斜杠：这几种 shell 在 Windows 上都能接受，
	// 也免去反斜杠在各家引号规则下的转义差异。
	script, err := shell.Script(args[0], map[string]string{
		"JOURNAL": filepath.ToSlash(jp),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "jcd:", err)
		return exitUsage
	}
	fmt.Print(script)
	return exitOK
}

// ------------------------------------------------------------------ 公共

// emitPath 把目标写到 stdout。成功时 stdout 里只有这一行。
func emitPath(p string) int {
	if err := pathutil.ValidateOutput(p); err != nil {
		fmt.Fprintln(os.Stderr, "jcd:", err)
		return exitFail
	}
	fmt.Println(p)
	return exitOK
}

func openStore() (*store.Store, error) {
	path, err := config.StorePath()
	if err != nil {
		return nil, err
	}
	return store.Open(path), nil
}

// foldJournal 把 shell 直接追加的访问日志折进历史。
//
// 日志是「先追加、后折叠」：hook 只写一行，昂贵的部分推迟到你敲 jcd 的时候做。
func foldJournal(st *store.Store, now int64) error {
	jp, err := config.JournalPath()
	if err != nil {
		return err
	}
	j := journal.Open(jp)

	entries, err := j.Drain()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return j.Done()
	}

	if err := st.Update(func(d *store.Data) error {
		for _, e := range entries {
			p, nErr := pathutil.Normalize(e.Path)
			if nErr != nil {
				continue
			}
			t := e.Time
			if t <= 0 {
				t = now
			}
			d.Touch(p, t)
		}
		return nil
	}); err != nil {
		return err
	}
	return j.Done()
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "jcd:", err)
	return exitFail
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// humanAge 返回「多久以前」。0 表示从未访问过（不会出现，但别显示成 1970 年）。
func humanAge(last, now int64) string {
	if last <= 0 {
		return "从未"
	}
	d := now - last
	switch {
	case d < 60:
		return "刚刚"
	case d < 3600:
		return fmt.Sprintf("%d 分钟前", d/60)
	case d < 86400:
		return fmt.Sprintf("%d 小时前", d/3600)
	case d < 30*86400:
		return fmt.Sprintf("%d 天前", d/86400)
	default:
		return fmt.Sprintf("%d 个月前", d/(30*86400))
	}
}
