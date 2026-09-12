# jump-cd PowerShell 集成 —— 由 jcd init powershell 生成，请勿手工编辑。
#
# 铁律一：子进程改不了父 shell 的 cwd，所以 Set-Location 必须由下面的函数执行。
# 铁律二：热路径只用 Add-Content 这个 cmdlet，不启动任何进程。
#         用 [char]9 拼制表符，是为了避免脚本里出现反引号转义。

$Global:__jcd_journal = '__JCD_JOURNAL__'

function global:__jcd_hook {
    if (-not $Global:__jcd_journal) { return }
    try {
        $ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
        Add-Content -LiteralPath $Global:__jcd_journal -Value ($ts.ToString() + [char]9 + $PWD.Path) -ErrorAction SilentlyContinue
    } catch {
    }
}

if (-not $Global:__jcd_original_prompt) {
    $Global:__jcd_original_prompt = $function:prompt
}

function global:prompt {
    __jcd_hook
    if ($Global:__jcd_original_prompt) {
        & $Global:__jcd_original_prompt
    } else {
        "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
    }
}

function global:jcd {
    $reserved = @('add', 'init', 'query', 'list', 'version', 'help',
                  '--help', '-h', '--version', '-V')
    if ($args.Count -gt 0) {
        $first = [string]$args[0]
        if (($reserved -contains $first) -or $first.StartsWith('__')) {
            & jcd.exe @args
            return
        }
    }
    $target = & jcd.exe query @args
    if ($LASTEXITCODE -ne 0 -or -not $target) {
        return
    }
    Set-Location -LiteralPath $target
}
