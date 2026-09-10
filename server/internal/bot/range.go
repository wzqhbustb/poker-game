package bot

import (
	"sync"

	"pocker/server/internal/cards"
)

// chen 用 Chen 公式评估起手牌强度（0-20 分），作为范围档位的基础。
func chen(hole []cards.Card) float64 {
	r0, r1 := int(hole[0].Rank()), int(hole[1].Rank())
	hi, lo := r0, r1
	if lo > hi {
		hi, lo = lo, hi
	}
	points := func(r int) float64 {
		switch r {
		case int(cards.Ace):
			return 10
		case int(cards.King):
			return 8
		case int(cards.Queen):
			return 7
		case int(cards.Jack):
			return 6
		default:
			return float64(r+2) / 2 // rank 值 0=2 ... 8=T
		}
	}
	if r0 == r1 {
		s := 2 * points(hi)
		if s < 5 {
			s = 5
		}
		return s
	}
	s := points(hi)
	if hole[0].Suit() == hole[1].Suit() {
		s += 2
	}
	gap := hi - lo - 1
	switch {
	case gap == 0:
		// 连张无罚分
	case gap == 1:
		s -= 1
	case gap == 2:
		s -= 2
	case gap == 3:
		s -= 4
	default:
		s -= 5
	}
	// 小于 Q 的连张/隔一张有额外顺子潜力
	if gap <= 1 && hi < int(cards.Queen) {
		s += 1
	}
	if s < 0 {
		s = 0
	}
	return s
}

// percentileOnce 惰性计算所有规范起手牌（169 种）的 Chen 分分布，用于 percentile。
var percentileOnce struct {
	sync.Once
	scores []float64 // 所有 1326 种组合的 Chen 分
}

func allChenScores() []float64 {
	percentileOnce.Do(func() {
		deck := cards.Deck()
		var ss []float64
		for i := 0; i < len(deck); i++ {
			for j := i + 1; j < len(deck); j++ {
				ss = append(ss, chen([]cards.Card{deck[i], deck[j]}))
			}
		}
		percentileOnce.scores = ss
	})
	return percentileOnce.scores
}

// Percentile 返回起手牌在全部 1326 种组合中的强度百分位 [0,1]，
// 1 为最强（AA 约等于 1）。同分时取中点。
func Percentile(hole []cards.Card) float64 {
	s := chen(hole)
	var less, equal int
	for _, o := range allChenScores() {
		if o < s {
			less++
		} else if o == s {
			equal++
		}
	}
	total := len(allChenScores())
	return (float64(less) + 0.5*float64(equal)) / float64(total)
}

// positionClass 位置分类。
type positionClass int

const (
	posEarly positionClass = iota // UTG/MP
	posHijack
	posCutoff
	posButton
	posSmallBlind
	posBigBlind
)

// classifyPosition 根据相对按钮的座位序号（0=BTN,1=SB,2=BB,...）与参与人数分类。
func classifyPosition(rel, numActive int) positionClass {
	switch rel {
	case 0:
		return posButton
	case 1:
		return posSmallBlind
	case 2:
		return posBigBlind
	}
	switch rel {
	case numActive - 1:
		return posCutoff
	case numActive - 2:
		return posHijack
	default:
		return posEarly
	}
}

// openThreshold 返回该位置开池（第一个主动加注入池）所需的起手牌百分位门槛。
// 基准对应 9 人桌标准紧凶范围：early ~14%、HJ ~20%、CO ~28%、BTN ~45%、SB ~40%。
func openThreshold(pc positionClass, s Style) float64 {
	var t float64
	switch pc {
	case posEarly:
		t = 0.86
	case posHijack:
		t = 0.80
	case posCutoff:
		t = 0.72
	case posButton:
		t = 0.55
	case posSmallBlind:
		t = 0.60
	case posBigBlind:
		t = 0.90 // BB 面对溜入的加注范围
	}
	// VPIP 以 0.25 为中性：越高门槛越低（越松）
	t -= (s.VPIP - 0.25) * 0.5
	if t < 0.30 {
		t = 0.30
	}
	if t > 0.97 {
		t = 0.97
	}
	return t
}
