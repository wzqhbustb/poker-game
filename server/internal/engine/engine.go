// Package engine 实现 2-9 人桌德州扑克单手牌的完整状态机（无上限注）。
//
// 驱动方式为外部轮询：
//
//	e := engine.New(stacks, button, engine.Config{}, rng)
//	for !e.Over() {
//		seat := e.CurrentActor()
//		la := e.LegalActions()
//		a := decide(seat, la) // 由 bot / WS 层实现
//		if err := e.Act(a); err != nil {
//			// 非法动作
//		}
//	}
//	results := e.Results()
//
// 按钮位在多手之间由调用方维护，可用 NextButton 辅助轮转。
// 盲注/全下/边池/摊牌分池均在内部处理；行动流水通过 Log 导出供复盘存储。
package engine

import (
	"math/rand"

	"pocker/server/internal/cards"
)

// Street 下注街。
type Street int

const (
	Preflop Street = iota
	Flop
	Turn
	River
)

func (s Street) String() string {
	switch s {
	case Preflop:
		return "preflop"
	case Flop:
		return "flop"
	case Turn:
		return "turn"
	case River:
		return "river"
	}
	return "unknown"
}

// ActionType 动作类型。
type ActionType int

const (
	ActionFold ActionType = iota
	ActionCheck
	ActionCall
	ActionBet
	ActionRaise
	ActionBlind // 强制盲注，仅出现在 Log 中，不可作为 Act 输入
)

func (t ActionType) String() string {
	switch t {
	case ActionFold:
		return "fold"
	case ActionCheck:
		return "check"
	case ActionCall:
		return "call"
	case ActionBet:
		return "bet"
	case ActionRaise:
		return "raise"
	case ActionBlind:
		return "blind"
	}
	return "unknown"
}

// Action 一次玩家动作。Bet/Raise 时 Amount 为“加注到”的本街总下注额（raise-to），
// 其余动作忽略 Amount。
type Action struct {
	Type   ActionType
	Amount int
}

// Config 牌桌配置。零值等价于默认盲注 1/2。
type Config struct {
	SmallBlind int
	BigBlind   int
}

func (c Config) withDefaults() Config {
	if c.SmallBlind <= 0 {
		c.SmallBlind = 1
	}
	if c.BigBlind <= 0 {
		c.BigBlind = 2
	}
	if c.BigBlind < c.SmallBlind {
		c.BigBlind = 2 * c.SmallBlind
	}
	return c
}

// ActionRecord 行动流水的一条记录（含盲注）。
type ActionRecord struct {
	Street Street
	Seat   int
	Type   ActionType
	Amount int // 本次动作投入的筹码增量（fold/check 为 0）
	To     int // 动作后该玩家本街总下注额
}

// PlayerState 玩家在某时刻的公开状态快照。
type PlayerState struct {
	Seat      int
	Stack     int  // 剩余筹码
	Bet       int  // 本街已下注
	Committed int  // 本手累计投入
	Folded    bool // 已弃牌
	AllIn     bool // 已全下
	Out       bool // 起手无筹码，未参与本手
	Hole      []cards.Card
}

// LegalActions 当前行动玩家的合法动作集合。
type LegalActions struct {
	CanFold    bool
	CanCheck   bool
	CallAmount int  // >0 时可跟注的额度（可能因筹码不足而全下）
	CanRaise   bool // 可下注/加注（sub-minimum all-in 后未重新开放的玩家为 false）
	MinRaiseTo int  // 最小 raise-to（本街总下注额）
	MaxRaiseTo int  // 最大 raise-to，即全下
}

// Result 一手结束后单个座位的结算。
type Result struct {
	Seat      int
	Won       int // 赢得的筹码（含拿回自己的投入）
	Committed int // 本手累计投入
	Net       int // Won - Committed
	Showdown  bool
	Score     cards.Score // 仅 Showdown 为 true 时有效
}

// Pot 一个主池/边池的分配结果。
type Pot struct {
	Amount   int
	Eligible []int // 有资格竞争的座位
	Winners  []int
}

type player struct {
	stack     int
	bet       int
	committed int
	folded    bool
	allIn     bool
	out       bool
	acted     bool // 本轮是否已行动（完整加注会重置其他玩家的该标记）
	hole      []cards.Card
	score     cards.Score
}

// Engine 单手牌状态机。非并发安全，由调用方串行驱动。
type Engine struct {
	cfg        Config
	button     int
	deck       []cards.Card
	deckPos    int
	board      []cards.Card
	players    []player
	street     Street
	currentBet int // 本街当前最高下注
	lastRaise  int // 最近一次完整加注的增量（决定最小加注）
	actor      int
	over       bool
	log        []ActionRecord
	pots       []Pot
	results    []Result
}

// New 用 rng 洗牌后开一手。stacks 为各座位筹码（0 表示该座空手不参与），
// 至少 2 个座位有筹码；button 为按钮位座位号。
func New(stacks []int, button int, cfg Config, rng *rand.Rand) *Engine {
	d := cards.Deck()
	cards.Shuffle(d, rng)
	return NewWithDeck(stacks, button, cfg, d)
}

// NewWithDeck 用给定牌堆开一手，供测试与可复现回放。
// deck[0] 最先发出；发牌顺序：从 SB 开始每人一张底牌共两轮，
// 之后翻牌前烧 1 张发 3 张，转牌/河牌各烧 1 张发 1 张。
// 牌堆需足够发完本手（2*参与人数+8 张）。
func NewWithDeck(stacks []int, button int, cfg Config, deck []cards.Card) *Engine {
	e := newEngine(stacks, button, cfg, deck)
	e.dealHole()
	e.postBlinds()
	e.progress()
	return e
}

// Over 报告本手是否已结束。
func (e *Engine) Over() bool { return e.over }

// CurrentActor 返回当前应行动的座位号；本手已结束时返回 -1。
func (e *Engine) CurrentActor() int {
	if e.over {
		return -1
	}
	return e.actor
}

// Street 返回当前街。
func (e *Engine) Street() Street { return e.street }

// Board 返回已发出的公共牌（副本）。
func (e *Engine) Board() []cards.Card {
	return append([]cards.Card(nil), e.board...)
}

// Button 返回按钮位座位号。
func (e *Engine) Button() int { return e.button }

// Pot 返回当前底池总额（所有玩家本手累计投入）。
func (e *Engine) Pot() int {
	total := 0
	for i := range e.players {
		total += e.players[i].committed
	}
	return total
}

// CurrentBet 返回本街当前最高下注额。
func (e *Engine) CurrentBet() int { return e.currentBet }

// Players 返回各座位状态快照。
func (e *Engine) Players() []PlayerState {
	out := make([]PlayerState, len(e.players))
	for i, p := range e.players {
		out[i] = PlayerState{
			Seat:      i,
			Stack:     p.stack,
			Bet:       p.bet,
			Committed: p.committed,
			Folded:    p.folded,
			AllIn:     p.allIn,
			Out:       p.out,
			Hole:      append([]cards.Card(nil), p.hole...),
		}
	}
	return out
}

// Log 返回完整行动流水（含盲注），供复盘存储。
func (e *Engine) Log() []ActionRecord {
	return append([]ActionRecord(nil), e.log...)
}

// Results 返回结算结果（按座位）。本手未结束时返回 nil。
func (e *Engine) Results() []Result {
	if !e.over {
		return nil
	}
	return append([]Result(nil), e.results...)
}

// Pots 返回主池/边池分配明细。本手未结束时返回 nil。
func (e *Engine) Pots() []Pot {
	if !e.over {
		return nil
	}
	return append([]Pot(nil), e.pots...)
}

// FinalStacks 返回结算后各座位筹码。本手未结束时返回 nil。
func (e *Engine) FinalStacks() []int {
	if !e.over {
		return nil
	}
	out := make([]int, len(e.players))
	for i, p := range e.players {
		out[i] = p.stack
	}
	return out
}

// NextButton 返回 button 之后第一个有筹码的座位，供多手之间轮转按钮位。
func NextButton(button int, stacks []int) int {
	n := len(stacks)
	for k := 1; k <= n; k++ {
		seat := (button + k) % n
		if stacks[seat] > 0 {
			return seat
		}
	}
	return button
}
