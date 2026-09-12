# jump-cd

> 记住你去过哪些目录，让你用几个字跳回去。

```bash
jcd ~/parent-dir/child-dir/popular   # 第一次：用路径进去，顺便记住
cd /somewhere/else
jcd popular                          # 之后：用名字直接跳回去
```

一个文件、一个二进制、零外部依赖。Windows / macOS / Linux 都能跑。

## 安装

```bash
git clone https://github.com/Violetylove/jump-cd
cd jump-cd
go build -o jcd ./cmd/jcd
# 把 jcd 放到 PATH 上（shell 函数靠名字找它）
```

编译只用到标准库，**不需要联网**。

## 启用

`jcd` 是「函数 + 二进制」的组合：二进制负责算出路径，函数负责在你当前这个 shell 里 `cd`。
子进程改不了父 shell 的目录，这是操作系统的限制，任何语言都一样。

```bash
# zsh
echo 'eval "$(jcd init zsh)"' >> ~/.zshrc

# bash
echo 'eval "$(jcd init bash)"' >> ~/.bashrc

# fish
jcd init fish | source

# PowerShell
Add-Content $PROFILE 'Invoke-Expression (&jcd init powershell)'

# nushell
jcd init nushell | save -f ~/.jcd.nu
```

想要更短的名字：`alias j=jcd`。

五套集成的验证程度不一样：bash、zsh、fish、PowerShell 都在 CI 上跑过真实的
「进去再跳回来」，nushell 的脚本提供了但**没验证过**（开发机与 CI 都没装）。

## 用法

```
jcd                    回到用户主目录
jcd <路径>             进入这个目录，并记住它
jcd <关键词...>        跳回之前去过的匹配目录
jcd -                  回到上一个目录（跟 cd - 一样）
jcd -l <关键词>        列出候选，不跳转
jcd -f <关键词>        有多个候选时直接取最高分（默认让你选）
jcd list               列出记住的全部目录
jcd add [路径]         只记录不跳转（shell hook 用，平时不用管）
jcd init <shell>       输出 shell 集成脚本
jcd doctor             体检：PATH、数据文件、shell 集成有没有接对
```

候选多于一个时会列出编号让你选，直接回车就取消。

打错一个字母也能找到：`jcd popluar` 一样能跳到 `popular`。

## 它怎么决定跳哪里

匹配看关键词命中了路径的哪一段、命中得多结实；排序是：

```
分数 = 匹配质量 x （1 + 去过几次） x 多久没去
```

前两名分数太接近时它**不跳**，而是把候选列出来让你再说清楚一点 ——
跳错地方比让你多打两个字贵得多。

细节在 [DESIGN.md](DESIGN.md)。

## 数据放在哪

- Linux / macOS：`~/.local/share/jump-cd/dirs.json`
- Windows：`%LOCALAPPDATA%\jump-cd\dirs.json`

就是个 JSON 文件，带缩进，可以直接看、直接改。删掉它等于清空记忆。

环境变量：`JCD_DATA_DIR`、`JCD_HALF_LIFE_DAYS`（默认 7）、`JCD_AMBIGUOUS_TAU`（默认 1.25）、
`JCD_IGNORE`（追加不记录的路径，如 `/cache/`）。

`node_modules`、`.git`、`/tmp` 这类地方默认就不会被记住 —— 进去过不等于想再回来。

## 开发

```bash
go test ./...
bash scripts/e2e.sh
```

贡献前读一眼 [AGENTS.md](AGENTS.md)。

## 许可

MIT
