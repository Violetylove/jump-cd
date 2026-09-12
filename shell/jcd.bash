# jump-cd bash 集成 —— 由 jcd init bash 生成，请勿手工编辑。
#
# 铁律一：子进程改不了父 shell 的 cwd，所以 cd 必须由下面的函数执行。
# 铁律二：热路径只用 bash 内建，不启动任何进程。
#         EPOCHSECONDS 需要 bash 5；取不到时写 0，折叠时退回当前时间。

__jcd_journal="__JCD_JOURNAL__"

__jcd_hook() {
  printf '%s	%s
' "${EPOCHSECONDS:-0}" "$PWD" >> "$__jcd_journal" 2>/dev/null || true
}

# 必须容得下 set -u：不少人在 rc 里开了 nounset，裸引用未定义的
# PROMPT_COMMAND 会让整个 shell 起不来。
case "${PROMPT_COMMAND:-}" in
  *__jcd_hook*) ;;
  *) PROMPT_COMMAND="__jcd_hook${PROMPT_COMMAND:+; $PROMPT_COMMAND}" ;;
esac
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
      if [ -n "$__jcd_path" ]; then
        builtin cd -- "$__jcd_path"
      fi
      ;;
  esac
}
