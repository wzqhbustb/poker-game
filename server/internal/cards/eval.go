package cards

import "fmt"

// Category 牌型类别，数值越大牌型越强。皇家同花顺归入 StraightFlush（最大的同花顺）。
type Category int

const (
	HighCard Category = iota
	Pair
	TwoPair
	Trips
	Straight
	Flush
	FullHouse
	Quads
	StraightFlush
)

func (c Category) String() string {
	switch c {
	case HighCard:
		return "high card"
	case Pair:
		return "pair"
	case TwoPair:
		return "two pair"
	case Trips:
		return "three of a kind"
	case Straight:
		return "straight"
	case Flush:
		return "flush"
	case FullHouse:
		return "full house"
	case Quads:
		return "four of a kind"
	case StraightFlush:
		return "straight flush"
	}
	return fmt.Sprintf("category(%d)", int(c))
}

// Score 可比较的牌力值：高 4 位为牌型类别，低 20 位为 5 个 4-bit 的踢脚（从大到小）。
// 值越大牌越强，直接用 < / > 比较即可。
type Score int32

// Category 返回牌力值对应的牌型类别。
func (s Score) Category() Category { return Category(s >> 20) }

// Compare 比较两个牌力值：a 强返回 1，相等 0，a 弱 -1。
func Compare(a, b Score) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	}
	return 0
}

// Evaluate 评估 5~7 张牌的最佳 5 张组合牌力。输入长度不在 [5,7] 时 panic。
func Evaluate(cs []Card) Score {
	switch len(cs) {
	case 5:
		return eval5(cs)
	case 6:
		best := Score(-1)
		for skip := 0; skip < 6; skip++ {
			var hand [5]Card
			k := 0
			for i := 0; i < 6; i++ {
				if i != skip {
					hand[k] = cs[i]
					k++
				}
			}
			if s := eval5(hand[:]); s > best {
				best = s
			}
		}
		return best
	case 7:
		best := Score(-1)
		for a := 0; a < 6; a++ {
			for b := a + 1; b < 7; b++ {
				var hand [5]Card
				k := 0
				for i := 0; i < 7; i++ {
					if i != a && i != b {
						hand[k] = cs[i]
						k++
					}
				}
				if s := eval5(hand[:]); s > best {
					best = s
				}
			}
		}
		return best
	default:
		panic(fmt.Sprintf("cards: Evaluate needs 5..7 cards, got %d", len(cs)))
	}
}

// eval5 评估恰好 5 张牌。
func eval5(cs []Card) Score {
	var cnt [13]int
	var suitCnt [4]int
	for _, c := range cs {
		cnt[c.Rank()]++
		suitCnt[c.Suit()]++
	}
	flush := false
	for _, n := range suitCnt {
		if n == 5 {
			flush = true
			break
		}
	}
	straight := straightHigh(&cnt)

	quad, trip := -1, -1
	var pairs [2]int
	np := 0
	for r := 12; r >= 0; r-- {
		switch cnt[r] {
		case 4:
			quad = r
		case 3:
			if trip < 0 {
				trip = r
			} else if np < 2 { // 第二个三条当对子（葫芦）
				pairs[np] = r
				np++
			}
		case 2:
			if np < 2 {
				pairs[np] = r
				np++
			}
		}
	}

	switch {
	case flush && straight >= 0:
		return makeScore(StraightFlush, straight)
	case quad >= 0:
		return makeScore(Quads, quad, kicker(&cnt, quad))
	case trip >= 0 && np > 0:
		return makeScore(FullHouse, trip, pairs[0])
	case flush:
		return makeScore(Flush, ranksDesc(&cnt, 5)...)
	case straight >= 0:
		return makeScore(Straight, straight)
	case trip >= 0:
		k := ranksDesc(&cnt, 3, trip)
		return makeScore(Trips, trip, k[0], k[1])
	case np == 2:
		return makeScore(TwoPair, pairs[0], pairs[1], kicker(&cnt, pairs[0], pairs[1]))
	case np == 1:
		k := ranksDesc(&cnt, 4, pairs[0])
		return makeScore(Pair, pairs[0], k[0], k[1], k[2])
	default:
		return makeScore(HighCard, ranksDesc(&cnt, 5)...)
	}
}

// straightHigh 返回顺子最高牌的 rank（A-2-3-4-5 轮子返回 3，即 Five），无顺子返回 -1。
func straightHigh(cnt *[13]int) int {
	for hi := 12; hi >= 4; hi-- {
		ok := true
		for r := hi; r > hi-5; r-- {
			if cnt[r] == 0 {
				ok = false
				break
			}
		}
		if ok {
			return hi
		}
	}
	if cnt[12] > 0 && cnt[0] > 0 && cnt[1] > 0 && cnt[2] > 0 && cnt[3] > 0 {
		return 3
	}
	return -1
}

// kicker 返回排除 exclude 后最大的 rank。
func kicker(cnt *[13]int, exclude ...int) int {
	return ranksDesc(cnt, 1, exclude...)[0]
}

// ranksDesc 返回降序的前 n 个 rank，跳过 exclude。
func ranksDesc(cnt *[13]int, n int, exclude ...int) []int {
	out := make([]int, 0, n)
loop:
	for r := 12; r >= 0 && len(out) < n; r-- {
		if cnt[r] == 0 {
			continue
		}
		for _, e := range exclude {
			if r == e {
				continue loop
			}
		}
		out = append(out, r)
	}
	return out
}

// makeScore 编码：类别 << 20 | 5 个 4-bit rank（不足补 0，类别相同才比较踢脚，故安全）。
func makeScore(cat Category, ranks ...int) Score {
	s := Score(cat) << 20
	for i := 0; i < 5 && i < len(ranks); i++ {
		s |= Score(ranks[i]) << uint(16-4*i)
	}
	return s
}
