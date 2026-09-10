package bot

import (
	"math/rand"

	"pocker/server/internal/cards"
	"pocker/server/internal/engine"
)

// DecisionView 一次决策所需的全部信息（对 bot 隐藏他人底牌：仅含自己的 Hole）。
type DecisionView struct {
	Seat       int
	Hole       []cards.Card
	Board      []cards.Card
	Street     engine.Street
	Pot        int
	CurrentBet int // 本街当前最高下注
	BigBlind   int
	Legal      engine.LegalActions

	NumOpponents  int // 未弃牌对手数（含已全下）
	RelPos        int // 参与玩家中距按钮的相对序号：0=BTN,1=SB,2=BB,...
	NumActive     int // 本手参与人数
	PreflopRaises int // 翻前已发生的加注次数（开池算 1）
}

// ViewFromEngine 从引擎状态构造 seat 视角的 DecisionView。
// 仅取 seat 自己的底牌；bot 决策不读他人底牌。
func ViewFromEngine(e *engine.Engine, seat int) DecisionView {
	ps := e.Players()
	v := DecisionView{
		Seat:       seat,
		Board:      e.Board(),
		Street:     e.Street(),
		Pot:        e.Pot(),
		CurrentBet: e.CurrentBet(),
		Legal:      e.LegalActions(),
		Hole:       ps[seat].Hole,
	}
	// 参与人数与相对位置：按钮起按座位顺序数未 Out 的玩家
	n := len(ps)
	active := 0
	for _, p := range ps {
		if !p.Out {
			active++
		}
	}
	v.NumActive = active
	rel := 0
	for k := 0; ; k++ {
		s := (e.Button() + k) % n
		if s == seat {
			v.RelPos = rel
			break
		}
		if !ps[s].Out {
			rel++
		}
	}
	opp := 0
	for i, p := range ps {
		if i != seat && !p.Out && !p.Folded {
			opp++
		}
	}
	v.NumOpponents = opp
	// 翻前加注次数与大盲
	for _, r := range e.Log() {
		if r.Street == engine.Preflop && (r.Type == engine.ActionRaise || r.Type == engine.ActionBet) {
			v.PreflopRaises++
		}
		if r.Type == engine.ActionBlind && r.Amount > v.BigBlind {
			v.BigBlind = r.Amount
		}
	}
	if v.BigBlind <= 0 {
		v.BigBlind = 2
	}
	return v
}

// Decide 按风格参数对标准打法做倾向偏移后给出合法动作。
// s 会被 clamp 到 [0,1]；rng 控制采样，保证同局面有倾向但不机械。
func Decide(v DecisionView, s Style, rng *rand.Rand) engine.Action {
	s = s.clamped()
	if v.Street == engine.Preflop {
		return decidePreflop(v, s, rng)
	}
	return decidePostflop(v, s, rng)
}

// ---------------------------------------------------------------- preflop

func decidePreflop(v DecisionView, s Style, rng *rand.Rand) engine.Action {
	la := v.Legal
	pct := Percentile(v.Hole)
	pc := classifyPosition(v.RelPos, v.NumActive)
	bb := v.BigBlind

	raiseTo := func(mult float64) engine.Action {
		to := int(float64(v.CurrentBet)*mult + 0.5)
		if v.CurrentBet == 0 || v.CurrentBet == bb {
			to = int(float64(bb)*mult + 0.5) // 开池尺度：mult 个 bb
		}
		if to < la.MinRaiseTo {
			to = la.MinRaiseTo
		}
		if to >= la.MaxRaiseTo {
			to = la.MaxRaiseTo
		}
		return engine.Action{Type: engine.ActionRaise, Amount: to}
	}

	call := func() engine.Action {
		if la.CallAmount > 0 {
			return engine.Action{Type: engine.ActionCall}
		}
		return engine.Action{Type: engine.ActionCheck}
	}
	foldOrCheck := func() engine.Action {
		if la.CanCheck {
			return engine.Action{Type: engine.ActionCheck}
		}
		return engine.Action{Type: engine.ActionFold}
	}

	unopened := v.PreflopRaises == 0 // 无人加注（可能有溜入）

	if unopened {
		// BB 面对全员溜入可直接过牌
		if la.CanCheck && pct < openThreshold(posBigBlind, s) {
			return engine.Action{Type: engine.ActionCheck}
		}
		th := openThreshold(pc, s)
		// 每个溜入者略微提高门槛
		limpers := countLimpers(v)
		th += float64(limpers) * 0.015
		if pct >= th && la.CanRaise {
			// 入池时以 PFR/VPIP 的概率加注，否则跟注（溜入）
			raiseProb := 1.0
			if s.VPIP > 0.01 {
				raiseProb = s.PFR / s.VPIP
			}
			if la.CallAmount > 0 && rng.Float64() >= raiseProb {
				return call()
			}
			return raiseTo(2.5 + rng.Float64()) // 2.5-3.5bb
		}
		// 不入池，但跟注站倾向下宽跟溜入
		if la.CallAmount > 0 && la.CallAmount <= bb && rng.Float64() < s.CallTendency*0.3 && pct >= th-0.15 {
			return call()
		}
		return foldOrCheck()
	}

	// 面对加注
	switch {
	case v.PreflopRaises == 1:
		// 3bet 范围：3% + 9%*ThreeBet
		threeTh := 1 - (0.03 + 0.09*s.ThreeBet)
		if pct >= threeTh && la.CanRaise {
			return raiseTo(3)
		}
		// 跟注范围：以开池门槛为基准，跟注倾向越高压得越低
		callTh := openThreshold(pc, s) - 0.05 - 0.15*s.CallTendency
		if pct >= callTh {
			return call()
		}
		// 诈唬 3bet：极低频，用 BluffFreq 控制
		if la.CanRaise && pct >= callTh-0.12 && rng.Float64() < s.BluffFreq*0.15 {
			return raiseTo(3)
		}
		return foldOrCheck()
	default:
		// 4bet+：只打顶端范围
		top := 1 - (0.015 + 0.03*s.ThreeBet)
		if pct >= top {
			if la.CanRaise && rng.Float64() < 0.4+0.5*s.Aggression {
				return engine.Action{Type: engine.ActionRaise, Amount: la.MaxRaiseTo}
			}
			return call()
		}
		if pct >= top-0.06 && rng.Float64() < s.CallTendency*0.4 {
			return call()
		}
		return foldOrCheck()
	}
}

// countLimpers 估计翻前溜入人数（call 大盲的非盲注动作数）。
func countLimpers(v DecisionView) int {
	// 简化：pot 中扣除盲注与当前待跟部分后估算
	base := v.BigBlind + v.BigBlind/2
	extra := v.Pot - base - v.CurrentBet
	if extra <= 0 {
		return 0
	}
	return extra / v.BigBlind
}

// ---------------------------------------------------------------- postflop

// drawKind 听牌类型。
type drawKind int

const (
	noDraw drawKind = iota
	straightDraw
	flushDraw
)

// detectDraw 检测听牌（河牌无听牌概念）。不区分卡顺与两头顺，够用即可。
func detectDraw(hole, board []cards.Card) drawKind {
	if len(board) >= 5 || len(board) < 3 {
		return noDraw
	}
	all := append(append([]cards.Card(nil), hole...), board...)
	// 同花听牌：某花色 >=4 张且自己手里有该花色
	var suitCnt [4]int
	var holeSuit [4]bool
	for _, c := range hole {
		holeSuit[c.Suit()] = true
	}
	for _, c := range all {
		suitCnt[c.Suit()]++
	}
	for su := 0; su < 4; su++ {
		if suitCnt[su] >= 4 && holeSuit[su] {
			return flushDraw
		}
	}
	// 顺子听牌：存在某个 5 连窗口恰含 4 种牌面（含 A2345 轮子窗口）
	var present [13]bool
	for _, c := range all {
		present[c.Rank()] = true
	}
	windows := [10][5]int{
		{0, 1, 2, 3, 12}, // A2345
	}
	for lo := 0; lo <= 8; lo++ { // 23456 ... TJQKA
		for r := 0; r < 5; r++ {
			windows[lo+1][r] = lo + r
		}
	}
	for _, w := range windows {
		cnt := 0
		for _, r := range w {
			if present[r] {
				cnt++
			}
		}
		if cnt == 4 {
			return straightDraw
		}
	}
	return noDraw
}

// EquityIters 决策时每次蒙特卡洛权益估算的迭代次数。
// 模拟程序可调小以提速；精度要求高的场景可调大。
var EquityIters = DefaultEquityIters

func decidePostflop(v DecisionView, s Style, rng *rand.Rand) engine.Action {
	la := v.Legal
	opp := v.NumOpponents
	if opp < 1 {
		opp = 1
	}
	eq := EquityN(v.Hole, v.Board, opp, EquityIters, rng)
	draw := detectDraw(v.Hole, v.Board)
	pot := v.Pot
	if pot <= 0 {
		pot = v.BigBlind
	}

	// 下注/加注尺度：pot 的 frac 倍，转成 raise-to
	sizeTo := func(frac float64) int {
		to := v.CurrentBet + int(float64(pot)*frac+0.5)
		if to < la.MinRaiseTo {
			to = la.MinRaiseTo
		}
		if to > la.MaxRaiseTo {
			to = la.MaxRaiseTo
		}
		return to
	}
	betFrac := func() float64 {
		// 激进度越高尺度越大：1/3 池到满池
		return 0.33 + 0.67*s.Aggression*rng.Float64()
	}
	if la.CallAmount > 0 {
		// 面对下注。下注者范围强于随机牌，对 vs 随机的权益打范围折扣，
		// 否则全员会系统性过度弃牌、让激进型白捡底池。
		eq := eq - 0.06
		potOdds := float64(la.CallAmount) / float64(pot+la.CallAmount)
		// 价值加注
		raiseTh := 0.70 - 0.10*s.Aggression
		if la.CanRaise && eq >= raiseTh {
			return engine.Action{Type: engine.ActionRaise, Amount: sizeTo(betFrac())}
		}
		// 半诈唬加注
		if la.CanRaise && draw != noDraw && rng.Float64() < s.BluffFreq*0.4 {
			return engine.Action{Type: engine.ActionRaise, Amount: sizeTo(0.5)}
		}
		// 纯诈唬加注（低频）
		if la.CanRaise && eq < 0.25 && rng.Float64() < s.BluffFreq*0.12 {
			return engine.Action{Type: engine.ActionRaise, Amount: sizeTo(0.5)}
		}
		// 跟注：权益覆盖赔率，跟注倾向降低门槛；听牌给隐含赔率宽限
		margin := potOdds + 0.08 - 0.06*s.CallTendency
		if draw != noDraw {
			margin -= 0.10
		}
		if eq >= margin {
			return engine.Action{Type: engine.ActionCall}
		}
		return engine.Action{Type: engine.ActionFold}
	}

	// 无人下注：过牌或主动下注。对手越多，领先下注的门槛越高。
	betTh := 0.60 - 0.14*s.Aggression + 0.03*float64(opp-1)
	if la.CanRaise && eq >= betTh {
		return engine.Action{Type: engine.ActionBet, Amount: sizeTo(betFrac())}
	}
	if la.CanRaise && draw != noDraw && rng.Float64() < s.BluffFreq*0.5 {
		return engine.Action{Type: engine.ActionBet, Amount: sizeTo(0.5)}
	}
	if la.CanRaise && eq < 0.25 && rng.Float64() < s.BluffFreq*0.2 {
		return engine.Action{Type: engine.ActionBet, Amount: sizeTo(0.5)}
	}
	return engine.Action{Type: engine.ActionCheck}
}
