# jump-cd zsh 集成 —— 由 jcd init zsh 生成，请勿手工编辑。
#
# 铁律一：子进程改不了父 shell 的 cwd，所以 cd 必须由下面的函数执行。
# 铁律二：热路径不启动任何进程。每个提示符都 spawn 一个二进制，在 Windows 上
#         要白付约 14ms（实测 15.9ms 是环境底线）；这里用 shell 内建 printf +
#         追加重定向，代价是微秒级。二进制只在真正敲 jcd 时才启动。

__jcd_journal="__JCD_JOURNAL__"

__jcd_hook() {
  printf '%s	%s
' "${EPOCHSECONDS:-0}" "$PWD" >> "$__jcd_journal" 2>/dev/null || true
}

if (( $+functions[add-zsh-hook] )); then
  add-zsh-hook chpwd __jcd_hook
fi
__jcd_hook

jcd() {
  case "${1-}" in
    add|init|query|list|version|help|--help|-h|--version|-V|__*)
      command jcd "$@"
      ;;
    -)
      # 回上一个目录。shell 自己就能做，不必启动二进制。
      builtin cd - > /dev/null
      ;;
    *)
      local __jcd_path
      __jcd_path="$(command jcd query "$@")" || return $?
      if [[ -n "$__jcd_path" ]]; then
        builtin cd -- "$__jcd_path"
      fi
      ;;
  esac
}
