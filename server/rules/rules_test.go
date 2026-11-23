package rules

import "testing"

// cardsOf 按牌点构造牌；同一点数自动取不同花色。
func cardsOf(ranks ...int) []Card {
	count := make(map[int]int)
	out := make([]Card, 0, len(ranks))
	for _, r := range ranks {
		if r == SmallJoker {
			out = append(out, CardByID(52))
			continue
		}
		if r == BigJoker {
			out = append(out, CardByID(53))
			continue
		}
		suit := count[r]
		count[r]++
		id := (r-3)*4 + suit
		out = append(out, CardByID(id))
	}
	SortCards(out)
	return out
}

func mustParse(t *testing.T, ranks ...int) Combo {
	t.Helper()
	c, ok := Parse(cardsOf(ranks...))
	if !ok {
		t.Fatalf("应该识别为合法牌型: %v", ranks)
	}
	return c
}

func TestParseBasic(t *testing.T) {
	cases := []struct {
		ranks []int
		typ   ComboType
		main  int
		len   int
	}{
		{[]int{3}, Single, 3, 1},
		{[]int{5, 5}, Pair, 5, 1},
		{[]int{7, 7, 7}, Triple, 7, 1},
		{[]int{8, 8, 8, 3}, TripleSingle, 8, 1},
		{[]int{9, 9, 9, 4, 4}, TriplePair, 9, 1},
		{[]int{3, 4, 5, 6, 7}, Straight, 3, 5},
		{[]int{10, 11, 12, 13, 14}, Straight, 10, 5},
		{[]int{3, 3, 4, 4, 5, 5}, DoubleStraight, 3, 3},
		{[]int{3, 3, 3, 4, 4, 4}, Airplane, 3, 2},
		{[]int{3, 3, 3, 4, 4, 4, 5, 6}, AirplaneSingle, 3, 2},
		{[]int{3, 3, 3, 4, 4, 4, 5, 5, 6, 6}, AirplanePair, 3, 2},
		{[]int{9, 9, 9, 9, 3, 4}, FourTwoSingle, 9, 1},
		{[]int{9, 9, 9, 9, 3, 3, 4, 4}, FourTwoPair, 9, 1},
		{[]int{10, 10, 10, 10}, Bomb, 10, 1},
		{[]int{SmallJoker, BigJoker}, Rocket, BigJoker, 1},
	}
	for _, tc := range cases {
		c := mustParse(t, tc.ranks...)
		if c.Type != tc.typ || c.Main != tc.main || c.Len != tc.len {
			t.Fatalf("%v => %+v，期望 type=%s main=%d len=%d", tc.ranks, c, tc.typ, tc.main, tc.len)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	cases := [][]int{
		{3, 4},
		{3, 3, 4, 5},
		{3, 4, 5, 6},
		{3, 3, 4, 4},
		{3, 3, 3, 4, 4, 5},
		{3, 3, 3, 4, 4, 4, 5},
		{3, 3, 3, 3, 4, 4, 4, 4},
	}
	for _, ranks := range cases {
		if c, ok := Parse(cardsOf(ranks...)); ok {
			t.Fatalf("%v 不应合法，得到 %+v", ranks, c)
		}
	}
}

func TestParseNoTwoInSequence(t *testing.T) {
	// 10 J Q K A 2 不能当顺子
	if c, ok := Parse(cardsOf(10, 11, 12, 13, 14, 15)); ok {
		t.Fatalf("2 不能进顺子，得到 %+v", c)
	}
	// 2 也不能进连对/飞机
	if c, ok := Parse(cardsOf(14, 14, 15, 15, 3, 3)); ok {
		t.Fatalf("2 不能进连对，得到 %+v", c)
	}
}

func TestBeats(t *testing.T) {
	if !mustParse(t, 4).Beats(mustParse(t, 3)) {
		t.Fatal("4 应该压 3")
	}
	if mustParse(t, 3).Beats(mustParse(t, 4)) {
		t.Fatal("3 不能压 4")
	}
	if mustParse(t, 5, 5).Beats(mustParse(t, 4)) {
		t.Fatal("对子不能压单张")
	}
	if !mustParse(t, 5, 5).Beats(mustParse(t, 4, 4)) {
		t.Fatal("5 对应该压 4 对")
	}
	if !mustParse(t, 10, 10, 10, 10).Beats(mustParse(t, 14, 14, 14, 14, 3, 3)) {
		t.Fatal("炸弹应该压四带二")
	}
	if mustParse(t, 14, 14, 14, 14).Beats(mustParse(t, 3, 3, 3, 3)) == false {
		t.Fatal("大炸弹应该压小炸弹")
	}
	if !mustParse(t, SmallJoker, BigJoker).Beats(mustParse(t, 14, 14, 14, 14)) {
		t.Fatal("王炸应该压炸弹")
	}
	if mustParse(t, 3, 4, 5, 6, 7).Beats(mustParse(t, 4, 5, 6, 7, 8, 9)) {
		t.Fatal("长度不同的顺子不能比")
	}
}

func TestHintsLeadingStartsWithSmallestSingle(t *testing.T) {
	hand := cardsOf(3, 4, 5, 5, 5, 6, 7, 8, 9, 9, 9, 9, 10, 11, 12, 13, 14, 15, SmallJoker, BigJoker)
	hints := Hints(hand, nil)
	if len(hints) == 0 {
		t.Fatal("应该有提示")
	}
	if hints[0].Combo.Type != Single || hints[0].Combo.Main != 3 {
		t.Fatalf("首出最小提示应是单 3，得到 %+v", hints[0])
	}
	// 应该包含炸弹和王炸
	hasBomb, hasRocket := false, false
	for _, p := range hints {
		if p.Combo.Type == Bomb {
			hasBomb = true
		}
		if p.Combo.Type == Rocket {
			hasRocket = true
		}
	}
	if !hasBomb || !hasRocket {
		t.Fatalf("提示应包含炸弹和王炸，bomb=%v rocket=%v", hasBomb, hasRocket)
	}
}

func TestHintsFollowOnlyBeating(t *testing.T) {
	hand := cardsOf(3, 4, 4, 5, 5, 5, 6, 6, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 15, 15)
	last := mustParse(t, 5)
	hints := Hints(hand, &last)
	if len(hints) == 0 {
		t.Fatal("应该有能压单 5 的提示")
	}
	for _, p := range hints {
		if !p.Combo.Beats(last) {
			t.Fatalf("提示包含不能压的牌: %+v", p)
		}
	}
	// 不能出现单 3/4
	for _, p := range hints {
		if p.Combo.Type == Single && p.Combo.Main <= 5 {
			t.Fatalf("单 %d 不能压单 5", p.Combo.Main)
		}
	}
}

func TestHintsRespectRocket(t *testing.T) {
	hand := cardsOf(3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 15, 15, 15, SmallJoker, BigJoker)
	last := mustParse(t, SmallJoker, BigJoker)
	hints := Hints(hand, &last)
	if len(hints) != 0 {
		t.Fatalf("王炸之后应该没有能压的牌，得到 %d 个", len(hints))
	}
}

func TestSettleBasic(t *testing.T) {
	out := Settle(ScoreInput{
		Base: 1, RobMultiplier: 1, Landlord: 0, LandlordWon: true,
		Balance: [3]int{100, 100, 100},
	})
	if out.PublicMultiplier != 1 {
		t.Fatalf("公共倍数应为 1，得到 %d", out.PublicMultiplier)
	}
	if out.Deltas != [3]int{2, -1, -1} {
		t.Fatalf("地主赢应 +2/-1/-1，得到 %+v", out.Deltas)
	}
}

func TestSettleDoubles(t *testing.T) {
	out := Settle(ScoreInput{
		Base: 1, RobMultiplier: 1, Landlord: 0, LandlordWon: true,
		LandlordDouble: true,
		Doubles:        [3]bool{false, true, false},
		Balance:        [3]int{100, 100, 100},
	})
	// 农民1加倍：地主从农民1拿 1*2*2=4，从农民2拿 1*2*1=2
	if out.Deltas != [3]int{6, -4, -2} {
		t.Fatalf("加倍结算错误，得到 %+v", out.Deltas)
	}
}

func TestSettleCapByBalance(t *testing.T) {
	out := Settle(ScoreInput{
		Base: 1, RobMultiplier: 1, Landlord: 0, LandlordWon: true,
		Balance: [3]int{100, 1, 0},
	})
	if out.Deltas != [3]int{1, -1, 0} {
		t.Fatalf("应按余额封顶，得到 %+v", out.Deltas)
	}
}

func TestSettleFarmersWinLandlordCap(t *testing.T) {
	out := Settle(ScoreInput{
		Base: 1, RobMultiplier: 1, Landlord: 0, LandlordWon: false,
		Balance: [3]int{1, 100, 100},
	})
	// 地主只输 1 分，两农民按 1:1 分配，余数给最后一个农民
	if out.Deltas != [3]int{-1, 0, 1} {
		t.Fatalf("地主封顶分配错误，得到 %+v", out.Deltas)
	}
}

func TestSettleZeroSum(t *testing.T) {
	for _, landlordWon := range []bool{true, false} {
		out := Settle(ScoreInput{
			Base: 1, RobMultiplier: 8, BombCount: 3, Spring: landlordWon,
			Landlord: 1, LandlordWon: landlordWon,
			LandlordDouble: true,
			Doubles:        [3]bool{true, false, true},
			Balance:        [3]int{37, 61, 13},
		})
		sum := 0
		for _, d := range out.Deltas {
			sum += d
		}
		if sum != 0 {
			t.Fatalf("结算必须零和，landlordWon=%v deltas=%+v sum=%d", landlordWon, out.Deltas, sum)
		}
	}
}
