# AGENTS.md — jump-cd 开发指南

> 写给在本仓库工作的 AI agent 与人类贡献者。**动手前先读完「硬性约束」。**

## 这是什么

`jcd` 是一个只干一件事的小工具：**记住你去过哪些目录，然后让你用几个字跳回去。**

```bash
jcd ~/parent-dir/child-dir/popular   # 第一次：用路径进去，顺便记住
cd /somewhere/else
jcd popular                          # 之后：用名字直接跳回去
```

不做别的。没有模糊匹配引擎、没有导入导出、没有交互式选择器 —— 那些都在
「以后可能加」的清单上，而不是「现在该有」。加功能前先问一句：它服务于上面那条主线吗？

## 命令

```bash
go build ./cmd/jcd          # 构建
go test ./...               # 全量测试
go vet ./...                # 静态检查
gofmt -l .                  # 格式检查
bash scripts/e2e.sh         # 端到端：真实 shell 里验证真的改变了目录
CGO_ENABLED=0 GOOS=windows go build ./cmd/jcd   # 交叉编译必须始终可用
```

## 目录结构

| 路径 | 职责 |
|---|---|
| `cmd/jcd/` | `main`，只做依赖装配 |
| `internal/cli/` | 命令分发、输入输出、日志折叠 |
| `internal/jump/` | 匹配 + 排序 + 决策（**纯函数**） |
| `internal/store/` | JSON 文件存储（文件锁 + 原子改名） |
| `internal/journal/` | shell 直接追加的访问日志 |
| `internal/pathutil/` | 所有平台路径差异的收敛点 |
| `internal/config/` | 环境变量与数据目录 |
| `shell/` | 五套 shell 集成脚本 + `go:embed` |
| `scripts/` | 端到端测试与安装脚本 |
| `packaging/scoop/` | Scoop manifest，拷进 bucket 仓库即可 |

## 安装脚本

`scripts/install.sh` 与 `scripts/install.ps1` 负责下载、校验、装到用户级默认位置，
并顺手把 shell 集成写进启动文件。

两条硬性约束：

- **`install.sh` 必须是 POSIX sh 且输出纯 ASCII**。它跑在用户碰巧拥有的任何 shell 里 ——
  macOS 至今自带 bash 3.2，最小容器里可能只有 dash。
- **`install.ps1` 必须纯 ASCII，且不能有 `param()` 块**。前者是因为 5.1 用系统
  代码页读脚本文件，后者是因为文档里的用法是 `irm ... | iex`，param 块在那个形式下
  不可靠。配置一律走环境变量。

改完请手动验一遍：造一个假的发布包（tar.gz/zip + checksums.txt），用一个 MSYS 感知的
假 `curl` 或本地 HTTP 服务喂给它，检查装到哪、启动文件写对没有、重复跑是否幂等、
校验和不匹配时是否拒绝。**在 Windows 上直接跑 `install.sh` 会因为平台检测提前退出**，
这是对的，别为此加特例。

## 硬性约束（违反 = 打回）

1. **stdout 是协议通道。** 成功时 stdout 里只有一行目标路径 —— shell 函数拿它的内容去 `cd`。
   提示、错误、候选列表一律走 stderr。污染 stdout 会让用户 `cd` 到一个乱七八糟的目录。
2. **不要试图改父 shell 的 cwd。** 子进程做不到，任何语言都一样。
   二进制只负责算并打印路径，`cd` 由 `jcd init` 生成的 shell 函数执行。
3. **热路径不启动任何进程。** shell hook 只用内建 `printf` + 追加重定向写一行日志，
   二进制只在你真正敲 `jcd` 时才启动。实测 Windows 原生 PowerShell 下启动一个进程的
   底线是 15.9ms —— 每个提示符都付这笔钱是无法接受的。
4. **零外部依赖。** 只准用标准库。`go.mod` 里不该出现 `require`。
   这不是洁癖：它意味着 `go build` 不需要联网，也意味着二进制里没有别人的代码。
5. **匹配与排序是纯函数。** `internal/jump` 不读磁盘、不读环境变量、不自行取时间
   （`now` 从参数传入）。测试必须能离线、确定性地跑。
6. **库代码不调用 `os.Exit`**，只返回 error。
7. **平台差异集中隔离。** 业务代码只用规范化后的路径；差异全部收敛在 `internal/pathutil`。
8. **数据文件是给人看的。** `dirs.json` 要带缩进、键名可读、损坏时备份成 `.bad` 而不是静默丢弃。

## 编码约定

- Go 1.25+；`gofmt` 与 `go vet` 必须干净。
- 错误用 `fmt.Errorf("...: %w", err)` 包装。
- 表驱动测试，每个用例带描述。测试里不要碰真实 HOME，用 `t.TempDir()` + `t.Setenv`。
- **文档注释写中文，标识符写英文。** 与用户交流用简体中文。
- 提交信息用 Conventional Commits。

## 陷阱清单

- **Windows + MSYS 路径**：git-bash 的 `$PWD` 是 `/c/Users/x` 甚至 `/tmp/x`，
  而 `jcd` 是原生程序。全部经 `pathutil.Normalize`，`msysToWindows` 负责转换；
  `/usr`、`/home` 这类 MSYS 内部伪路径会明确报错而不是猜盘符。
- **bash 的 `PROMPT_COMMAND` 只在交互式 shell 里执行**。写 e2e 时要手动触发一次。
  它必须写成 `${PROMPT_COMMAND:-}`，否则在 `set -u` 的 rc 里会让 shell 起不来。
- **Windows 文件锁**：`O_CREATE|O_EXCL` 在别人正创建或删除同名文件时返回的是
  「拒绝访问」而不是「已存在」。两者都要当作「先等等」重试。
- **锁必须能接管陈旧文件**：持有者崩溃留下的锁若不处理，工具会永久卡死。
- **PowerShell 脚本必须纯 ASCII**。Windows PowerShell 5.1 用控制台代码页解码
  外部程序的输出，UTF-8 注释会被解错，解出来的字符可能破坏语法 —— 表现出来就是
  「集成没定义出 jcd 函数」。`shell/embed_test.go` 里有守卫测试。
- **PowerShell 不认 MSYS 路径**。`scripts/e2e.sh` 里传给 pwsh 的脚本路径要先转成原生形式。
- **PowerShell 拿到的是字符串数组**。`& jcd init powershell` 的输出是多行，
  要 `-join [char]10` 拼回一整段才能喂给 `Invoke-Expression`。
- **macOS 自带的是 bash 3.2**。变量名后面紧跟非 ASCII 字符时（哪怕只是中文括号），
  它会把那个字符的字节当成变量名的一部分，配合 `set -u` 直接致命；而 Windows 和
  Linux 的 bash 5 完全不报错。规矩：变量名一律加花括号。`shell/embed_test.go` 有守卫测试。
- **测试别碰真实数据目录**：`JCD_*` 环境变量必须指向 `t.TempDir()`。

## 实测数据（做性能决定前请先看这里）

Windows 11 / 原生 PowerShell，30 次均值：

| 被测对象 | 每次耗时 |
|---|---|
| `cmd.exe /c exit`（环境底线） | 15.9 ms |
| 纯 Go 二进制（1.8 MB） | 13.7 ms |
| 仅 `import _ "modernc.org/sqlite"` | 25.5 ms |

结论：进程启动在 Windows 上本来就贵，所以「每个提示符跑一次二进制」的方案不成立。
这就是硬性约束 3 的由来，也是这个项目从 SQLite 退回 JSON 的直接原因。
