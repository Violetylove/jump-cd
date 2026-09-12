# jump-cd nushell 集成 —— 由 jcd init nushell 生成，请勿手工编辑。
#
# 铁律一：子进程改不了父 shell 的 cwd，所以 cd 必须由 --env 函数执行。
# 铁律二：热路径只用 nushell 内建 save --append，不启动任何进程。
#
# 注意：本文件尚未在真实 nushell 上验证过（开发机没装），语法按 0.9x 编写。

$env.JCD_JOURNAL = "__JCD_JOURNAL__"

def __jcd_hook [] {
    $env.PWD | save --append $env.JCD_JOURNAL
}

$env.config = ($env.config | upsert hooks.env_change.PWD (
    ($env.config?.hooks?.env_change?.PWD? | default []) | append {|_, after|
        if $after != null { __jcd_hook }
    }
))

def --env jcd [...rest: string] {
    let reserved = [add init query list version help]
    let first = ($rest | first | default '')
    if ($reserved | any {|x| $x == $first }) or ($first | str starts-with '__') {
        ^jcd ...$rest
    } else {
        let target = (^jcd query ...$rest | str trim)
        if ($target | is-empty) {
            error make {msg: 'jump-cd: 没有找到匹配的目录'}
        } else {
            cd $target
        }
    }
}
