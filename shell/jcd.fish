# jump-cd fish 集成 —— 由 jcd init fish 生成，请勿手工编辑。
#
# 铁律一：子进程改不了父 shell 的 cwd，所以 cd 必须由下面的函数执行。
# 铁律二：热路径只用 fish 内建，不启动任何进程。
#         fish 没有 EPOCHSECONDS，因此写 0，折叠时退回当前时间。

set -g __jcd_journal "__JCD_JOURNAL__"

function __jcd_hook --on-variable PWD --description 'jump-cd: 记录目录变化'
    printf '%s	%s
' 0 "$PWD" >> "$__jcd_journal" 2>/dev/null
end

__jcd_hook

function jcd --description 'jump-cd: 目录跳转'
    set -l __jcd_first "$argv[1]"
    switch "$__jcd_first"
        case add init query list version help --help -h --version -V '__*'
            command jcd $argv
        case '*'
            set -l __jcd_path (command jcd query $argv)
            if test $status -eq 0; and test -n "$__jcd_path"
                cd -- "$__jcd_path"
            end
    end
end
