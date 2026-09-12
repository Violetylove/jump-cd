# 由 scripts/e2e.sh 调用；环境里已有 JCD_BIN / JCD_DATA_DIR / WORK / JCD_SHELL。
#
# 这里验证的是单元测试永远碰不到的东西：shell 函数真的改变了当前 shell 的目录。
set -u

eval "$("$JCD_BIN" init "$JCD_SHELL")"

# bash 的 PROMPT_COMMAND 只在交互式 shell 里跑，而这个脚本是非交互的。
# 这里手动跑一遍，等价于交互式下每次提示符前的那一次调用。
# zsh 走 chpwd，不依赖 PROMPT_COMMAND，因此这里是空操作。
jcd_prompt() {
  if [ -n "${PROMPT_COMMAND:-}" ]; then
    eval "$PROMPT_COMMAND" > /dev/null 2>&1 || true
  fi
}

# bash 的 $PWD 在 Windows 上是 MSYS 形式（/tmp/x），而 WORK 是原生形式，
# 直接比较永远不相等。统一取原生形式。
native_pwd() { pwd -W 2>/dev/null || pwd; }

target="$WORK/targets/alpha-project"

# 第 1 步：用一个路径进入从没去过的目录。
jcd "$target" || { echo "FAIL($JCD_SHELL): jcd <路径> 失败了" >&2; exit 1; }
if [ "$(native_pwd)" != "$target" ]; then
  echo "FAIL($JCD_SHELL): 用路径没能进去，PWD=$(native_pwd)" >&2
  exit 1
fi
jcd_prompt

# 换到别处，把刚才那次访问留在历史里。
cd "$WORK/targets" || exit 1
jcd_prompt

# 热路径必须已经写进日志（shell 内建追加，零进程）。
if [ ! -s "$JCD_DATA_DIR/journal" ]; then
  echo "FAIL($JCD_SHELL): 热路径没有写日志" >&2
  exit 1
fi

# 第 2 步：用名字跳回去。这是整件事的意义所在。
jcd alpha || { echo "FAIL($JCD_SHELL): jcd <名字> 失败了" >&2; exit 1; }
if [ "$(native_pwd)" != "$target" ]; then
  echo "FAIL($JCD_SHELL): 用名字没能跳回去，PWD=$(native_pwd)" >&2
  exit 1
fi

# 子命令必须原样透传给二进制，不能被当成关键词。
jcd version > /dev/null || { echo "FAIL($JCD_SHELL): 子命令没有透传" >&2; exit 1; }

echo "ok $JCD_SHELL"
