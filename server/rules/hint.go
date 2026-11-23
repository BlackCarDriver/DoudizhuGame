package rules

import "sort"

// Play 一组可出的牌。
type Play struct {
	Cards []Card `json:"cards"`
	Combo Combo  `json:"combo"`
}

// Hints 返回所有能压过 last 的候选；last 为 nil 表示自己首出。
// 结果按“普通牌型 → 炸弹 → 王炸”排序，首出时最小单张排在最前。
func Hints(hand []Card, last *Combo) []Play {
	all := AllPlays(hand)
	out := make([]Play, 0, len(all))
	for _, p := range all {
		if last == nil || p.Combo.Beats(*last) {
			out = append(out, p)
		}
	}
	SortPlays(out)
	if len(out) > 400 {
		out = out[:400]
	}
	return out
}

// AllPlays 生成手牌里值得提示的合法组合。
//
// 这不是“所有组合”的穷举（那会爆炸），而是每个主牌型给出一个最低配的候选：
// 单、对、三、三带一/二、顺子、连对、飞机及带翅膀、四带二、炸弹、王炸。
// 玩家仍然可以手动选任意牌；提示只负责给出可用的代表组合。
func AllPlays(hand []Card) []Play {
	if len(hand) == 0 {
		return nil
	}
	byRank := make(map[int][]Card)
	for _, c := range hand {
		byRank[c.Rank] = append(byRank[c.Rank], c)
	}
	ranks := make([]int, 0, len(byRank))
	for r := range byRank {
		sort.Slice(byRank[r], func(i, j int) bool { return byRank[r][i].ID < byRank[r][j].ID })
		ranks = append(ranks, r)
	}
	sort.Ints(ranks)

	seen := make(map[string]bool)
	out := make([]Play, 0, 64)
	add := func(cards []Card) {
		if len(cards) == 0 {
			return
		}
		cp := append([]Card(nil), cards...)
		ct, ok := Parse(cp)
		if !ok {
			return
		}
		key := cardsKey(cp)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Play{Cards: cp, Combo: ct})
	}

	// 单 / 对 / 三 / 炸
	for _, r := range ranks {
		cs := byRank[r]
		add(cs[:1])
		if len(cs) >= 2 {
			add(cs[:2])
		}
		if len(cs) >= 3 {
			add(cs[:3])
		}
		if len(cs) == 4 {
			add(cs[:4])
		}
	}
	// 三带一 / 三带二
	for _, r := range ranks {
		if len(byRank[r]) < 3 {
			continue
		}
		triple := byRank[r][:3]
		for _, r2 := range ranks {
			if r2 == r {
				continue
			}
			cs := byRank[r2]
			if len(cs) >= 1 {
				add(append(append([]Card{}, triple...), cs[0]))
			}
			if len(cs) >= 2 {
				add(append(append([]Card{}, triple...), cs[:2]...))
			}
		}
	}
	// 顺子（5 张起）
	for start := Rank3; start <= RankA; start++ {
		for length := 5; start+length-1 <= RankA; length++ {
			ok := true
			cards := make([]Card, 0, length)
			for r := start; r < start+length; r++ {
				if len(byRank[r]) == 0 {
					ok = false
					break
				}
				cards = append(cards, byRank[r][0])
			}
			if ok {
				add(cards)
			}
		}
	}
	// 连对（3 对起）
	for start := Rank3; start <= RankA; start++ {
		for pairs := 3; start+pairs-1 <= RankA; pairs++ {
			ok := true
			cards := make([]Card, 0, pairs*2)
			for r := start; r < start+pairs; r++ {
				if len(byRank[r]) < 2 {
					ok = false
					break
				}
				cards = append(cards, byRank[r][:2]...)
			}
			if ok {
				add(cards)
			}
		}
	}
	// 飞机（2 个三张起），以及带单/带对
	for start := Rank3; start <= RankA; start++ {
		for groups := 2; start+groups-1 <= RankA; groups++ {
			ok := true
			body := make([]Card, 0, groups*3)
			runSet := make(map[int]bool, groups)
			for r := start; r < start+groups; r++ {
				if len(byRank[r]) < 3 {
					ok = false
					break
				}
				body = append(body, byRank[r][:3]...)
				runSet[r] = true
			}
			if !ok {
				continue
			}
			add(body)
			// 剩余牌（排除飞机本体牌点）
			rest := make([]Card, 0, len(hand))
			for _, c := range hand {
				if !runSet[c.Rank] {
					rest = append(rest, c)
				}
			}
			SortCards(rest)
			if len(rest) >= groups {
				add(append(append([]Card{}, body...), rest[:groups]...))
			}
			// 带对：从剩余里挑 groups 个不同牌点的对子
			pairCards := make([]Card, 0, groups*2)
			used := 0
			for _, r := range ranks {
				if runSet[r] || len(byRank[r]) < 2 {
					continue
				}
				pairCards = append(pairCards, byRank[r][:2]...)
				used++
				if used == groups {
					break
				}
			}
			if used == groups {
				add(append(append([]Card{}, body...), pairCards...))
			}
		}
	}
	// 四带二：两单 / 两对
	for _, r := range ranks {
		if len(byRank[r]) != 4 {
			continue
		}
		four := byRank[r][:4]
		rest := make([]Card, 0, len(hand)-4)
		for _, c := range hand {
			if c.Rank != r {
				rest = append(rest, c)
			}
		}
		SortCards(rest)
		if len(rest) >= 2 {
			add(append(append([]Card{}, four...), rest[:2]...))
		}
		pairCards := make([]Card, 0, 4)
		used := 0
		for _, r2 := range ranks {
			if r2 == r || len(byRank[r2]) < 2 {
				continue
			}
			pairCards = append(pairCards, byRank[r2][:2]...)
			used++
			if used == 2 {
				break
			}
		}
		if used == 2 {
			add(append(append([]Card{}, four...), pairCards...))
		}
	}
	// 王炸
	if len(byRank[SmallJoker]) > 0 && len(byRank[BigJoker]) > 0 {
		add([]Card{byRank[SmallJoker][0], byRank[BigJoker][0]})
	}

	SortPlays(out)
	return out
}

// SortPlays 普通牌型在前，炸弹/王炸在后；同类型按主牌点、序列长度、牌 ID 排序。
func SortPlays(plays []Play) {
	sort.SliceStable(plays, func(i, j int) bool {
		a, b := plays[i].Combo, plays[j].Combo
		if TypeOrder(a.Type) != TypeOrder(b.Type) {
			return TypeOrder(a.Type) < TypeOrder(b.Type)
		}
		if a.Main != b.Main {
			return a.Main < b.Main
		}
		if a.Len != b.Len {
			return a.Len < b.Len
		}
		return cardsKey(plays[i].Cards) < cardsKey(plays[j].Cards)
	})
}

func cardsKey(cards []Card) string {
	ids := make([]int, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	sort.Ints(ids)
	b := make([]byte, 0, len(ids)*3)
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, byte('0'+id/10), byte('0'+id%10))
	}
	return string(b)
}
