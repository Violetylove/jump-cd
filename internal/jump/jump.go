// Package jump 把关键词变成目标目录：匹配、排序、决策。
//
// 三件事各自独立、都是纯函数，所以可以离线测透：
//
//	匹配 —— 这个关键词命中了这条路径的哪一段、命中得有多结实
//	排序 —— 质量 x 去过几次 x 多久没去
//	决策 —— 够确定就跳，不确定就交给人
package jump

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Violetylove/jump-cd/internal/pathutil"
	"github.com/Violetylove/jump-cd/internal/store"
)

// 命中档位。数值就是匹配质量的基础分。
const (
	qualityNone        = 0.0
	qualitySegContain  = 20.0
	qualitySegPrefix   = 40.0
	qualitySegExact    = 50.0
	qualityBaseContain = 60.0
	qualityBasePrefix  = 80.0
	qualityBaseExact   = 100.0

	// qualityBaseFuzzy 是「末段只差一个字符」的档位。
	// 定得比「末段子串」(60) 高：打错一个字母的往往就是整个目录名，
	// 那比「碰巧包含这几个字母」是强得多的意图信号。
	qualityBaseFuzzy = 70.0
)

// minFuzzyRunes 是启用容错的最短关键词。
// 太短的词放松一格就会命中一大堆无关目录，得不偿失。
const minFuzzyRunes = 4

// maxVisitWeight 给「去过几次」封顶：否则一个天天去的目录会永远压住新目标。
const maxVisitWeight = 20

// Candidate 是一条候选目录及其得分。
type Candidate struct {
	Path    string
	Quality float64
	Visits  int64
	Last    int64
	Score   float64
}

// Search 在历史里找匹配的目录，按分数降序返回。
func Search(d *store.Data, keywords []string, now int64, halfLifeDays float64) []Candidate {
	out := make([]Candidate, 0, len(d.Dirs))
	for path, e := range d.Dirs {
		q := matchQuality(path, keywords)
		if q <= qualityNone {
			continue
		}
		out = append(out, Candidate{
			Path:    path,
			Quality: q,
			Visits:  e.Visits,
			Last:    e.Last,
			Score:   score(q, e, now, halfLifeDays),
		})
	}
	sortCandidates(out)
	return out
}

// matchQuality 返回关键词命中这条路径的质量（0–100）。
//
// 多个关键词时取**最弱的一环**：一个词命中最深的档，另一个词勉强沾边，
// 整体就该按勉强沾边算。
func matchQuality(path string, keywords []string) float64 {
	if len(keywords) == 0 {
		return qualityNone
	}

	segs := pathutil.Segments(path)
	if len(segs) == 0 {
		return qualityNone
	}
	last := len(segs) - 1
	base := strings.ToLower(segs[last])

	worst := qualityBaseExact
	for _, kw := range keywords {
		best := tierOf(base, kw, true)
		for i := 0; i < last; i++ {
			if t := tierOf(strings.ToLower(segs[i]), kw, false); t > best {
				best = t
			}
		}
		if best <= qualityNone {
			return qualityNone
		}
		if best < worst {
			worst = best
		}
	}
	return worst
}

// tierOf 是单段对单个关键词的命中最强档位。
func tierOf(seg, kw string, isBase bool) float64 {
	switch {
	case seg == kw:
		if isBase {
			return qualityBaseExact
		}
		return qualitySegExact
	case strings.HasPrefix(seg, kw):
		if isBase {
			return qualityBasePrefix
		}
		return qualitySegPrefix
	case strings.Contains(seg, kw):
		if isBase {
			return qualityBaseContain
		}
		return qualitySegContain
	}

	// 容错：打错一个字母（含相邻两字写反）也要找得到。
	// 只在末段启用 —— 在中间段放松会大面积误召回。
	if isBase && utf8.RuneCountInString(kw) >= minFuzzyRunes && withinOneEditAny(kw, seg) {
		return qualityBaseFuzzy
	}
	return qualityNone
}

// withinOneEditAny 报告 kw 与 seg（或 seg 的一个同长前缀）是否只差一个字符。
//
// 比前缀是为了覆盖「目录名比关键词长」的常见情形：
// 关键词 popluar 对目录 popular-tools，该拿来比的是前缀 popular。
func withinOneEditAny(kw, seg string) bool {
	if withinOneEdit(kw, seg) {
		return true
	}
	r := []rune(seg)
	n := utf8.RuneCountInString(kw)
	for _, cut := range []int{n, n + 1} {
		if cut > 0 && cut < len(r) && withinOneEdit(kw, string(r[:cut])) {
			return true
		}
	}
	return false
}

// withinOneEdit 报告 a 与 b 是否只差一个字符：替换、增、删，或相邻两字写反。
//
// 只判定「差一个」，所以不用写完整的编辑距离算法 —— 直接分类讨论更快也更好读。
func withinOneEdit(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)

	switch {
	case la == lb:
		diff, first := 0, -1
		for i := 0; i < la; i++ {
			if ar[i] != br[i] {
				diff++
				if first < 0 {
					first = i
				}
			}
		}
		switch diff {
		case 0, 1:
			return true
		case 2:
			// 相邻两字写反：popluar / popular
			return first+1 < la && ar[first] == br[first+1] && ar[first+1] == br[first]
		}
		return false

	case la+1 == lb:
		return oneDeletion(ar, br)
	case lb+1 == la:
		return oneDeletion(br, ar)
	}
	return false
}

// oneDeletion 报告 long 去掉一个字符后是否等于 short。
func oneDeletion(short, long []rune) bool {
	i, j, skipped := 0, 0, false
	for i < len(short) && j < len(long) {
		if short[i] == long[j] {
			i++
			j++
			continue
		}
		if skipped {
			return false
		}
		skipped = true
		j++
	}
	return true
}

// Ignored 报告某个目录是否命中忽略名单。
//
// 名单里存的是路径片段，比较前统一成「小写、正斜杠、带尾斜杠」，
// 于是 "/tmp/" 能命中 "/tmp" 也命中 "/tmp/x"，但不会命中 "/tmpx"。
func Ignored(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	p := normaliseForMatch(path, true)
	for _, pat := range patterns {
		if pat = normaliseForMatch(pat, false); pat != "" && strings.Contains(p, pat) {
			return true
		}
	}
	return false
}

func normaliseForMatch(s string, ensureTrailingSlash bool) string {
	s = strings.ToLower(strings.ReplaceAll(s, "\\", "/"))
	if ensureTrailingSlash && !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

// score = 匹配质量 x （1 + 去过几次） x 多久没去。
//
// 三项相乘而不是相加：任何一项为零，这条候选就不该存在。
// 时间项按半衰期衰减，「上个月常去」会自然让位给「今天刚去过」。
func score(q float64, e store.Entry, now int64, halfLifeDays float64) float64 {
	visits := e.Visits
	if visits < 1 {
		visits = 1
	}
	if visits > maxVisitWeight {
		visits = maxVisitWeight
	}

	age := float64(now - e.Last)
	if age < 0 {
		age = 0
	}
	decay := 1.0
	if halfLifeDays > 0 {
		decay = math.Exp2(-age / (halfLifeDays * 86400))
	}

	return q * float64(1+visits) * decay
}

// sortCandidates 按分数降序。同分时路径短者优先，再同就按字典序 ——
// 最后一级是为了让结果可复现：map 的遍历顺序在 Go 里是随机的。
func sortCandidates(out []Candidate) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if len(out[i].Path) != len(out[j].Path) {
			return len(out[i].Path) < len(out[j].Path)
		}
		return out[i].Path < out[j].Path
	})
}

// Outcome 是一次决策的结果。
type Outcome int

// 三种结果。
const (
	OutcomeNone Outcome = iota
	OutcomeUnique
	OutcomeAmbiguous
)

// Decision 是决策结果。
type Decision struct {
	Outcome Outcome
	Best    Candidate
	Top     []Candidate
	Ratio   float64
}

// Decide 决定跳还是不跳。
//
// 前两名分数太接近时不猜：跳错了比让你再打两个字贵得多。
func Decide(cands []Candidate, tau float64, topN int) Decision {
	if topN <= 0 {
		topN = 10
	}
	switch len(cands) {
	case 0:
		return Decision{Outcome: OutcomeNone}
	case 1:
		return Decision{Outcome: OutcomeUnique, Best: cands[0], Top: cands}
	}

	top := cands
	if len(top) > topN {
		top = top[:topN]
	}

	ratio := 1.0
	if cands[1].Score > 0 {
		ratio = cands[0].Score / cands[1].Score
	}
	if tau > 0 && ratio < tau {
		return Decision{Outcome: OutcomeAmbiguous, Best: cands[0], Top: top, Ratio: ratio}
	}
	return Decision{Outcome: OutcomeUnique, Best: cands[0], Top: top, Ratio: ratio}
}

// Keywords 规范化用户输入：小写、去空白、丢掉空串。
func Keywords(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			out = append(out, a)
		}
	}
	return out
}
