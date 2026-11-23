package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"DoudizhuGame/server/rules"
	"github.com/BlackCarDriver/nichijouSDK"
)

func fakeClient(nick string) *client {
	return &client{nick: nick, send: make(chan []byte, 512), watchSeat: -1}
}

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{
		CountdownSeconds:    30,
		BidSeconds:          30,
		DoubleSeconds:       30,
		TurnSeconds:         30,
		SeatGraceSeconds:    30,
		AllowSpectatorHands: true,
		InitialPoints:       100,
		MinPointsToSit:      50,
		DailyActiveReward:   0,
		RakePercent:         0,
		BaseScore:           1,
		DataDir:             dir,
		BoardLimit:          100,
	}
	store := NewStore(filepath.Join(dir, "players.json"), cfg.InitialPoints, cfg.MinPointsToSit, cfg.DailyActiveReward)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	h := NewHub(cfg, store)
	for _, nick := range []string{"alice", "bob", "carol"} {
		h.addClient(fakeClient(nick))
	}
	t.Cleanup(func() {
		h.lock.Lock()
		if h.match != nil && h.match.Timer != nil {
			h.match.Timer.Stop()
		}
		for _, e := range h.grace {
			e.timer.Stop()
		}
		h.lock.Unlock()
	})
	return h
}

func startGame(t *testing.T, h *Hub) {
	t.Helper()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.sitAt(nick, ""); err != "" {
			t.Fatalf("sit %s: %s", nick, err)
		}
	}
	// 默认让两个座位投“允许观战”，保证涉及观众看牌的测试仍能看到手牌。
	h.lock.Lock()
	h.allowVotes[0] = true
	h.allowVotes[1] = true
	h.lock.Unlock()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.setReady(nick, true); err != "" {
			t.Fatalf("ready %s: %s", nick, err)
		}
	}
	h.lock.Lock()
	if h.match == nil || h.match.Phase != "countdown" {
		h.lock.Unlock()
		t.Fatal("三人准备后应进入 countdown")
	}
	h.beginDealLocked()
	h.lock.Unlock()
	h.syncAll()
	h.lock.Lock()
	if h.match.Phase != "bidding" {
		h.lock.Unlock()
		t.Fatalf("发牌后应进入 bidding，得到 %s", h.match.Phase)
	}
	for i := 0; i < 3; i++ {
		if len(h.match.Hands[i]) != 17 {
			h.lock.Unlock()
			t.Fatalf("座位 %d 手牌应为 17，得到 %d", i, len(h.match.Hands[i]))
		}
	}
	if len(h.match.Bottom) != 3 {
		h.lock.Unlock()
		t.Fatalf("底牌应为 3，得到 %d", len(h.match.Bottom))
	}
	h.lock.Unlock()
}

func bidFirstCallThenAllRob(t *testing.T, h *Hub) {
	t.Helper()
	h.lock.Lock()
	first := h.seats[h.match.BidTurn].Nick
	h.lock.Unlock()
	if err := h.bid(first, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}
	for i := 0; i < 10; i++ {
		h.lock.Lock()
		if h.match == nil || h.match.Phase != "bidding" {
			h.lock.Unlock()
			return
		}
		nick := h.seats[h.match.BidTurn].Nick
		h.lock.Unlock()
		if err := h.bid(nick, "rob"); err != "" {
			t.Fatalf("rob %s: %s", nick, err)
		}
	}
	t.Fatal("抢地主没有在合理次数内结束")
}

func bidFirstCallThenPass(t *testing.T, h *Hub) {
	t.Helper()
	h.lock.Lock()
	first := h.seats[h.match.BidTurn].Nick
	h.lock.Unlock()
	if err := h.bid(first, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}
	for i := 0; i < 10; i++ {
		h.lock.Lock()
		if h.match == nil || h.match.Phase != "bidding" {
			h.lock.Unlock()
			return
		}
		nick := h.seats[h.match.BidTurn].Nick
		h.lock.Unlock()
		if err := h.bid(nick, "pass"); err != "" {
			t.Fatalf("pass %s: %s", nick, err)
		}
	}
	t.Fatal("叫地主没有在合理次数内结束")
}

func doubleAll(t *testing.T, h *Hub) {
	t.Helper()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.double(nick, true); err != "" {
			t.Fatalf("double %s: %s", nick, err)
		}
	}
	h.lock.Lock()
	phase := h.match.Phase
	h.lock.Unlock()
	if phase != "playing" {
		t.Fatalf("三家加倍后应进入 playing，得到 %s", phase)
	}
}

func drainMessages(c *client) []map[string]interface{} {
	var out []map[string]interface{}
	for {
		select {
		case data := <-c.send:
			var msg map[string]interface{}
			if json.Unmarshal(data, &msg) == nil {
				out = append(out, msg)
			}
		default:
			return out
		}
	}
}

func findMessage(msgs []map[string]interface{}, typ string) map[string]interface{} {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i]["type"] == typ {
			return msgs[i]
		}
	}
	return nil
}

func TestSeatReadyDealAndRobMultiplier(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)

	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "doubling" {
		t.Fatalf("抢完应进入 doubling，得到 %s", h.match.Phase)
	}
	if h.match.RobMultiplier != 8 {
		t.Fatalf("三次抢地主后倍数应为 8，得到 %d", h.match.RobMultiplier)
	}
	if got := len(h.match.Hands[h.match.Landlord]); got != 20 {
		t.Fatalf("地主拿底牌后应有 20 张，得到 %d", got)
	}
}

func TestDoubleAndSettleWithSpring(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	landlord := h.match.Landlord
	landlordNick := h.seats[landlord].Nick
	if h.match.Turn != landlord {
		h.lock.Unlock()
		t.Fatalf("出牌开始时应由地主先出，turn=%d landlord=%d", h.match.Turn, landlord)
	}
	card := h.match.Hands[landlord][0]
	h.match.Hands[landlord] = []rules.Card{card}
	h.lock.Unlock()

	if err := h.play(landlordNick, []int{card.ID}); err != "" {
		t.Fatalf("地主出最后一张失败：%s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "over" || h.match.Result == nil {
		t.Fatalf("出完牌应结算，phase=%s result=%v", h.match.Phase, h.match.Result)
	}
	r := h.match.Result
	if r.WinnerSide != "landlord" || !r.Spring {
		t.Fatalf("应地主春天获胜，得到 %+v", r)
	}
	// 抢地主 x8 × 春天 x2 = 16；三家都加倍，地主从每个农民拿 16*2*2=64。
	if r.PublicMultiplier != 16 {
		t.Fatalf("公共倍数应为 16，得到 %d", r.PublicMultiplier)
	}
	if r.Players[landlord].Delta != 128 {
		t.Fatalf("地主应 +128，得到 %d", r.Players[landlord].Delta)
	}
	for i, p := range r.Players {
		if i == landlord {
			if len(p.Remain) != 0 {
				t.Fatalf("地主已出完，剩余手牌应为空，得到 %+v", p.Remain)
			}
			continue
		}
		if p.Delta != -64 {
			t.Fatalf("农民应 -64，得到 %+v", p)
		}
		if len(p.Remain) != 17 {
			t.Fatalf("农民剩余手牌应为 17，得到 %d", len(p.Remain))
		}
	}
}

func TestHintPassAndBomb(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenPass(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	landlord := h.match.Landlord
	landlordNick := h.seats[landlord].Nick
	h.lock.Unlock()

	c := h.clients[landlordNick]
	if c == nil {
		t.Fatal("找不到地主连接")
	}
	drainMessages(c)
	h.hint(landlordNick)
	msgs := drainMessages(c)
	hm := findMessage(msgs, "hint")
	if hm == nil {
		t.Fatal("地主首出应收到 hint")
	}
	plays, _ := hm["plays"].([]interface{})
	if len(plays) == 0 {
		t.Fatal("首出提示不应为空")
	}

	// 首出最小单张，另外两家过，回合回到地主且 lastPlay 清空。
	playMap := plays[0].(map[string]interface{})
	cardList := playMap["cards"].([]interface{})
	ids := make([]int, 0, len(cardList))
	for _, it := range cardList {
		cm := it.(map[string]interface{})
		ids = append(ids, int(cm["id"].(float64)))
	}
	if err := h.play(landlordNick, ids); err != "" {
		t.Fatalf("提示出牌失败：%s", err)
	}
	h.lock.Lock()
	next1 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(next1); err != "" {
		t.Fatalf("pass1: %s", err)
	}
	h.lock.Lock()
	next2 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(next2); err != "" {
		t.Fatalf("pass2: %s", err)
	}
	h.lock.Lock()
	if h.match.Turn != landlord || h.match.LastPlay != nil {
		h.lock.Unlock()
		t.Fatalf("两家过牌后应回到地主首出，turn=%d last=%v", h.match.Turn, h.match.LastPlay)
	}
	// 换成四张 3 + 一张 4，测试炸弹倍数和最后一手。
	bomb := []rules.Card{rules.CardByID(0), rules.CardByID(1), rules.CardByID(2), rules.CardByID(3)}
	last := rules.CardByID(4)
	h.match.Hands[landlord] = append(append([]rules.Card{}, bomb...), last)
	h.lock.Unlock()
	if err := h.play(landlordNick, []int{0, 1, 2, 3}); err != "" {
		t.Fatalf("炸弹出牌失败：%s", err)
	}
	h.lock.Lock()
	if h.match.BombCount != 1 {
		h.lock.Unlock()
		t.Fatalf("炸弹计数应为 1，得到 %d", h.match.BombCount)
	}
	n1 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(n1); err != "" {
		t.Fatalf("pass after bomb 1: %s", err)
	}
	h.lock.Lock()
	n2 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(n2); err != "" {
		t.Fatalf("pass after bomb 2: %s", err)
	}
	if err := h.play(landlordNick, []int{last.ID}); err != "" {
		t.Fatalf("最后一手失败：%s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "over" || h.match.Result == nil {
		t.Fatal("出完炸弹和最后一手后应结算")
	}
	if h.match.Result.BombCount != 1 {
		t.Fatalf("结算炸弹数应为 1，得到 %d", h.match.Result.BombCount)
	}
	// 抢地主 x1 × 炸弹 x2 × 春天 x2 = 4
	if h.match.Result.PublicMultiplier != 4 {
		t.Fatalf("公共倍数应为 4，得到 %d", h.match.Result.PublicMultiplier)
	}
}

func TestSpectatorHandSwitch(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	dave := fakeClient("dave")
	h.addClient(dave)

	h.lock.Lock()
	data := h.handPayloadLocked(dave, time.Now())
	h.lock.Unlock()
	var hand map[string]interface{}
	if err := json.Unmarshal(data, &hand); err != nil {
		t.Fatal(err)
	}
	if hand["watching"] != true || hand["allowed"] != true {
		t.Fatalf("观众应能看牌并处于跟随状态，得到 %+v", hand)
	}
	if cards, _ := hand["cards"].([]interface{}); len(cards) == 0 {
		t.Fatal("观众应收到跟随座位的手牌")
	}

	h.lock.Lock()
	h.match.SpectatorCards = false
	data = h.handPayloadLocked(dave, time.Now())
	h.lock.Unlock()
	if err := json.Unmarshal(data, &hand); err != nil {
		t.Fatal(err)
	}
	if hand["allowed"] != false {
		t.Fatalf("关闭观众看牌后 allowed 应为 false，得到 %+v", hand)
	}
	if cards, _ := hand["cards"].([]interface{}); len(cards) != 0 {
		t.Fatalf("关闭观众看牌后不应下发手牌，得到 %d 张", len(cards))
	}
}

func TestChatAndEmotionOnlySeated(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	dave := fakeClient("dave")
	h.addClient(dave)

	alice := h.clients["alice"]
	drainMessages(alice)
	drainMessages(dave)

	h.BroadcastChat("alice", "hello", time.Now().Unix())
	if msg := findMessage(drainMessages(dave), "chat"); msg == nil || msg["seated"] != true {
		t.Fatal("上座玩家的聊天应广播，并标记 seated=true")
	}

	drainMessages(alice)
	drainMessages(dave)
	h.BroadcastChat("dave", "i am watching", time.Now().Unix())
	if msg := findMessage(drainMessages(dave), "chat"); msg == nil || msg["seated"] != false {
		t.Fatal("观众聊天应进入聊天记录，并标记 seated=false")
	}

	drainMessages(alice)
	drainMessages(dave)
	h.BroadcastEmotion(nichijouSDK.Emotion{Nick: "alice", Text: "/哈哈", URL: "https://example.com/a.gif"})
	if msg := findMessage(drainMessages(dave), "emotion"); msg == nil || msg["seated"] != true {
		t.Fatal("上座玩家的表情应透传，并标记 seated=true")
	}

	drainMessages(alice)
	drainMessages(dave)
	h.BroadcastEmotion(nichijouSDK.Emotion{Nick: "dave", Text: "/哈哈", URL: "https://example.com/a.gif"})
	if msg := findMessage(drainMessages(dave), "emotion"); msg == nil || msg["seated"] != false {
		t.Fatal("观众表情应进入聊天记录，并标记 seated=false")
	}
}

func TestDisconnectMarksAutoAndGrace(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	h.lock.Lock()
	aliceSeat := h.seatOfNickLocked("alice")
	h.lock.Unlock()
	if aliceSeat < 0 {
		t.Fatal("alice 应在座位上")
	}
	alice := h.clients["alice"]
	h.removeClient("alice", alice)
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.seats[aliceSeat].Connected || !h.seats[aliceSeat].Auto {
		t.Fatalf("掉线后座位应标记为托管，得到 %+v", h.seats[aliceSeat])
	}
	if h.match == nil || h.match.Phase == "over" {
		t.Fatal("对局中掉线不应中止对局")
	}
}

func TestCancelReadyDuringCountdown(t *testing.T) {
	h := newTestHub(t)
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.sitAt(nick, ""); err != "" {
			t.Fatal(err)
		}
	}
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.setReady(nick, true); err != "" {
			t.Fatal(err)
		}
	}
	h.lock.Lock()
	if h.match == nil || h.match.Phase != "countdown" {
		h.lock.Unlock()
		t.Fatal("三人准备后应进入倒数")
	}
	h.lock.Unlock()
	if err := h.setReady("alice", false); err != "" {
		t.Fatalf("倒数期间取消准备应允许：%s", err)
	}
	h.lock.Lock()
	if h.match != nil {
		h.lock.Unlock()
		t.Fatal("取消准备后应取消开局")
	}
	h.lock.Unlock()
}

func TestVoteSpectatorThreshold(t *testing.T) {
	h := newTestHub(t)
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.sitAt(nick, ""); err != "" {
			t.Fatal(err)
		}
	}
	h.lock.Lock()
	h.allowVotes = [3]bool{true, false, false}
	h.lock.Unlock()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.setReady(nick, true); err != "" {
			t.Fatal(err)
		}
	}
	h.lock.Lock()
	if h.match == nil || h.match.Phase != "countdown" {
		h.lock.Unlock()
		t.Fatal("应进入 countdown")
	}
	if h.match.SpectatorCards {
		h.lock.Unlock()
		t.Fatal("只有 1 票时不应允许观众看牌")
	}
	h.lock.Unlock()
	// 取消倒数后重新投票到 2 票
	if err := h.setReady("alice", false); err != "" {
		t.Fatal(err)
	}
	h.lock.Lock()
	h.allowVotes = [3]bool{true, true, false}
	h.lock.Unlock()
	if err := h.setReady("alice", true); err != "" {
		t.Fatal(err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match == nil || h.match.Phase != "countdown" {
		t.Fatal("应再次进入 countdown")
	}
	if !h.match.SpectatorCards {
		t.Fatal("2 票时应允许观众看牌")
	}
}

func TestMatchViewCanBeat(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	h.lock.Lock()
	m := h.match
	m.Phase = "playing"
	m.Turn = 0
	m.Landlord = 0
	m.Hands[0] = []rules.Card{rules.CardByID(0), rules.CardByID(4)}
	m.LastPlay = &PlayInfo{Seat: 1, Combo: rules.Combo{Type: rules.Single, Main: rules.BigJoker, Len: 1}}
	view := h.matchViewLocked(time.Now())
	h.lock.Unlock()
	if view["canBeat"] != false {
		t.Fatal("手里没有能压过大王的牌，canBeat 应为 false")
	}
	h.lock.Lock()
	m.LastPlay = &PlayInfo{Seat: 1, Combo: rules.Combo{Type: rules.Single, Main: 3, Len: 1}}
	view = h.matchViewLocked(time.Now())
	h.lock.Unlock()
	if view["canBeat"] != true {
		t.Fatal("手里有 4，能压单 3，canBeat 应为 true")
	}
}

func TestCallerGetsFinalChanceAfterOneRob(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)

	h.lock.Lock()
	caller := h.match.BidTurn
	callerNick := h.seats[caller].Nick
	h.lock.Unlock()
	if err := h.bid(callerNick, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}

	h.lock.Lock()
	firstRobber := h.match.BidTurn
	firstRobberNick := h.seats[firstRobber].Nick
	h.lock.Unlock()
	if err := h.bid(firstRobberNick, "rob"); err != "" {
		t.Fatalf("first rob: %s", err)
	}

	h.lock.Lock()
	secondNick := h.seats[h.match.BidTurn].Nick
	h.lock.Unlock()
	if err := h.bid(secondNick, "pass"); err != "" {
		t.Fatalf("second pass: %s", err)
	}

	h.lock.Lock()
	ok := h.match.Phase == "bidding" && h.match.BidStage == "rob" && h.match.BidTurn == caller
	phase, stage, turn := h.match.Phase, h.match.BidStage, h.match.BidTurn
	h.lock.Unlock()
	if !ok {
		t.Fatalf("只有一家抢地主后，应回到原叫地主者反抢，phase=%s stage=%s turn=%d caller=%d", phase, stage, turn, caller)
	}

	if err := h.bid(callerNick, "rob"); err != "" {
		t.Fatalf("caller final rob: %s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "doubling" {
		t.Fatalf("原叫地主者反抢后应进入 doubling，得到 %s", h.match.Phase)
	}
	if h.match.Landlord != caller {
		t.Fatalf("原叫地主者反抢后应成为地主，得到 %d", h.match.Landlord)
	}
	if h.match.RobMultiplier != 4 {
		t.Fatalf("一家抢 + 原叫地主者反抢，倍数应为 4，得到 %d", h.match.RobMultiplier)
	}
	if got := len(h.match.Hands[caller]); got != 20 {
		t.Fatalf("地主拿底牌后应有 20 张，得到 %d", got)
	}
}

func TestCallPassBecomesRobPassWhenRobStarts(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)

	h.lock.Lock()
	first := h.match.BidTurn
	firstNick := h.seats[first].Nick
	h.lock.Unlock()
	if err := h.bid(firstNick, "pass"); err != "" {
		t.Fatalf("first pass: %s", err)
	}

	h.lock.Lock()
	caller := h.match.BidTurn
	callerNick := h.seats[caller].Nick
	h.lock.Unlock()
	if err := h.bid(callerNick, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}

	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.BidStage != "rob" {
		t.Fatalf("叫地主后应进入抢地主，得到 %s", h.match.BidStage)
	}
	if h.match.BidActions[first] != "pass" {
		t.Fatalf("叫地主阶段不叫的人进入抢地主后应直接记为不抢，得到 %q", h.match.BidActions[first])
	}
	if h.match.BidActions[caller] != "call" {
		t.Fatalf("原叫地主者应保留叫地主标记，得到 %q", h.match.BidActions[caller])
	}
}

func TestRedealAfterThreeNoCalls(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)

	for round := 0; round < 3; round++ {
		for i := 0; i < 3; i++ {
			h.lock.Lock()
			if h.match == nil || h.match.Phase != "bidding" {
				phase := ""
				if h.match != nil {
					phase = h.match.Phase
				}
				h.lock.Unlock()
				t.Fatalf("第 %d 轮第 %d 次叫地主前阶段异常：%s", round+1, i+1, phase)
			}
			nick := h.seats[h.match.BidTurn].Nick
			h.lock.Unlock()
			if err := h.bid(nick, "pass"); err != "" {
				t.Fatalf("第 %d 轮第 %d 次 pass：%s", round+1, i+1, err)
			}
		}
	}

	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "doubling" {
		t.Fatalf("连续 3 次流局后应随机指定地主并进入 doubling，得到 %s", h.match.Phase)
	}
	if h.match.Landlord < 0 {
		t.Fatal("随机指定地主失败")
	}
	if h.match.RobMultiplier != 1 {
		t.Fatalf("流局指定地主时倍数应为 1，得到 %d", h.match.RobMultiplier)
	}
	if got := len(h.match.Hands[h.match.Landlord]); got != 20 {
		t.Fatalf("地主拿底牌后应有 20 张，得到 %d", got)
	}
}

func TestSitAutoGrantsRelief(t *testing.T) {
	h := newTestHub(t)
	h.store.ApplyResult(
		map[string]int{"alice": -100},
		map[string]bool{"alice": false},
		map[string]string{"alice": ""},
	)
	if err := h.sitAt("alice", "a"); err != "" {
		t.Fatalf("余额不足时上桌应自动领取救济，得到 %s", err)
	}
	h.lock.Lock()
	seat := h.seatOfNickLocked("alice")
	points := h.seats[seat].Points
	h.lock.Unlock()
	if seat != 0 || points != 50 {
		t.Fatalf("应自动补到 50 后坐在 a，得到 seat=%d points=%d", seat, points)
	}
}

func TestReadyAutoGrantsRelief(t *testing.T) {
	h := newTestHub(t)
	if err := h.sitAt("alice", "a"); err != "" {
		t.Fatalf("sit: %s", err)
	}
	h.store.ApplyResult(
		map[string]int{"alice": -100},
		map[string]bool{"alice": false},
		map[string]string{"alice": ""},
	)
	if err := h.setReady("alice", true); err != "" {
		t.Fatalf("准备时应自动领取救济，得到 %s", err)
	}
	h.lock.Lock()
	seat := h.seatOfNickLocked("alice")
	points := h.seats[seat].Points
	ready := h.ready[seat]
	h.lock.Unlock()
	if points != 50 || !ready {
		t.Fatalf("应补到 50 且已准备，得到 points=%d ready=%v", points, ready)
	}
}

func TestSettleAppliesRakeAndDailyBonus(t *testing.T) {
	h := newTestHub(t)
	h.cfg.RakePercent = 5
	h.cfg.RakeMinProfit = 10
	h.cfg.RakeCap = 30
	h.store.dailyActiveReward = 10
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	landlord := h.match.Landlord
	landlordNick := h.seats[landlord].Nick
	card := h.match.Hands[landlord][0]
	h.match.Hands[landlord] = []rules.Card{card}
	h.lock.Unlock()

	if err := h.play(landlordNick, []int{card.ID}); err != "" {
		t.Fatalf("地主出最后一张失败：%s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Result == nil {
		t.Fatal("缺少结算结果")
	}
	r := h.match.Result
	if r.Rake != 7 {
		t.Fatalf("地主赢 128 抽 5%% 应为 7，得到 %d", r.Rake)
	}
	for _, p := range r.Players {
		if p.Seat == seatAt(landlord) {
			if p.Delta != 121 {
				t.Fatalf("地主扣除抽水后应为 +121，得到 %+v", p)
			}
			if p.DailyBonus != 10 || p.Points != 231 {
				t.Fatalf("地主活跃奖励/最终积分错误：%+v", p)
			}
		}
	}
}

func TestTurnStartClearsPreviousAction(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	m := h.match
	landlord := m.Landlord
	next := nextIndex(landlord)
	m.Hands[landlord] = []rules.Card{rules.CardByID(0), rules.CardByID(4)}
	m.LastActions[next] = &ActionInfo{Seat: next, Pass: true}
	card := m.Hands[landlord][0]
	nick := h.seats[landlord].Nick
	h.lock.Unlock()

	if err := h.play(nick, []int{card.ID}); err != "" {
		t.Fatalf("play: %s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.LastActions[next] != nil {
		t.Fatalf("轮到下一家时，应清空其上一轮动作，得到 %+v", h.match.LastActions[next])
	}
}

func TestCallPasserCannotRob(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)

	h.lock.Lock()
	first := h.match.BidTurn
	firstNick := h.seats[first].Nick
	h.lock.Unlock()
	if err := h.bid(firstNick, "pass"); err != "" {
		t.Fatalf("first pass: %s", err)
	}

	h.lock.Lock()
	caller := h.match.BidTurn
	callerNick := h.seats[caller].Nick
	h.lock.Unlock()
	if err := h.bid(callerNick, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}

	h.lock.Lock()
	if h.match.Phase != "bidding" || h.match.BidStage != "rob" {
		phase, stage := h.match.Phase, h.match.BidStage
		h.lock.Unlock()
		t.Fatalf("应进入抢地主，得到 %s/%s", phase, stage)
	}
	if h.match.BidTurn == first {
		h.lock.Unlock()
		t.Fatal("叫地主阶段不叫的人不应再获得抢地主回合")
	}
	if h.match.BidActions[first] != "pass" {
		got := h.match.BidActions[first]
		h.lock.Unlock()
		t.Fatalf("不叫的人应直接记为不抢，得到 %q", got)
	}
	h.lock.Unlock()
}

func TestCallPasserAndOtherPassEndsBidding(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)

	h.lock.Lock()
	first := h.match.BidTurn
	firstNick := h.seats[first].Nick
	h.lock.Unlock()
	if err := h.bid(firstNick, "pass"); err != "" {
		t.Fatalf("first pass: %s", err)
	}
	h.lock.Lock()
	caller := h.match.BidTurn
	callerNick := h.seats[caller].Nick
	h.lock.Unlock()
	if err := h.bid(callerNick, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}
	h.lock.Lock()
	thirdNick := h.seats[h.match.BidTurn].Nick
	h.lock.Unlock()
	if err := h.bid(thirdNick, "pass"); err != "" {
		t.Fatalf("third pass: %s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "doubling" {
		t.Fatalf("不叫的人被跳过且另一家不抢后应进入 doubling，得到 %s", h.match.Phase)
	}
	if h.match.RobMultiplier != 1 {
		t.Fatalf("无人抢地主时倍数应为 1，得到 %d", h.match.RobMultiplier)
	}
}

func TestFinishedMatchClearedWhenRoomEmpty(t *testing.T) {
	h := newTestHub(t)
	h.lock.Lock()
	h.match = &Match{
		ID:     99,
		Phase:  "over",
		Result: &ResultInfo{WinnerSide: "landlord", Landlord: "a"},
	}
	h.lock.Unlock()

	h.lock.Lock()
	clients := []struct {
		nick string
		c    *client
	}{
		{"alice", h.clients["alice"]},
		{"bob", h.clients["bob"]},
		{"carol", h.clients["carol"]},
	}
	h.lock.Unlock()
	for _, item := range clients {
		h.removeClient(item.nick, item.c)
	}

	h.lock.Lock()
	match := h.match
	h.lock.Unlock()
	if match != nil {
		t.Fatalf("空房间应清理已结束牌局，得到 %+v", match)
	}

	h.addClient(fakeClient("dave"))
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match != nil {
		t.Fatalf("新玩家进入后不应收到上一局结果，得到 %+v", h.match)
	}
	if got := h.snapshotLocked(time.Now())["match"]; got != nil {
		t.Fatalf("空房间新玩家的 state 不应包含旧 match，得到 %+v", got)
	}
}

func TestSitClearsFinishedMatch(t *testing.T) {
	h := newTestHub(t)
	h.lock.Lock()
	h.match = &Match{
		ID:     77,
		Phase:  "over",
		Result: &ResultInfo{WinnerSide: "landlord", Landlord: "a"},
		Hands:  [3][]rules.Card{{rules.CardByID(0)}, {rules.CardByID(1)}, {rules.CardByID(2)}},
	}
	h.lock.Unlock()
	if err := h.sitAt("alice", "a"); err != "" {
		t.Fatalf("sit: %s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match != nil {
		t.Fatalf("上坐后应清理上一局，得到 %+v", h.match)
	}
}

func TestBaseScoreTwoAndTableFee(t *testing.T) {
	h := newTestHub(t)
	h.cfg.BaseScore = 2
	h.cfg.TableFee = 1
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.double(nick, false); err != "" {
			t.Fatalf("no double %s: %s", nick, err)
		}
	}

	h.lock.Lock()
	landlord := h.match.Landlord
	landlordNick := h.seats[landlord].Nick
	card := h.match.Hands[landlord][0]
	h.match.Hands[landlord] = []rules.Card{card}
	h.lock.Unlock()
	if err := h.play(landlordNick, []int{card.ID}); err != "" {
		t.Fatalf("play: %s", err)
	}

	h.lock.Lock()
	defer h.lock.Unlock()
	r := h.match.Result
	if r == nil {
		t.Fatal("缺少结果")
	}
	if r.Base != 2 || r.TableFee != 3 {
		t.Fatalf("底分/桌费错误：base=%d tableFee=%d", r.Base, r.TableFee)
	}
	var landlordResult PlayerResult
	for _, p := range r.Players {
		if p.Seat == seatAt(landlord) {
			landlordResult = p
		} else if p.Delta != -33 {
			t.Fatalf("农民扣桌费后应 -33，得到 %+v", p)
		}
	}
	if landlordResult.Delta != 63 || landlordResult.Points != 163 {
		t.Fatalf("地主扣桌费后应 +63/163，得到 %+v", landlordResult)
	}
}

func TestReconnectFollowingSingleRejectsPair(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	alice := h.seats[h.seatOfNickLocked("alice")]
	h.match.Phase = "playing"
	h.match.Turn = h.seatOfNickLocked("alice")
	h.match.LastPlayer = h.seatOfNickLocked("bob")
	h.match.LastPlay = &PlayInfo{
		Seat:  h.seatOfNickLocked("bob"),
		Cards: []rules.Card{rules.CardByID(0)},
		Combo: rules.Combo{Type: rules.Single, Main: 3, Len: 1},
	}
	h.match.Hands[h.match.Turn] = []rules.Card{rules.CardByID(4), rules.CardByID(5)}
	aliceNick := alice.Nick
	h.lock.Unlock()

	// 模拟掉线后重新连接，LastPlay 仍是上一家的单张。
	h.lock.Lock()
	c := h.clients[aliceNick]
	h.lock.Unlock()
	if c != nil {
		h.removeClient(aliceNick, c)
	}
	h.addClient(fakeClient(aliceNick))

	if err := h.play(aliceNick, []int{4, 5}); err == "" {
		t.Fatal("上一手是单张时，重连后普通对子不应被允许")
	} else if err != "压不过上一手" {
		t.Fatalf("错误提示不对：%s", err)
	}
}

func TestRevealDoublesMultiplierAndSharesCards(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h) // 停在 doubling
	h.lock.Lock()
	seat := h.seatOfNickLocked("alice")
	before := h.match.RobMultiplier
	h.lock.Unlock()

	if err := h.reveal("alice", true); err != "" {
		t.Fatalf("reveal: %s", err)
	}
	h.lock.Lock()
	if !h.match.Reveal[seat] {
		h.lock.Unlock()
		t.Fatal("明牌状态未记录")
	}
	view := h.matchViewLocked(time.Now())
	seatView := h.seatViewLocked(seat).(map[string]interface{})
	h.lock.Unlock()
	if view["publicMultiplier"] != before*2 {
		t.Fatalf("明牌后公共倍数应为 %d，得到 %v", before*2, view["publicMultiplier"])
	}
	if seatView["revealed"] != true {
		t.Fatal("明牌玩家应标记 revealed")
	}
	if len(seatView["revealedCards"].([]rules.Card)) == 0 {
		t.Fatal("明牌玩家应公开手牌")
	}

	if err := h.reveal("alice", false); err != "" {
		t.Fatalf("cancel reveal: %s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Reveal[seat] {
		t.Fatal("取消明牌后状态应清除")
	}
	if got := h.matchViewLocked(time.Now())["publicMultiplier"]; got != before {
		t.Fatalf("取消明牌后倍数应恢复 %d，得到 %v", before, got)
	}
}
