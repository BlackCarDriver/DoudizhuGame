package rules

// ScoreInput 一局结算的输入。
type ScoreInput struct {
	Base           int  // 底分，默认 1
	RobMultiplier  int  // 抢地主倍数：1/2/4/8
	BombCount      int  // 本局打出的炸弹 + 王炸数量
	Spring         bool // 地主春天
	AntiSpring     bool // 农民反春
	Landlord       int  // 地主座位 0..2
	LandlordDouble bool // 地主是否加倍
	Doubles        [3]bool
	Balance        [3]int
	LandlordWon    bool
}

// ScoreOutput 结算结果。
type ScoreOutput struct {
	PublicMultiplier int
	Deltas           [3]int // 每个座位的积分变化，正数赢、负数输
}

// Settle 按欢乐斗地主的口径计算输赢：
//
//	公共倍数 = 抢地主倍数 × 2^(炸弹数) × 春天/反春
//	农民 i 的输赢 = 底分 × 公共倍数 × 地主加倍 × 农民 i 自己加倍
//	地主的输赢 = 两个农民输赢之和（方向相反）
//
// 每个玩家最多输掉自己当前积分；赢家只拿实际扣掉的部分，保证零和。
func Settle(in ScoreInput) ScoreOutput {
	base := in.Base
	if base < 1 {
		base = 1
	}
	rob := in.RobMultiplier
	if rob < 1 {
		rob = 1
	}
	public := rob
	for i := 0; i < in.BombCount; i++ {
		public *= 2
	}
	if in.Spring || in.AntiSpring {
		public *= 2
	}
	if public < 1 {
		public = 1
	}
	unit := base * public
	if unit < 1 {
		unit = 1
	}
	landlordFactor := 1
	if in.LandlordDouble {
		landlordFactor = 2
	}

	var out ScoreOutput
	out.PublicMultiplier = public
	var claim [3]int
	for i := 0; i < 3; i++ {
		if i == in.Landlord {
			continue
		}
		f := 1
		if in.Doubles[i] {
			f = 2
		}
		claim[i] = unit * landlordFactor * f
	}

	if in.LandlordWon {
		total := 0
		for i := 0; i < 3; i++ {
			if i == in.Landlord {
				continue
			}
			loss := claim[i]
			if loss > in.Balance[i] {
				loss = in.Balance[i]
			}
			if loss < 0 {
				loss = 0
			}
			out.Deltas[i] = -loss
			total += loss
		}
		out.Deltas[in.Landlord] = total
		return out
	}

	// 农民赢：地主先按余额封顶，再按两个农民的应得比例分配。
	totalClaim := 0
	for i := 0; i < 3; i++ {
		if i != in.Landlord {
			totalClaim += claim[i]
		}
	}
	landlordLoss := totalClaim
	if landlordLoss > in.Balance[in.Landlord] {
		landlordLoss = in.Balance[in.Landlord]
	}
	if landlordLoss < 0 {
		landlordLoss = 0
	}
	out.Deltas[in.Landlord] = -landlordLoss
	if landlordLoss == 0 || totalClaim == 0 {
		return out
	}
	remaining := landlordLoss
	lastFarmer := -1
	for i := 0; i < 3; i++ {
		if i == in.Landlord {
			continue
		}
		share := landlordLoss * claim[i] / totalClaim
		if share > remaining {
			share = remaining
		}
		out.Deltas[i] = share
		remaining -= share
		lastFarmer = i
	}
	// 整数除法的余数给最后一个农民，保证零和。
	if remaining > 0 && lastFarmer >= 0 {
		out.Deltas[lastFarmer] += remaining
	}
	return out
}
