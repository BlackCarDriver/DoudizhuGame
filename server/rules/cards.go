package rules

import (
	"crypto/rand"
	"math/big"
	"sort"
	"strconv"
)

// 牌点：3..15 表示 3..2，16 小王，17 大王。
const (
	Rank3      = 3
	Rank4      = 4
	Rank5      = 5
	Rank6      = 6
	Rank7      = 7
	Rank8      = 8
	Rank9      = 9
	Rank10     = 10
	RankJ      = 11
	RankQ      = 12
	RankK      = 13
	RankA      = 14
	Rank2      = 15
	SmallJoker = 16
	BigJoker   = 17
)

// 花色只用于展示，不参与大小比较。
const (
	SuitSpade   = 0
	SuitHeart   = 1
	SuitClub    = 2
	SuitDiamond = 3
	SuitJoker   = -1
)

// Card 一张牌。ID 在 0..53 之间，前端用它取图片。
//
//	0..51 : (rank-3)*4 + suit
//	52    : 小王
//	53    : 大王
type Card struct {
	ID   int `json:"id"`
	Rank int `json:"rank"`
	Suit int `json:"suit"`
}

// CardByID 由 ID 还原牌面。
func CardByID(id int) Card {
	switch id {
	case 52:
		return Card{ID: 52, Rank: SmallJoker, Suit: SuitJoker}
	case 53:
		return Card{ID: 53, Rank: BigJoker, Suit: SuitJoker}
	}
	if id < 0 || id > 51 {
		return Card{ID: id, Rank: 0, Suit: SuitJoker}
	}
	return Card{ID: id, Rank: id/4 + 3, Suit: id % 4}
}

// NewDeck 一副 54 张牌。
func NewDeck() []Card {
	deck := make([]Card, 0, 54)
	for id := 0; id < 54; id++ {
		deck = append(deck, CardByID(id))
	}
	return deck
}

// Shuffle Fisher-Yates，使用 crypto/rand；随机源失败时退回到一个确定但可用的顺序。
func Shuffle(cards []Card) {
	for i := len(cards) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			// 极端情况下不让游戏卡死：用下标交换继续。
			cards[i], cards[0] = cards[0], cards[i]
			continue
		}
		j := int(n.Int64())
		cards[i], cards[j] = cards[j], cards[i]
	}
}

// SortCards 按牌点升序、同点按花色排序；王在最后。
func SortCards(cards []Card) {
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].Rank != cards[j].Rank {
			return cards[i].Rank < cards[j].Rank
		}
		return cards[i].Suit < cards[j].Suit
	})
}

// Deal 洗牌并发牌：三人各 17 张，底牌 3 张。
func Deal() ([3][]Card, []Card) {
	deck := NewDeck()
	Shuffle(deck)
	var hands [3][]Card
	for i := 0; i < 51; i++ {
		hands[i%3] = append(hands[i%3], deck[i])
	}
	bottom := append([]Card(nil), deck[51:]...)
	for i := range hands {
		SortCards(hands[i])
	}
	SortCards(bottom)
	return hands, bottom
}

// RankName 仅用于日志/调试。
func RankName(rank int) string {
	switch rank {
	case RankJ:
		return "J"
	case RankQ:
		return "Q"
	case RankK:
		return "K"
	case RankA:
		return "A"
	case Rank2:
		return "2"
	case SmallJoker:
		return "小王"
	case BigJoker:
		return "大王"
	case 0:
		return "?"
	default:
		return strconv.Itoa(rank)
	}
}

// CardsByIDs 把客户端提交的 ID 列表还原成牌；任何一个 ID 非法都会失败。
func CardsByIDs(ids []int) ([]Card, bool) {
	out := make([]Card, 0, len(ids))
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id < 0 || id > 53 || seen[id] {
			return nil, false
		}
		seen[id] = true
		out = append(out, CardByID(id))
	}
	SortCards(out)
	return out, true
}

// ContainsAll 手牌是否包含 cards（按 ID 计数，防止重复提交同一张牌）。
func ContainsAll(hand, cards []Card) bool {
	count := make(map[int]int, len(hand))
	for _, c := range hand {
		count[c.ID]++
	}
	for _, c := range cards {
		count[c.ID]--
		if count[c.ID] < 0 {
			return false
		}
	}
	return true
}

// RemoveCards 从手牌里移除指定牌，返回剩余手牌和是否成功。
func RemoveCards(hand, cards []Card) ([]Card, bool) {
	if !ContainsAll(hand, cards) {
		return hand, false
	}
	remove := make(map[int]int, len(cards))
	for _, c := range cards {
		remove[c.ID]++
	}
	out := make([]Card, 0, len(hand)-len(cards))
	for _, c := range hand {
		if remove[c.ID] > 0 {
			remove[c.ID]--
			continue
		}
		out = append(out, c)
	}
	return out, true
}
