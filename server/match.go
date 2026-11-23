package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"time"

	"DoudizhuGame/server/rules"
)

type PlayInfo struct {
	Seat  int          `json:"seat"`
	Cards []rules.Card `json:"cards"`
	Combo rules.Combo  `json:"combo"`
}

type ActionInfo struct {
	Seat  int          `json:"seat"`
	Pass  bool         `json:"pass"`
	Cards []rules.Card `json:"cards,omitempty"`
	Combo *rules.Combo `json:"combo,omitempty"`
}

type PlayerResult struct {
	Seat       string `json:"seat"`
	Nick       string `json:"nick"`
	Avatar     string `json:"avatar"`
	Role       string `json:"role"`
	Double     bool   `json:"double"`
	Delta      int    `json:"delta"`
	Points     int    `json:"points"`
	Rake       int    `json:"rake,omitempty"`
	DailyBonus int    `json:"dailyBonus,omitempty"`
}

type ResultInfo struct {
	WinnerSide       string         `json:"winnerSide"`
	Landlord         string         `json:"landlord"`
	Spring           bool           `json:"spring"`
	AntiSpring       bool           `json:"antiSpring"`
	BombCount        int            `json:"bombCount"`
	RobMultiplier    int            `json:"robMultiplier"`
	PublicMultiplier int            `json:"publicMultiplier"`
	Base             int            `json:"base"`
	Rake             int            `json:"rake,omitempty"`
	Players          []PlayerResult `json:"players"`
	Reason           string         `json:"reason"`
}

type Match struct {
	ID       int64
	Phase    string // countdown | bidding | doubling | playing | over
	Turn     int    // -1 = 无
	TurnSeq  int64
	Timer    *time.Timer
	Deadline int64

	CountdownEnd int64
	StartAt      int64

	Hands  [3][]rules.Card
	Bottom []rules.Card
	// SpectatorCards 本局是否允许观众看牌，开局前由三个座位的投票决定。
	SpectatorCards bool

	Landlord      int
	RobMultiplier int
	RobCount      int
	FinalChance   bool
	BidStage      string // call | rob
	BidTurn       int
	Caller        int
	RobQueue      []int
	BidActions    [3]string
	RedealCount   int

	Double        [3]bool
	DoubleDone    [3]bool
	DoubleActions [3]string

	LastPlay    *PlayInfo
	LastPlayer  int
	PassCount   int
	LastActions [3]*ActionInfo
	PlayCount   [3]int
	BombCount   int

	WinnerSide       string
	PublicMultiplier int
	Result           *ResultInfo
}

// ============================================================
// 座位 / 准备
// ============================================================

func (h *Hub) seatOfNickLocked(nick string) int {
	for i := range h.seats {
		if nick != "" && h.seats[i].Nick == nick {
			return i
		}
	}
	return -1
}

func (h *Hub) controlledSeatLocked(nick string) int {
	i := h.seatOfNickLocked(nick)
	if i >= 0 && h.seats[i].Connected && !h.seats[i].Auto {
		return i
	}
	return -1
}

func (h *Hub) ensureMinPointsLocked(nick string) (int, string) {
	points := h.store.Points(nick)
	if h.cfg.MinPointsToSit <= 0 || points >= h.cfg.MinPointsToSit {
		return points, ""
	}
	if amount, ok := h.store.ClaimRelief(nick); ok {
		log.Printf("[hub] %s 领取救济 +%d，当前 %d", nick, amount, h.store.Points(nick))
		points = h.store.Points(nick)
	} else {
		return points, "积分不足，今日救济已领"
	}
	// 如果今天已经打过一局，但结算时余额不足没发活跃奖励，
	// 领取救济后余额达标，可以在这里补发一次。
	if bonus, ok := h.store.ClaimActiveReward(nick); ok {
		log.Printf("[hub] %s 补发每日活跃奖励 +%d", nick, bonus)
		points = h.store.Points(nick)
	}
	return points, ""
}

func (h *Hub) sitAt(nick, want string) string {
	h.lock.Lock()
	if h.closed {
		h.lock.Unlock()
		return "房间已关闭"
	}
	if h.activeMatchLocked() {
		h.lock.Unlock()
		return "对局中不能换座"
	}
	wantIdx := seatIndex(want)
	current := h.seatOfNickLocked(nick)
	if current >= 0 && (h.seats[current].Auto || !h.seats[current].Connected) {
		// 上一局托管的残留座位：先释放，再按普通上座处理。
		h.seats[current] = SeatInfo{}
		h.ready[current] = false
		h.allowVotes[current] = false
		current = -1
	}
	if wantIdx < 0 {
		if current >= 0 {
			h.lock.Unlock()
			return ""
		}
		for i := 0; i < 3; i++ {
			if h.seats[i].Nick == "" {
				wantIdx = i
				break
			}
		}
		if wantIdx < 0 {
			h.lock.Unlock()
			return "座位已满，先观战这一局"
		}
	}
	if wantIdx == current {
		h.lock.Unlock()
		return ""
	}
	if current >= 0 && h.ready[current] {
		h.lock.Unlock()
		return "先取消准备再换座"
	}
	if h.seats[wantIdx].Nick != "" {
		h.lock.Unlock()
		return "这个座位已经有人了"
	}
	points, minErr := h.ensureMinPointsLocked(nick)
	if minErr != "" {
		h.lock.Unlock()
		return minErr
	}
	c := h.clients[nick]
	info := SeatInfo{Nick: nick, Connected: true, Points: points}
	if c != nil {
		info.Avatar, info.Frame, info.RoleID = c.avatar, c.frame, c.roleID
	}
	vote := true
	if current >= 0 {
		vote = h.allowVotes[current]
		h.seats[current] = SeatInfo{}
		h.ready[current] = false
		h.allowVotes[current] = false
	}
	h.seats[wantIdx] = info
	h.ready[wantIdx] = false
	h.allowVotes[wantIdx] = vote
	started := h.maybeStartLocked()
	h.lock.Unlock()
	if started {
		h.eventLog("对局开始")
	}
	h.syncAll()
	return ""
}

func (h *Hub) stand(nick string) {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return
	}
	if h.match != nil && h.match.Phase != "over" {
		if h.match.Phase == "countdown" {
			h.cancelCountdownLocked()
			h.seats[seat] = SeatInfo{}
			h.ready[seat] = false
			h.allowVotes[seat] = false
		} else {
			// 对局中离座：座位保留，系统托管；本人转为观众。
			h.seats[seat].Connected = false
			h.seats[seat].Auto = true
			h.ready[seat] = false
			h.allowVotes[seat] = false
			if h.match.Turn == seat {
				h.armTimerLocked(autoActSeconds)
			}
		}
	} else {
		h.seats[seat] = SeatInfo{}
		h.ready[seat] = false
		h.allowVotes[seat] = false
	}
	h.lock.Unlock()
	log.Printf("[hub] %s 离座", nick)
	h.syncAll()
}

func (h *Hub) voteSpectator(nick string, allow bool) string {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return "先上座"
	}
	if h.activeMatchLocked() {
		h.lock.Unlock()
		return "对局已经开始，不能修改投票"
	}
	h.allowVotes[seat] = allow
	h.lock.Unlock()
	h.pushState()
	return ""
}

func (h *Hub) setReady(nick string, ready bool) string {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return "先上座再准备"
	}
	if h.match != nil && h.match.Phase != "over" && h.match.Phase != "countdown" {
		h.lock.Unlock()
		return "本局还没结束"
	}
	if ready {
		points, minErr := h.ensureMinPointsLocked(nick)
		if minErr != "" {
			h.lock.Unlock()
			return minErr
		}
		h.seats[seat].Points = points
	}
	h.ready[seat] = ready
	if !ready && h.match != nil && h.match.Phase == "countdown" {
		h.cancelCountdownLocked()
	}
	started := h.maybeStartLocked()
	h.lock.Unlock()
	if started {
		h.eventLog("对局开始")
	}
	h.syncAll()
	return ""
}

func (h *Hub) maybeStartLocked() bool {
	if h.closed || h.activeMatchLocked() {
		return false
	}
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick == "" || !h.seats[i].Connected || h.seats[i].Auto || !h.ready[i] {
			return false
		}
	}
	votes := 0
	for i := 0; i < 3; i++ {
		if h.allowVotes[i] {
			votes++
		}
	}
	spectatorCards := h.cfg.AllowSpectatorHands && votes >= 2
	h.seq++
	h.match = &Match{
		ID: h.seq, Phase: "countdown", Turn: -1, Landlord: -1, Caller: -1, BidTurn: -1,
		RobMultiplier: 1, SpectatorCards: spectatorCards,
	}
	h.armTimerLocked(h.cfg.CountdownSeconds)
	h.match.CountdownEnd = h.match.Deadline
	return true
}

func (h *Hub) cancelCountdownLocked() {
	if h.match == nil || h.match.Phase != "countdown" {
		return
	}
	if h.match.Timer != nil {
		h.match.Timer.Stop()
	}
	h.match = nil
}

// ============================================================
// 计时器
// ============================================================

func (h *Hub) armTimerLocked(seconds int) {
	if h.match == nil {
		return
	}
	if seconds < 1 {
		seconds = 1
	}
	if h.match.Timer != nil {
		h.match.Timer.Stop()
	}
	h.match.TurnSeq++
	seq := h.match.TurnSeq
	id := h.match.ID
	h.match.Deadline = time.Now().Add(time.Duration(seconds) * time.Second).UnixMilli()
	h.match.Timer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		h.onTimer(id, seq)
	})
}

func (h *Hub) armTurnLocked(seconds int) {
	if h.match == nil {
		return
	}
	if h.match.Turn >= 0 && h.match.Turn < 3 {
		s := h.seats[h.match.Turn]
		if s.Auto || !s.Connected {
			if seconds > autoActSeconds {
				seconds = autoActSeconds
			}
		}
	}
	h.armTimerLocked(seconds)
}

func (h *Hub) onTimer(id, seq int64) {
	h.lock.Lock()
	m := h.match
	if m == nil || m.ID != id || m.TurnSeq != seq {
		h.lock.Unlock()
		return
	}
	switch m.Phase {
	case "countdown":
		h.beginDealLocked()
	case "bidding":
		h.autoBidLocked()
	case "doubling":
		h.autoDoubleLocked()
	case "playing":
		h.autoPlayLocked()
	}
	h.lock.Unlock()
	h.syncAll()
}

// ============================================================
// 发牌 / 叫抢 / 加倍
// ============================================================

func (h *Hub) beginDealLocked() {
	m := h.match
	if m == nil || m.Phase != "countdown" {
		return
	}
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick == "" || !h.seats[i].Connected || h.seats[i].Auto || !h.ready[i] {
			h.cancelCountdownLocked()
			return
		}
	}
	hands, bottom := rules.Deal()
	m.Hands = hands
	m.Bottom = bottom
	m.Phase = "bidding"
	// 投票只对刚开的这一局生效；下一局默认全部勾选“允许观战”。
	h.allowVotes = [3]bool{true, true, true}
	m.StartAt = time.Now().UnixMilli()
	m.CountdownEnd = 0
	m.BidStage = "call"
	m.Caller = -1
	m.Landlord = -1
	m.RobMultiplier = 1
	m.RobCount = 0
	m.RobQueue = nil
	m.FinalChance = false
	m.BidActions = [3]string{}
	m.Double = [3]bool{}
	m.DoubleDone = [3]bool{}
	m.DoubleActions = [3]string{}
	m.LastPlay = nil
	m.LastPlayer = -1
	m.PassCount = 0
	m.LastActions = [3]*ActionInfo{}
	m.PlayCount = [3]int{}
	m.BombCount = 0
	m.Result = nil
	m.WinnerSide = ""
	for i := 0; i < 3; i++ {
		h.ready[i] = false
	}
	m.BidTurn = rand.Intn(3)
	m.Turn = m.BidTurn
	h.armTimerLocked(h.cfg.BidSeconds)
	log.Printf("[match #%d] 发牌完成，首叫 %s", m.ID, seatAt(m.BidTurn))
}

func (h *Hub) bid(nick, action string) string {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return "先上座"
	}
	err := h.bidLocked(seat, action)
	h.lock.Unlock()
	if err == "" {
		h.syncAll()
	}
	return err
}

func (h *Hub) bidLocked(seat int, action string) string {
	m := h.match
	if m == nil || m.Phase != "bidding" {
		return "现在不是叫地主阶段"
	}
	if seat != m.BidTurn {
		return "还没轮到你"
	}
	switch m.BidStage {
	case "call":
		if action != "call" && action != "pass" {
			return "请选择叫地主或不叫"
		}
		m.BidActions[seat] = action
		if action == "call" {
			m.Caller = seat
			m.Landlord = seat
			// 进入抢地主阶段后，清掉叫地主阶段的“不叫”状态，
			// 否则还没轮到抢的人会提前显示成“不抢”。
			m.BidActions = [3]string{}
			m.BidActions[seat] = "call"
			m.BidStage = "rob"
			m.RobCount = 0
			m.FinalChance = false
			m.RobQueue = []int{nextIndex(seat), nextIndex(nextIndex(seat))}
			m.BidTurn = m.RobQueue[0]
			m.Turn = m.BidTurn
			h.armTimerLocked(h.cfg.BidSeconds)
			return ""
		}
		passes := 0
		for i := 0; i < 3; i++ {
			if m.BidActions[i] == "pass" {
				passes++
			}
		}
		if passes >= 3 {
			m.RedealCount++
			// 连续 3 次无人叫地主后，随机指定地主；第 3 次流局就应该结束叫抢。
			if m.RedealCount >= 3 {
				m.Landlord = rand.Intn(3)
				m.Caller = m.Landlord
				m.RobMultiplier = 1
				h.startDoublingLocked()
				return ""
			}
			hands, bottom := rules.Deal()
			m.Hands = hands
			m.Bottom = bottom
			m.BidActions = [3]string{}
			m.RobMultiplier = 1
			m.BidTurn = rand.Intn(3)
			m.Turn = m.BidTurn
			h.armTimerLocked(h.cfg.BidSeconds)
			return ""
		}
		m.BidTurn = nextIndex(m.BidTurn)
		m.Turn = m.BidTurn
		h.armTimerLocked(h.cfg.BidSeconds)
		return ""
	case "rob":
		if action != "rob" && action != "pass" {
			return "请选择抢地主或不抢"
		}
		m.BidActions[seat] = action
		if action == "rob" {
			m.Landlord = seat
			m.RobMultiplier *= 2
			m.RobCount++
		}
		if len(m.RobQueue) > 0 && m.RobQueue[0] == seat {
			m.RobQueue = m.RobQueue[1:]
		}
		if m.RobCount >= 3 {
			h.startDoublingLocked()
			return ""
		}
		if len(m.RobQueue) == 0 {
			// 只要有人抢过地主，原叫地主者就有一次反抢机会；
			// 之前只在另外两家都抢时才给反抢，只有一家抢会直接结束。
			if m.RobCount > 0 && !m.FinalChance {
				m.FinalChance = true
				m.RobQueue = []int{m.Caller}
			} else {
				h.startDoublingLocked()
				return ""
			}
		}
		m.BidTurn = m.RobQueue[0]
		m.Turn = m.BidTurn
		h.armTimerLocked(h.cfg.BidSeconds)
		return ""
	default:
		return "叫地主阶段异常"
	}
}

func (h *Hub) autoBidLocked() {
	m := h.match
	if m == nil || m.Phase != "bidding" || m.BidTurn < 0 {
		return
	}
	_ = h.bidLocked(m.BidTurn, "pass")
}

func (h *Hub) startDoublingLocked() {
	m := h.match
	if m == nil {
		return
	}
	if m.Landlord < 0 {
		m.Landlord = rand.Intn(3)
	}
	// 地主拿底牌
	m.Hands[m.Landlord] = append(m.Hands[m.Landlord], m.Bottom...)
	rules.SortCards(m.Hands[m.Landlord])
	m.Phase = "doubling"
	m.Double = [3]bool{}
	m.DoubleDone = [3]bool{}
	m.DoubleActions = [3]string{}
	m.Turn = -1
	h.armTimerLocked(h.cfg.DoubleSeconds)
	// 已经掉线/托管的座位不用等读条。
	anyPending := false
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick == "" {
			continue
		}
		if h.seats[i].Auto || !h.seats[i].Connected {
			m.DoubleDone[i] = true
			m.DoubleActions[i] = "pass"
		} else if !m.DoubleDone[i] {
			anyPending = true
		}
	}
	if !anyPending {
		h.startPlayingLocked()
	}
	log.Printf("[match #%d] 地主 %s，倍数 x%d", m.ID, seatAt(m.Landlord), m.RobMultiplier)
}

func (h *Hub) double(nick string, value bool) string {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return "先上座"
	}
	err := h.doubleLocked(seat, value)
	h.lock.Unlock()
	if err == "" {
		h.syncAll()
	}
	return err
}

func (h *Hub) doubleLocked(seat int, value bool) string {
	m := h.match
	if m == nil || m.Phase != "doubling" {
		return "现在不是加倍阶段"
	}
	if m.DoubleDone[seat] {
		return "已经选择过了"
	}
	m.Double[seat] = value
	m.DoubleDone[seat] = true
	if value {
		m.DoubleActions[seat] = "double"
	} else {
		m.DoubleActions[seat] = "pass"
	}
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick != "" && !m.DoubleDone[i] {
			return ""
		}
	}
	h.startPlayingLocked()
	return ""
}

func (h *Hub) autoDoubleLocked() {
	m := h.match
	if m == nil || m.Phase != "doubling" {
		return
	}
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick != "" && !m.DoubleDone[i] {
			m.DoubleDone[i] = true
			m.DoubleActions[i] = "pass"
		}
	}
	h.startPlayingLocked()
}

func (h *Hub) startPlayingLocked() {
	m := h.match
	if m == nil {
		return
	}
	m.Phase = "playing"
	m.Turn = m.Landlord
	m.LastPlay = nil
	m.LastPlayer = -1
	m.PassCount = 0
	m.LastActions = [3]*ActionInfo{}
	m.PlayCount = [3]int{}
	m.BombCount = 0
	h.armTurnLocked(h.cfg.TurnSeconds)
	log.Printf("[match #%d] 出牌开始，地主 %s", m.ID, seatAt(m.Landlord))
}

// ============================================================
// 出牌
// ============================================================

func (h *Hub) play(nick string, ids []int) string {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return "先上座"
	}
	cards, ok := rules.CardsByIDs(ids)
	if !ok {
		h.lock.Unlock()
		return "牌数据不合法"
	}
	err := h.playLocked(seat, cards)
	h.lock.Unlock()
	if err == "" {
		h.syncAll()
	}
	return err
}

func (h *Hub) playLocked(seat int, cards []rules.Card) string {
	m := h.match
	if m == nil || m.Phase != "playing" {
		return "现在不能出牌"
	}
	if seat != m.Turn {
		return "还没轮到你"
	}
	if len(cards) == 0 {
		return "请选择要出的牌"
	}
	combo, ok := rules.Parse(cards)
	if !ok {
		return "牌型不合法"
	}
	if !rules.ContainsAll(m.Hands[seat], cards) {
		return "手牌里没有这些牌"
	}
	if m.LastPlay != nil && !combo.Beats(m.LastPlay.Combo) {
		return "压不过上一手"
	}
	next, ok := rules.RemoveCards(m.Hands[seat], cards)
	if !ok {
		return "手牌里没有这些牌"
	}
	m.Hands[seat] = next
	m.PlayCount[seat]++
	if combo.Type == rules.Bomb || combo.Type == rules.Rocket {
		m.BombCount++
	}
	m.LastPlay = &PlayInfo{Seat: seat, Cards: append([]rules.Card(nil), cards...), Combo: combo}
	m.LastPlayer = seat
	m.PassCount = 0
	m.LastActions[seat] = &ActionInfo{Seat: seat, Cards: append([]rules.Card(nil), cards...), Combo: &combo}
	if len(m.Hands[seat]) == 0 {
		h.finishRoundLocked(seat)
		return ""
	}
	m.Turn = nextIndex(seat)
	h.armTurnLocked(h.cfg.TurnSeconds)
	return ""
}

func (h *Hub) pass(nick string) string {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return "先上座"
	}
	err := h.passLocked(seat)
	h.lock.Unlock()
	if err == "" {
		h.syncAll()
	}
	return err
}

func (h *Hub) passLocked(seat int) string {
	m := h.match
	if m == nil || m.Phase != "playing" {
		return "现在不能操作"
	}
	if seat != m.Turn {
		return "还没轮到你"
	}
	if m.LastPlay == nil {
		return "你是首出，必须出牌"
	}
	m.LastActions[seat] = &ActionInfo{Seat: seat, Pass: true}
	m.PassCount++
	if m.PassCount >= 2 {
		m.Turn = m.LastPlayer
		m.LastPlay = nil
		m.PassCount = 0
		m.LastActions = [3]*ActionInfo{}
	} else {
		m.Turn = nextIndex(seat)
	}
	h.armTurnLocked(h.cfg.TurnSeconds)
	return ""
}

func (h *Hub) autoPlayLocked() {
	m := h.match
	if m == nil || m.Phase != "playing" || m.Turn < 0 {
		return
	}
	seat := m.Turn
	if len(m.Hands[seat]) == 0 {
		h.finishRoundLocked(seat)
		return
	}
	if m.LastPlay == nil {
		// 首出：系统出最小单张。
		_ = h.playLocked(seat, []rules.Card{m.Hands[seat][0]})
		return
	}
	// 掉线/主动离开的托管：能压则出最小可压牌，否则过。
	if h.seats[seat].Auto || !h.seats[seat].Connected {
		last := m.LastPlay.Combo
		hints := rules.Hints(m.Hands[seat], &last)
		if len(hints) > 0 {
			_ = h.playLocked(seat, hints[0].Cards)
			return
		}
	}
	_ = h.passLocked(seat)
}

// hint 返回能压过当前牌型的候选，供前端循环选中。
func (h *Hub) hint(nick string) {
	h.lock.Lock()
	seat := h.controlledSeatLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		h.sendToNick(nick, map[string]interface{}{"type": "error", "message": "先上座"})
		return
	}
	m := h.match
	if m == nil || m.Phase != "playing" || seat != m.Turn {
		h.lock.Unlock()
		h.sendToNick(nick, map[string]interface{}{"type": "error", "message": "还没轮到你"})
		return
	}
	var last *rules.Combo
	if m.LastPlay != nil {
		c := m.LastPlay.Combo
		last = &c
	}
	plays := rules.Hints(m.Hands[seat], last)
	h.lock.Unlock()
	h.sendToNick(nick, map[string]interface{}{
		"type":  "hint",
		"plays": plays,
	})
}

func (h *Hub) watch(nick, seatName string) {
	h.lock.Lock()
	c := h.clients[nick]
	if c == nil {
		h.lock.Unlock()
		return
	}
	if h.controlledSeatLocked(nick) >= 0 {
		h.lock.Unlock()
		return
	}
	idx := seatIndex(seatName)
	if idx < 0 {
		idx = -1
	}
	c.watchSeat = idx
	data := h.handPayloadLocked(c, time.Now())
	h.lock.Unlock()
	c.trySend(data)
	// 观众切换跟随对象后，通知所有客户端把该观众的 avatar 放到对应选手卡片上。
	h.pushState()
}

func (h *Hub) sendToNick(nick string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.lock.Lock()
	c := h.clients[nick]
	h.lock.Unlock()
	if c != nil {
		c.trySend(data)
	}
}

// ============================================================
// 结算
// ============================================================

func (h *Hub) finishRoundLocked(winnerSeat int) {
	m := h.match
	if m == nil || m.Phase == "over" {
		return
	}
	landlordWon := winnerSeat == m.Landlord
	var balances [3]int
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick != "" {
			balances[i] = h.store.Points(h.seats[i].Nick)
		}
	}
	spring := false
	antiSpring := false
	if landlordWon {
		spring = true
		for i := 0; i < 3; i++ {
			if i != m.Landlord && m.PlayCount[i] > 0 {
				spring = false
			}
		}
	} else if m.PlayCount[m.Landlord] <= 1 {
		antiSpring = true
	}
	out := rules.Settle(rules.ScoreInput{
		Base:           h.cfg.BaseScore,
		RobMultiplier:  m.RobMultiplier,
		BombCount:      m.BombCount,
		Spring:         spring,
		AntiSpring:     antiSpring,
		Landlord:       m.Landlord,
		LandlordDouble: m.Double[m.Landlord],
		Doubles:        m.Double,
		Balance:        balances,
		LandlordWon:    landlordWon,
		RakePercent:    h.cfg.RakePercent,
		RakeMinProfit:  h.cfg.RakeMinProfit,
		RakeCap:        h.cfg.RakeCap,
	})

	deltas := map[string]int{}
	win := map[string]bool{}
	avatars := map[string]string{}
	players := make([]PlayerResult, 0, 3)
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick == "" {
			continue
		}
		role := "farmer"
		if i == m.Landlord {
			role = "landlord"
		}
		won := (landlordWon && i == m.Landlord) || (!landlordWon && i != m.Landlord)
		np := balances[i] + out.Deltas[i]
		if np < 0 {
			np = 0
		}
		h.seats[i].Points = np
		deltas[h.seats[i].Nick] = out.Deltas[i]
		win[h.seats[i].Nick] = won
		avatars[h.seats[i].Nick] = h.seats[i].Avatar
		players = append(players, PlayerResult{
			Seat: seatAt(i), Nick: h.seats[i].Nick, Avatar: h.seats[i].Avatar,
			Role: role, Double: m.Double[i], Delta: out.Deltas[i], Points: np, Rake: out.Rakes[i],
		})
	}
	h.store.ApplyResult(deltas, win, avatars)

	// 每日活跃奖励：当天完成过一局且余额达到最低上桌积分，自动发放一次。
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick == "" {
			continue
		}
		if bonus, ok := h.store.ClaimActiveReward(h.seats[i].Nick); ok {
			h.seats[i].Points = h.store.Points(h.seats[i].Nick)
			for j := range players {
				if players[j].Seat == seatAt(i) {
					players[j].Points = h.seats[i].Points
					players[j].DailyBonus = bonus
					break
				}
			}
		}
	}

	if m.Timer != nil {
		m.Timer.Stop()
		m.Timer = nil
	}
	m.Phase = "over"
	m.Turn = -1
	m.Deadline = 0
	m.PublicMultiplier = out.PublicMultiplier
	m.WinnerSide = "farmers"
	if landlordWon {
		m.WinnerSide = "landlord"
	}
	if out.Rake > 0 {
		log.Printf("[match #%d] 抽水 %d 分（赢家正收益 %d%%）", m.ID, out.Rake, h.cfg.RakePercent)
	}
	m.Result = &ResultInfo{
		WinnerSide: m.WinnerSide, Landlord: seatAt(m.Landlord),
		Spring: spring, AntiSpring: antiSpring, BombCount: m.BombCount,
		RobMultiplier: m.RobMultiplier, PublicMultiplier: out.PublicMultiplier,
		Base: h.cfg.BaseScore, Rake: out.Rake, Players: players, Reason: "normal",
	}
	for i := 0; i < 3; i++ {
		h.ready[i] = false
	}
	// 本局结束后，托管/掉线座位释放。
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick != "" && (h.seats[i].Auto || !h.seats[i].Connected) {
			h.seats[i] = SeatInfo{}
			h.ready[i] = false
			h.allowVotes[i] = false
		}
	}
	winnerText := "农民获胜"
	if landlordWon {
		winnerText = "地主获胜"
	}
	log.Printf("[match #%d] 结算完成，phase=%s result=%v", m.ID, m.Phase, m.Result != nil)
	log.Printf("[match #%d] 结束：%s，倍数 x%d（抢 x%d / 炸弹 %d / 春天 %v 反春 %v）",
		m.ID, winnerText, out.PublicMultiplier, m.RobMultiplier, m.BombCount, spring, antiSpring)
	h.pendingEvents = append(h.pendingEvents, "对局结束："+winnerText+"，倍数 x"+itoa(out.PublicMultiplier))
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
