// Package cards 提供扑克牌表示、洗牌与 5/6/7 张牌力评估。
package cards

import (
	"fmt"
	"math/rand"
	"strings"
)

// Rank 牌面大小，0=2 ... 12=Ace。
type Rank uint8

const (
	Two Rank = iota
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

// Suit 花色。
type Suit uint8

const (
	Spades Suit = iota
	Hearts
	Diamonds
	Clubs
)

// Card 单张牌，编码为 rank*4+suit，值本身无大小语义，比较用 Evaluate。
type Card uint8

// New 构造一张牌。
func New(r Rank, s Suit) Card { return Card(uint8(r)*4 + uint8(s)) }

// Rank 返回牌面。
func (c Card) Rank() Rank { return Rank(c / 4) }

// Suit 返回花色。
func (c Card) Suit() Suit { return Suit(c % 4) }

const rankChars = "23456789TJQKA"
const suitChars = "shdc"

// String 返回如 "As"、"Td" 的短表示。
func (c Card) String() string {
	return string([]byte{rankChars[c.Rank()], suitChars[c.Suit()]})
}

// Parse 解析 "As"、"td" 等短表示（大小写不敏感）。
func Parse(s string) (Card, error) {
	if len(s) != 2 {
		return 0, fmt.Errorf("cards: bad card %q", s)
	}
	r := strings.IndexByte(rankChars, toUpper(s[0]))
	su := strings.IndexByte(suitChars, toLower(s[0+1]))
	if r < 0 || su < 0 {
		return 0, fmt.Errorf("cards: bad card %q", s)
	}
	return New(Rank(r), Suit(su)), nil
}

// MustParse 同 Parse，失败时 panic，供测试/脚本场景使用。
func MustParse(s string) Card {
	c, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return c
}

// ParseList 解析空格分隔的牌列表，如 "As Kd Qc"。失败 panic。
func ParseList(s string) []Card {
	fields := strings.Fields(s)
	out := make([]Card, len(fields))
	for i, f := range fields {
		out[i] = MustParse(f)
	}
	return out
}

func toUpper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// Deck 返回一副新的 52 张有序牌。
func Deck() []Card {
	d := make([]Card, 52)
	for i := range d {
		d[i] = Card(i)
	}
	return d
}

// Shuffle 用调用方提供的 rng 原地洗牌，种子由调用方控制以保证可复现。
func Shuffle(d []Card, rng *rand.Rand) {
	rng.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
}
