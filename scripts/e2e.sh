#!/usr/bin/env bash
# 端到端：在真实 shell 里验证 shell 集成。
#
# 单元测试证明不了「jcd 能改变当前目录」——那需要真的有一个 shell。
#
# Windows/git-bash 下有三个坑，这里都得处理：
#   1. shell 用 MSYS 路径（/tmp/x），jcd 是原生程序只认 Windows 路径；
#   2. PATH 里必须放 MSYS 形式，否则 bash 找不到二进制；
#   3. jcd 这个 shell 函数用 command jcd 找二进制，所以它必须在 PATH 上；
#      而 PowerShell 只认带 .exe 后缀的名字。
set -u

repo=$(cd "$(dirname "$0")/.." && pwd)

norm() {
  ( cd "$1" && { pwd -W 2>/dev/null || pwd; } )
}

# 刻意不用 mktemp：它落在 /tmp 下（CI 上 TMPDIR 未设置时必然如此），
# 而 /tmp/ 正在默认忽略名单里 —— 于是 e2e 记录的每一条都会被过滤掉，
# 测试自己把自己绊倒。放到 HOME 下面就没有这个冲突。
work_msys="$HOME/.jcd-e2e.$$"
rm -rf "$work_msys"
mkdir -p "$work_msys"
work=$(norm "$work_msys")
trap 'rm -rf "$work_msys"' EXIT

bindir="$work_msys/bin"       # MSYS 形式：给 PATH 与 bash 自己用
bindir_native="$work/bin"     # 原生形式：给编译器与 exec 用

mkdir -p "$bindir" "$work_msys/data" "$work_msys/conf" \
         "$work_msys/targets/alpha-project" "$work_msys/targets/beta-project"

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) binname="jcd.exe" ;;
  *)                    binname="jcd" ;;
esac

export PATH="$bindir:$PATH"
export JCD_DATA_DIR="$work/data"
export JCD_CONFIG_DIR="$work/conf"
export WORK="$work"

JCD_BIN="$bindir_native/$binname"
if ! ( cd "$repo" && go build -o "$JCD_BIN" ./cmd/jcd ); then
  echo "构建失败" >&2
  exit 1
fi
export JCD_BIN

status=0
run_shell() {
  shell="$1"
  script="$2"
  if ! command -v "$shell" > /dev/null 2>&1; then
    echo "skip $shell（未安装）"
    return 0
  fi

  # PowerShell 不认 MSYS 形式的路径（/c/...），得换成原生形式。
  case "$shell" in
    pwsh|powershell)
      script="$(norm "$(dirname "$script")")/$(basename "$script")"
      ;;
  esac

  if JCD_SHELL="$shell" "$shell" "$script"; then
    return 0
  fi
  echo "FAIL: $shell" >&2
  status=1
}

run_shell bash       "$repo/scripts/e2e-inner.bash"
run_shell zsh        "$repo/scripts/e2e-inner.bash"
run_shell fish       "$repo/scripts/e2e-inner.bash"
run_shell pwsh       "$repo/scripts/e2e-inner.ps1"
run_shell powershell "$repo/scripts/e2e-inner.ps1"
run_shell nu         "$repo/scripts/e2e-inner.bash"

exit $status
