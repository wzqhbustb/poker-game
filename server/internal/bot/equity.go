package bot

import (
	"math/rand"

	"pocker/server/internal/cards"
)

// DefaultEquityIters 是 Equity 默认的蒙特卡洛迭代次数。
const DefaultEquityIters = 1000

// Equity 用蒙特卡洛采样估算 hole 在当前 board 下、面对 numOpponents 个随机对手的
// 胜率（平局按人数分份额）。对未知底牌与剩余公共牌采样，单次调用为毫秒级。
func Equity(hole []cards.Card, board []cards.Card, numOpponents int, rng *rand.Rand) float64 {
	return EquityN(hole, board, numOpponents, DefaultEquityIters, rng)
}

// EquityN 同 Equity，但迭代次数由调用方指定，用于精度与耗时的权衡。
func EquityN(hole []cards.Card, board []cards.Card, numOpponents, iterations int, rng *rand.Rand) float64 {
	if numOpponents < 1 {
		return 1
	}
	if iterations < 1 {
		iterations = 1
	}
	rest := make([]cards.Card, 0, 52)
	known := make(map[cards.Card]bool, len(hole)+len(board))
	for _, c := range hole {
		known[c] = true
	}
	for _, c := range board {
		known[c] = true
	}
	for _, c := range cards.Deck() {
		if !known[c] {
			rest = append(rest, c)
		}
	}
	need := 2*numOpponents + (5 - len(board))

	hero := make([]cards.Card, 0, 7)
	opp := make([]cards.Card, 0, 7)
	var total float64
	for it := 0; it < iterations; it++ {
		// 部分 Fisher-Yates：前 need 张即均匀随机样本（原地，无需拷贝）
		for i := 0; i < need; i++ {
			j := i + rng.Intn(len(rest)-i)
			rest[i], rest[j] = rest[j], rest[i]
		}
		runout := rest[2*numOpponents : need]

		hero = append(hero[:0], hole...)
		hero = append(hero, board...)
		hero = append(hero, runout...)
		hs := cards.Evaluate(hero)

		win, ties := true, 0
		for o := 0; o < numOpponents; o++ {
			opp = append(opp[:0], rest[2*o], rest[2*o+1])
			opp = append(opp, board...)
			opp = append(opp, runout...)
			s := cards.Evaluate(opp)
			if s > hs {
				win = false
				break
			}
			if s == hs {
				ties++
			}
		}
		if win {
			total += 1 / float64(ties+1)
		}
	}
	return total / float64(iterations)
}
