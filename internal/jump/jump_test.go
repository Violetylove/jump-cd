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

func TestKeywordsNormalisation(t *testing.T) {
	got := Keywords([]string{"  Jump ", "", "CD"})
	if len(got) != 2 || got[0] != "jump" || got[1] != "cd" {
		t.Fatalf("Keywords = %v", got)
	}
}
