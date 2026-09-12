package jump

import (
	"testing"

	"github.com/Violetylove/jump-cd/internal/store"
)

const now = int64(1_700_000_000)

func qualityOf(path string, keywords ...string) float64 {
	return matchQuality(path, keywords)
}

func TestMatchTiers(t *testing.T) {
	tests := []struct {
		name string
		path string
		kw   string
		want float64
	}{
		{"末段全等", "/home/u/code/jump-cd", "jump-cd", qualityBaseExact},
		{"末段前缀", "/home/u/code/jump-cd", "jump", qualityBasePrefix},
		{"末段子串", "/home/u/code/jump-cd", "ump", qualityBaseContain},
		{"中间段全等", "/home/u/code/jump-cd", "code", qualitySegExact},
		{"中间段前缀", "/home/u/code/jump-cd", "cod", qualitySegPrefix},
		{"中间段子串", "/home/u/code/jump-cd", "ode", qualitySegContain},
		{"完全不沾边", "/home/u/code/jump-cd", "zzz", qualityNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualityOf(tt.path, tt.kw); got != tt.want {
				t.Fatalf("quality = %v want %v", got, tt.want)
			}
		})
	}
}

func TestMatchIsCaseInsensitive(t *testing.T) {
	if got := qualityOf("/home/u/CodeBase", "codebase"); got != qualityBaseExact {
		t.Fatalf("quality = %v", got)
	}
}

// 多个关键词取最弱一环。
func TestWeakestLinkWins(t *testing.T) {
	if got := qualityOf("/foo/bar", "foo", "bar"); got != qualitySegExact {
		t.Fatalf("quality = %v，应当按最弱的那个（中间段全等）算", got)
	}
	if got := qualityOf("/foo/bar", "foo", "zzz"); got != qualityNone {
		t.Fatalf("有一个词没命中，整体就该不匹配，实际 %v", got)
	}
}

func TestSearchPrefersTheExactBasename(t *testing.T) {
	d := store.NewData()
	d.Touch("/home/u/code/jump-cd", now)
	d.Touch("/home/u/old/jump-cd-legacy", now)

	// jump-cd-legacy 以 jump-cd 开头，本来就算候选；但它只是前缀命中，
	// 必须排在末段全等的那个后面。
	got := Search(d, []string{"jump-cd"}, now, 7)
	if len(got) != 2 {
		t.Fatalf("应当有 2 条候选，实际 %d 条：%+v", len(got), got)
	}
	if got[0].Path != "/home/u/code/jump-cd" {
		t.Fatalf("top = %q，末段全等的应当排第一", got[0].Path)
	}
	if got[0].Quality <= got[1].Quality {
		t.Fatalf("质量分没有拉开：%v vs %v", got[0].Quality, got[1].Quality)
	}
}

// 分数接近时必须承认「分不清」，而不是猜一个。
func TestDecideRefusesWhenTooClose(t *testing.T) {
	closeCands := []Candidate{
		{Path: "/a", Score: 100},
		{Path: "/b", Score: 95},
	}
	if got := Decide(closeCands, 1.25, 10); got.Outcome != OutcomeAmbiguous {
		t.Fatalf("比值 %.2f 应当判为歧义，实际 %v", got.Ratio, got.Outcome)
	}

	clearCands := []Candidate{
		{Path: "/a", Score: 100},
		{Path: "/b", Score: 30},
	}
	if got := Decide(clearCands, 1.25, 10); got.Outcome != OutcomeUnique {
		t.Fatalf("比值 %.2f 应当判为确定，实际 %v", got.Ratio, got.Outcome)
	}
}

func TestDecideNoneAndSingle(t *testing.T) {
	if got := Decide(nil, 1.25, 10); got.Outcome != OutcomeNone {
		t.Fatalf("空候选 = %v", got.Outcome)
	}
	one := []Candidate{{Path: "/only", Score: 1}}
	if got := Decide(one, 1.25, 10); got.Outcome != OutcomeUnique {
		t.Fatalf("单候选 = %v", got.Outcome)
	}
}

// 越久没去分数越低；去过越多次分数越高。
func TestScoreUsesRecencyAndVisits(t *testing.T) {
	fresh := store.Entry{Visits: 1, Last: now}
	stale := store.Entry{Visits: 1, Last: now - 30*86400}
	if score(100, fresh, now, 7) <= score(100, stale, now, 7) {
		t.Fatal("最近去过的应当更高")
	}

	often := store.Entry{Visits: 10, Last: now}
	once := store.Entry{Visits: 1, Last: now}
	if score(100, often, now, 7) <= score(100, once, now, 7) {
		t.Fatal("去过更多次的应当更高")
	}
}

// 结果必须可复现：map 的遍历顺序在 Go 里是随机的。
func TestSearchIsDeterministic(t *testing.T) {
	d := store.NewData()
	d.Touch("/a/x", now)
	d.Touch("/b/x", now)
	d.Touch("/c/x", now)

	first := Search(d, []string{"x"}, now, 7)
	for i := 0; i < 20; i++ {
		got := Search(d, []string{"x"}, now, 7)
		for j := range first {
			if got[j].Path != first[j].Path {
				t.Fatalf("第 %d 次顺序不同：%v vs %v", i, got, first)
			}
		}
	}
}

// 打错一个字母也要找得到：替换、增、删、相邻写反。
func TestFuzzyToleratesOneTypo(t *testing.T) {
	tests := []struct {
		name string
		kw   string
	}{
		{"相邻写反", "popluar"},
		{"少一个字母", "populr"},
		{"多一个字母", "populare"},
		{"写错一个字母", "populat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualityOf("/home/u/code/popular", tt.kw); got != qualityBaseFuzzy {
				t.Fatalf("quality(%q) = %v want %v", tt.kw, got, qualityBaseFuzzy)
			}
		})
	}
}

// 目录名比关键词长时，该拿来比的是「同长前缀」。
func TestFuzzyComparesAgainstPrefix(t *testing.T) {
	if got := qualityOf("/home/u/code/popular-tools", "popluar"); got != qualityBaseFuzzy {
		t.Fatalf("quality = %v want %v", got, qualityBaseFuzzy)
	}
}

// 放松必须有限度：太短的词不该容错，差太远的词不该命中。
func TestFuzzyStaysStrictWhenItShould(t *testing.T) {
	tests := []struct {
		name string
		path string
		kw   string
	}{
		{"关键词太短", "/home/u/code/car", "cat"},
		{"差得太远", "/home/u/code/popular", "zzzz"},
		{"长度差太多", "/home/u/code/popular", "popularity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualityOf(tt.path, tt.kw); got != qualityNone {
				t.Fatalf("quality = %v，不该命中", got)
			}
		})
	}
}

// 容错只在末段启用：中间段放松会大面积误召回。
func TestFuzzyOnlyAppliesToBaseSegment(t *testing.T) {
	if got := qualityOf("/home/u/popluar/x", "popular"); got != qualityNone {
		t.Fatalf("中间段不该容错，实际 %v", got)
	}
}

func TestIgnored(t *testing.T) {
	patterns := []string{"/node_modules/", "/tmp/", "/.git/"}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"命中 node_modules", "/home/u/app/node_modules/lodash", true},
		{"命中 tmp", "/tmp/scratch", true},
		{"tmp 本身", "/tmp", true},
		{"命中 .git", "/home/u/app/.git", true},
		{"普通目录", "/home/u/code/jump-cd", false},
		{"只是前缀相同", "/home/u/tmpx", false},
		{"Windows 反斜杠", "C:/Users/u/app/node_modules/x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Ignored(tt.path, patterns); got != tt.want {
				t.Fatalf("Ignored(%q) = %v want %v", tt.path, got, tt.want)
			}
		})
	}

	if Ignored("/anything", nil) {
		t.Fatal("空名单不该忽略任何东西")
	}
}

// 忽略名单用的是字面量子串，不是正则 —— 免得路径里的特殊字符炸掉匹配。
func TestIgnoredIsLiteral(t *testing.T) {
	if !Ignored("/home/u/[weird]/x", []string{"[weird]"}) {
		t.Fatal("方括号应当按字面量匹配")
	}
}

func TestKeywordsNormalisation(t *testing.T) {
	got := Keywords([]string{"  Jump ", "", "CD"})
	if len(got) != 2 || got[0] != "jump" || got[1] != "cd" {
		t.Fatalf("Keywords = %v", got)
	}
}

// 回归：目录名命中不能被「访问次数很高的祖先段命中」顶下去。
//
// 曾经的排序只按分数比大小。一个被 cd 过 32 次的深层目录，靠祖先段里恰好
// 路过 winter-space 命中，拿到 40 x 21 = 840 分；而名字就叫 winter-space、
// 只去过 3 次的那条只有 80 x 4 = 320 分 —— jcd winter-spa 于是跳去了深层目录，
// 连 jcd winter-space 这种把名字打全的查询都跳不对。
func TestSearchPrefersTheNamedDirectoryOverAHotAncestor(t *testing.T) {
	d := store.NewData()
	named := "/home/u/winter-space"
	hot := "/home/u/winter-space/code-space/pwsh-hu-line"
	for i := 0; i < 3; i++ {
		d.Touch(named, now)
	}
	for i := 0; i < 32; i++ {
		d.Touch(hot, now)
	}

	got := Search(d, []string{"winter-spa"}, now, 7)
	if len(got) != 2 {
		t.Fatalf("应当有 2 条候选，实际 %d 条：%+v", len(got), got)
	}
	if got[0].Path != named {
		t.Fatalf("top = %q，名字命中的应当排第一", got[0].Path)
	}
	if !got[0].Anchored || got[1].Anchored {
		t.Fatalf("命中类别判定反了：%+v", got)
	}
	// 排序对了还不够：决策层若仍拿原始分数比值判歧义，这里会退化成「问用户」。
	if dec := Decide(got, 1.25, 10); dec.Outcome != OutcomeUnique || dec.Best.Path != named {
		t.Fatalf("decision = %+v，应当确定地跳到名字命中的那条", dec)
	}
}

// 歧义只在同类之间判：末段命中与祖先段命中不是一个意图，分数不该互相比较。
func TestDecideComparesWithinClass(t *testing.T) {
	mixed := []Candidate{
		{Path: "/named", Anchored: true, Score: 10},
		{Path: "/ancestor", Score: 1000},
	}
	if got := Decide(mixed, 1.25, 10); got.Outcome != OutcomeUnique || got.Best.Path != "/named" {
		t.Fatalf("唯一的末段命中不该被判成歧义：%+v", got)
	}

	sameClass := []Candidate{
		{Path: "/a", Anchored: true, Score: 100},
		{Path: "/b", Anchored: true, Score: 95},
		{Path: "/c", Score: 9999},
	}
	if got := Decide(sameClass, 1.25, 10); got.Outcome != OutcomeAmbiguous {
		t.Fatalf("同类中两条不相上下时仍应当判歧义，实际 %v", got.Outcome)
	}
}

// isBaseQuality 靠「末段档位全部高于中间段档位」这个不变量把质量换算成类别。
// 这里守住它，免得哪天调档位时悄悄把两类段弄反。
func TestBaseTiersAllOutrankMiddleTiers(t *testing.T) {
	base := []float64{qualityBaseContain, qualityBaseFuzzy, qualityBasePrefix, qualityBaseExact}
	middle := []float64{qualitySegContain, qualitySegPrefix, qualitySegExact}
	for _, b := range base {
		for _, m := range middle {
			if b <= m {
				t.Fatalf("末段档位 %v 不高于中间段档位 %v，isBaseQuality 的假设被打破", b, m)
			}
		}
	}
	if isBaseQuality(qualitySegExact) {
		t.Fatal("中间段全等被误判成末段命中")
	}
	if !isBaseQuality(qualityBaseContain) {
		t.Fatal("末段子串被误判成非末段命中")
	}
}
