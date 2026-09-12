# 由 scripts/e2e.sh 调用；环境里已有 JCD_BIN / JCD_DATA_DIR / WORK / JCD_SHELL。
#
# 验证单元测试永远碰不到的东西：shell 函数真的改变了当前 shell 的目录。
#
# 注意这里不能写 set -u：在 fish 里 -u 是「删除变量」，不是 nounset。

"$JCD_BIN" init "$JCD_SHELL" | source

set -l target "$WORK/targets/alpha-project"

# 第 1 步：用一个路径进入从没去过的目录。
jcd "$target"
if test "$PWD" != "$target"
    echo "FAIL($JCD_SHELL): 用路径没能进去，PWD=$PWD" >&2
    exit 1
end

# 换到别处，把刚才那次访问留在历史里。
# fish 的 --on-variable PWD 在非交互式下也会触发，不需要手动补一次 hook。
cd "$WORK/targets"

if not test -s "$JCD_DATA_DIR/journal"
    echo "FAIL($JCD_SHELL): 热路径没有写日志" >&2
    exit 1
end

# 第 2 步：用名字跳回去。这是整件事的意义所在。
jcd alpha
if test "$PWD" != "$target"
    echo "FAIL($JCD_SHELL): 用名字没能跳回去，PWD=$PWD" >&2
    exit 1
end

# 子命令必须原样透传给二进制，不能被当成关键词。
jcd version > /dev/null; or begin
    echo "FAIL($JCD_SHELL): 子命令没有透传" >&2
    exit 1
end

echo "ok $JCD_SHELL"
