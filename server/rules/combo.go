package rules

import "sort"

// ComboType 牌型。
type ComboType string

const (
	Single         ComboType = "single"
	Pair           ComboType = "pair"
	Triple         ComboType = "triple"
	TripleSingle   ComboType = "triple_single"
	TriplePair     ComboType = "triple_pair"
	Straight       ComboType = "straight"
	DoubleStraight ComboType = "double_straight"
	Airplane       ComboType = "airplane"
	AirplaneSingle ComboType = "airplane_single"
	AirplanePair   ComboType = "airplane_pair"
	FourTwoSingle  ComboType = "four_two_single"
	FourTwoPair    ComboType = "four_two_pair"
	Bomb           ComboType = "bomb"
	Rocket         ComboType = "rocket"
)

// Combo 一组合法牌型。
//
// Main 是用于比较的主牌点：
//   - 单/对/三/三带一/三带二：主牌点
//   - 顺子/连对/飞机：连续牌点的最小值
//   - 四带二：四张的牌点
//   - 炸弹：四张的牌点
//   - 王炸：大王牌点
//
// Len 是序列长度（顺子=张数，连对=对子数，飞机=三张组数），其他牌型为 1。
type Combo struct {
	Type ComboType `json:"type"`
	Main int       `json:"main"`
	Len  int       `json:"len"`
}

// Beats 判断 c 是否能压过 last。
func (c Combo) Beats(last Combo) bool {
	if c.Type == Rocket {
		return last.Type != Rocket
	}
	if last.Type == Rocket {
		return false
	}
	if c.Type == Bomb && last.Type != Bomb {
		return true
	}
	if c.Type != Bomb && last.Type == Bomb {
		return false
	}
	if c.Type != last.Type {
		return false
	}
	if c.Len != last.Len {
		return false
	}
	return c.Main > last.Main
}

func countsOf(cards []Card) map[int]int {
	m := make(map[int]int, len(cards))
	for _, c := range cards {
		m[c.Rank]++
	}
	return m
}

func sortedRanks(m map[int]int) []int {
	out := make([]int, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	sort.Ints(out)
	return out
}

func allCounts(m map[int]int, want int) bool {
	for _, n := range m {
		if n != want {
			return false
		}
	}
	return true
}

func consecutive(ranks []int) bool {
	for i := 1; i < len(ranks); i++ {
		if ranks[i] != ranks[i-1]+1 {
			return false
		}
	}
	return true
}

func maxRank(ranks []int) int {
	if len(ranks) == 0 {
		return 0
	}
	return ranks[len(ranks)-1]
}

func sequenceRanks(ranks []int) bool {
	if len(ranks) == 0 {
		return false
	}
	// 2 和王不能进顺子/连对/飞机。
	return maxRank(ranks) <= RankA
}

// Parse 识别一组牌的牌型。cards 顺序不影响结果。
func Parse(cards []Card) (Combo, bool) {
	n := len(cards)
	if n == 0 {
		return Combo{}, false
	}
	cnt := countsOf(cards)
	ranks := sortedRanks(cnt)

	// 王炸
	if n == 2 && cnt[SmallJoker] == 1 && cnt[BigJoker] == 1 {
		return Combo{Type: Rocket, Main: BigJoker, Len: 1}, true
	}
	// 炸弹
	if n == 4 && len(ranks) == 1 {
		return Combo{Type: Bomb, Main: ranks[0], Len: 1}, true
	}
	// 单/对/三
	if len(ranks) == 1 {
		switch n {
		case 1:
			return Combo{Type: Single, Main: ranks[0], Len: 1}, true
		case 2:
			return Combo{Type: Pair, Main: ranks[0], Len: 1}, true
		case 3:
			return Combo{Type: Triple, Main: ranks[0], Len: 1}, true
		}
	}
	// 三带一
	if n == 4 && len(ranks) == 2 {
		if cnt[ranks[0]] == 3 && cnt[ranks[1]] == 1 {
			return Combo{Type: TripleSingle, Main: ranks[0], Len: 1}, true
		}
		if cnt[ranks[1]] == 3 && cnt[ranks[0]] == 1 {
			return Combo{Type: TripleSingle, Main: ranks[1], Len: 1}, true
		}
	}
	// 三带二
	if n == 5 && len(ranks) == 2 {
		if cnt[ranks[0]] == 3 && cnt[ranks[1]] == 2 {
			return Combo{Type: TriplePair, Main: ranks[0], Len: 1}, true
		}
		if cnt[ranks[1]] == 3 && cnt[ranks[0]] == 2 {
			return Combo{Type: TriplePair, Main: ranks[1], Len: 1}, true
		}
	}
	// 顺子
	if n >= 5 && allCounts(cnt, 1) && consecutive(ranks) && sequenceRanks(ranks) {
		return Combo{Type: Straight, Main: ranks[0], Len: n}, true
	}
	// 连对
	if n >= 6 && n%2 == 0 && allCounts(cnt, 2) && len(ranks) >= 3 && consecutive(ranks) && sequenceRanks(ranks) {
		return Combo{Type: DoubleStraight, Main: ranks[0], Len: len(ranks)}, true
	}
	// 飞机（不带翅膀）
	if n >= 6 && n%3 == 0 && allCounts(cnt, 3) && len(ranks) >= 2 && consecutive(ranks) && sequenceRanks(ranks) {
		return Combo{Type: Airplane, Main: ranks[0], Len: len(ranks)}, true
	}
	// 四带二：四张 + 两单，或四张 + 两对
	for _, four := range ranks {
		if cnt[four] != 4 {
			continue
		}
		rest := make([]Card, 0, n-4)
		for _, c := range cards {
			if c.Rank != four {
				rest = append(rest, c)
			}
		}
		if len(rest) == 2 {
			return Combo{Type: FourTwoSingle, Main: four, Len: 1}, true
		}
		if len(rest) == 4 {
			rc := countsOf(rest)
			rr := sortedRanks(rc)
			if len(rr) == 2 && rc[rr[0]] == 2 && rc[rr[1]] == 2 {
				return Combo{Type: FourTwoPair, Main: four, Len: 1}, true
			}
		}
	}
	// 飞机带翅膀：连续三张组 + 同数量单牌/对子
	if c, ok := parseAirplaneWithWings(cnt, ranks, n); ok {
		return c, true
	}
	return Combo{}, false
}

func parseAirplaneWithWings(cnt map[int]int, ranks []int, n int) (Combo, bool) {
	// 找出所有连续的三张组（2 和王不能进飞机）。
	var run []int
	tryRun := func(run []int) (Combo, bool) {
		k := len(run)
		if k < 2 {
			return Combo{}, false
		}
		rem := make(map[int]int, len(cnt))
		for r, c := range cnt {
			rem[r] = c
		}
		runSet := make(map[int]bool, k)
		for _, r := range run {
			runSet[r] = true
			rem[r] -= 3
			if rem[r] < 0 {
				return Combo{}, false
			}
			if rem[r] == 0 {
				delete(rem, r)
			}
		}
		// 翅膀不能再用三张组里的牌点。
		for r := range rem {
			if runSet[r] {
				return Combo{}, false
			}
		}
		remTotal := 0
		for _, c := range rem {
			remTotal += c
		}
		if remTotal == k {
			return Combo{Type: AirplaneSingle, Main: run[0], Len: k}, true
		}
		if remTotal == 2*k {
			rr := sortedRanks(rem)
			if len(rr) == k {
				allPairs := true
				for _, r := range rr {
					if rem[r] != 2 {
						allPairs = false
						break
					}
				}
				if allPairs {
					return Combo{Type: AirplanePair, Main: run[0], Len: k}, true
				}
			}
		}
		return Combo{}, false
	}

	for i := 0; i < len(ranks); i++ {
		if cnt[ranks[i]] < 3 || ranks[i] > RankA {
			continue
		}
		run = append(run, ranks[i])
		for j := i + 1; j < len(ranks); j++ {
			if ranks[j] != ranks[j-1]+1 || ranks[j] > RankA || cnt[ranks[j]] < 3 {
				break
			}
			run = append(run, ranks[j])
		}
		// 从长到短尝试，优先识别更长的飞机。
		for k := len(run); k >= 2; k-- {
			sub := run[:k]
			if c, ok := tryRun(sub); ok {
				return c, true
			}
		}
		// 跳过本段，避免重复。
		i += len(run) - 1
		run = nil
	}
	return Combo{}, false
}

// TypeOrder 提示排序：普通牌型在前，炸弹/王炸在后。
func TypeOrder(t ComboType) int {
	switch t {
	case Single:
		return 0
	case Pair:
		return 1
	case Triple:
		return 2
	case TripleSingle:
		return 3
	case TriplePair:
		return 4
	case Straight:
		return 5
	case DoubleStraight:
		return 6
	case Airplane:
		return 7
	case AirplaneSingle:
		return 8
	case AirplanePair:
		return 9
	case FourTwoSingle:
		return 10
	case FourTwoPair:
		return 11
	case Bomb:
		return 12
	case Rocket:
		return 13
	default:
		return 99
	}
}
