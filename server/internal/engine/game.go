package engine

import (
	"errors"
	"fmt"

	"pocker/server/internal/cards"
)

func newEngine(stacks []int, button int, cfg Config, deck []cards.Card) *Engine {
	cfg = cfg.withDefaults()
	n := len(stacks)
	if n < 2 || n > 9 {
		panic(fmt.Sprintf("engine: need 2..9 seats, got %d", n))
	}
	e := &Engine{cfg: cfg, deck: deck, players: make([]player, n), street: Preflop}
	active := 0
	for i, s := range stacks {
		if s < 0 {
			panic(fmt.Sprintf("engine: negative stack at seat %d", i))
		}
		e.players[i].stack = s
		e.players[i].out = s == 0
		if s > 0 {
			active++
		}
	}
	if active < 2 {
		panic("engine: need at least 2 players with chips")
	}
	button = ((button % n) + n) % n
	for e.players[button].out {
		button = (button + 1) % n
	}
	e.button = button
	return e
}

// nextIn 返回 i 之后第一个有筹码参与本手的座位（严格向后）。
func (e *Engine) nextIn(i int) int {
	n := len(e.players)
	for k := 1; k <= n; k++ {
		seat := (i + k) % n
		if !e.players[seat].out {
			return seat
		}
	}
	return i
}

func (e *Engine) draw() cards.Card {
	c := e.deck[e.deckPos]
	e.deckPos++
	return c
}

// dealHole 从 SB 开始每人一张底牌，共两轮。两人桌时 SB 即按钮位。
func (e *Engine) dealHole() {
	sb := e.sbSeat()
	for round := 0; round < 2; round++ {
		seat := sb
		for {
			if !e.players[seat].out {
				e.players[seat].hole = append(e.players[seat].hole, e.draw())
			}
			seat = (seat + 1) % len(e.players)
			if seat == sb {
				break
			}
		}
	}
}

// sbSeat 小盲座位：两人桌时按钮位即小盲。
func (e *Engine) sbSeat() int {
	active := 0
	for i := range e.players {
		if !e.players[i].out {
			active++
		}
	}
	if active == 2 {
		return e.button
	}
	return e.nextIn(e.button)
}

func (e *Engine) postBlinds() {
	sb := e.sbSeat()
	bb := e.nextIn(sb)
	e.postBlind(sb, e.cfg.SmallBlind)
	e.postBlind(bb, e.cfg.BigBlind)
	e.currentBet = e.cfg.BigBlind
	e.lastRaise = e.cfg.BigBlind
	e.actor = (bb + 1) % len(e.players) // UTG，progress 内再精确定位
}

func (e *Engine) postBlind(seat, amount int) {
	p := &e.players[seat]
	if amount > p.stack {
		amount = p.stack
	}
	e.chipIn(p, amount)
	e.record(seat, ActionBlind, amount)
}

func (e *Engine) chipIn(p *player, amount int) {
	p.stack -= amount
	p.bet += amount
	p.committed += amount
	if p.stack == 0 {
		p.allIn = true
	}
}

func (e *Engine) record(seat int, t ActionType, amount int) {
	e.log = append(e.log, ActionRecord{
		Street: e.street,
		Seat:   seat,
		Type:   t,
		Amount: amount,
		To:     e.players[seat].bet,
	})
}

// LegalActions 返回当前行动玩家的合法动作。本手已结束时返回零值。
func (e *Engine) LegalActions() LegalActions {
	var la LegalActions
	if e.over {
		return la
	}
	p := &e.players[e.actor]
	toCall := e.currentBet - p.bet
	la.CanFold = true
	la.CanCheck = toCall == 0
	if toCall > 0 {
		la.CallAmount = min(toCall, p.stack)
	}
	// acted 为 true 且被未到最小加注额的全下再次逼到行动时，只允许跟注/弃牌
	la.CanRaise = p.stack > toCall && !p.acted
	la.MinRaiseTo = e.currentBet + e.lastRaise
	la.MaxRaiseTo = p.bet + p.stack
	return la
}

// Act 让当前行动玩家执行动作。非法动作返回错误且状态不变。
func (e *Engine) Act(a Action) error {
	if e.over {
		return errors.New("engine: hand is over")
	}
	p := &e.players[e.actor]
	la := e.LegalActions()
	toCall := e.currentBet - p.bet

	switch a.Type {
	case ActionFold:
		p.folded = true
		e.record(e.actor, ActionFold, 0)
	case ActionCheck:
		if !la.CanCheck {
			return fmt.Errorf("engine: cannot check, need to call %d", toCall)
		}
		e.record(e.actor, ActionCheck, 0)
	case ActionCall:
		if la.CallAmount <= 0 {
			return errors.New("engine: nothing to call")
		}
		e.chipIn(p, la.CallAmount)
		e.record(e.actor, ActionCall, la.CallAmount)
	case ActionBet:
		if e.currentBet != 0 {
			return errors.New("engine: cannot bet, use raise")
		}
		if err := e.applyRaiseTo(la, a.Amount); err != nil {
			return err
		}
	case ActionRaise:
		if e.currentBet == 0 {
			return errors.New("engine: cannot raise, use bet")
		}
		if err := e.applyRaiseTo(la, a.Amount); err != nil {
			return err
		}
	default:
		return fmt.Errorf("engine: unknown action type %d", a.Type)
	}
	p.acted = true
	e.progress()
	return nil
}

// applyRaiseTo 把本街总下注提高到 amount（bet 与 raise 共用）。
func (e *Engine) applyRaiseTo(la LegalActions, amount int) error {
	if !la.CanRaise {
		return errors.New("engine: raise not allowed (betting not reopened)")
	}
	if amount != la.MaxRaiseTo && amount < la.MinRaiseTo {
		return fmt.Errorf("engine: raise to %d below minimum %d", amount, la.MinRaiseTo)
	}
	if amount > la.MaxRaiseTo {
		return fmt.Errorf("engine: raise to %d above max %d", amount, la.MaxRaiseTo)
	}
	p := &e.players[e.actor]
	delta := amount - p.bet
	isBet := e.currentBet == 0
	e.chipIn(p, delta)
	raiseSize := amount - e.currentBet
	if raiseSize >= e.lastRaise {
		// 完整加注：重新开放行动，其他玩家需重新表态
		for i := range e.players {
			if i != e.actor {
				e.players[i].acted = false
			}
		}
		e.lastRaise = raiseSize
	}
	e.currentBet = amount
	if isBet {
		e.record(e.actor, ActionBet, delta)
	} else {
		e.record(e.actor, ActionRaise, delta)
	}
	return nil
}

// progress 推进状态机直到需要某个玩家行动或本手结束。
func (e *Engine) progress() {
	for !e.over {
		if alive := e.aliveCount(); alive == 1 {
			e.settle(false)
			return
		}
		if i, ok := e.nextActor(); ok {
			e.actor = i
			return
		}
		// 本轮下注结束
		if e.street == River || e.canBetCount() < 2 {
			// 河牌打完，或剩余玩家已无法形成新的下注（全员/仅一人有筹码），直接发完摊牌
			e.runout()
			e.showdown()
			return
		}
		e.advanceStreet()
	}
}

// nextActor 从 e.actor 起找第一个需要行动的玩家（未弃牌、未全下、且未表态或注额未跟上）。
func (e *Engine) nextActor() (int, bool) {
	n := len(e.players)
	for k := 0; k < n; k++ {
		i := (e.actor + k) % n
		p := &e.players[i]
		if p.out || p.folded || p.allIn {
			continue
		}
		if p.acted && p.bet == e.currentBet {
			continue
		}
		return i, true
	}
	return -1, false
}

func (e *Engine) aliveCount() int {
	n := 0
	for i := range e.players {
		if !e.players[i].out && !e.players[i].folded {
			n++
		}
	}
	return n
}

// canBetCount 返回仍能参与下注（未弃牌且有筹码）的玩家数。
func (e *Engine) canBetCount() int {
	n := 0
	for i := range e.players {
		p := &e.players[i]
		if !p.out && !p.folded && !p.allIn {
			n++
		}
	}
	return n
}

func (e *Engine) advanceStreet() {
	for i := range e.players {
		e.players[i].bet = 0
		e.players[i].acted = false
	}
	e.currentBet = 0
	e.lastRaise = e.cfg.BigBlind
	switch e.street {
	case Preflop:
		e.draw() // 烧牌
		e.board = append(e.board, e.draw(), e.draw(), e.draw())
		e.street = Flop
	case Flop:
		e.draw()
		e.board = append(e.board, e.draw())
		e.street = Turn
	case Turn:
		e.draw()
		e.board = append(e.board, e.draw())
		e.street = River
	}
	e.actor = (e.button + 1) % len(e.players) // 按钮后第一个存活玩家，由 nextActor 定位
}

// runout 不再产生下注，直接把公共牌发满 5 张。
func (e *Engine) runout() {
	for len(e.board) < 5 {
		e.draw()
		if len(e.board) == 0 {
			e.board = append(e.board, e.draw(), e.draw(), e.draw())
		} else {
			e.board = append(e.board, e.draw())
		}
	}
	e.street = River
}

func (e *Engine) showdown() {
	for i := range e.players {
		p := &e.players[i]
		if p.out || p.folded {
			continue
		}
		hand := make([]cards.Card, 0, 7)
		hand = append(hand, p.hole...)
		hand = append(hand, e.board...)
		p.score = cards.Evaluate(hand)
	}
	e.settle(true)
}

// settle 按投入分层构造主池/边池并分配。showdown 为 false 时只剩一名未弃牌玩家。
func (e *Engine) settle(showdown bool) {
	n := len(e.players)
	// 收集所有不同的投入档位（含已弃牌玩家的投入）
	seen := map[int]bool{}
	var levels []int
	for i := range e.players {
		if c := e.players[i].committed; c > 0 && !seen[c] {
			seen[c] = true
			levels = append(levels, c)
		}
	}
	for i := 0; i < len(levels); i++ { // 小的插入排序
		for j := i + 1; j < len(levels); j++ {
			if levels[j] < levels[i] {
				levels[i], levels[j] = levels[j], levels[i]
			}
		}
	}

	won := make([]int, n)
	prev := 0
	for _, lv := range levels {
		amount := 0
		var eligible []int
		for i := range e.players {
			p := &e.players[i]
			c := min(p.committed, lv)
			if c > prev {
				amount += c - prev
			}
			if !p.out && !p.folded && p.committed >= lv {
				eligible = append(eligible, i)
			}
		}
		prev = lv
		if amount == 0 || len(eligible) == 0 {
			continue
		}
		winners := eligible
		if showdown {
			winners = nil
			var best cards.Score = -1
			for _, i := range eligible {
				if s := e.players[i].score; s > best {
					best = s
					winners = []int{i}
				} else if s == best {
					winners = append(winners, i)
				}
			}
		}
		share := amount / len(winners)
		rem := amount % len(winners)
		for _, i := range winners {
			won[i] += share
		}
		// 零头从按钮位后第一个赢家开始一人一枚
		for k := 1; rem > 0 && k <= n; k++ {
			seat := (e.button + k) % n
			for _, w := range winners {
				if w == seat {
					won[seat]++
					rem--
					break
				}
			}
		}
		e.pots = append(e.pots, Pot{Amount: amount, Eligible: eligible, Winners: winners})
	}

	e.results = make([]Result, n)
	for i := range e.players {
		p := &e.players[i]
		p.stack += won[i]
		e.results[i] = Result{
			Seat:      i,
			Won:       won[i],
			Committed: p.committed,
			Net:       won[i] - p.committed,
			Showdown:  showdown && !p.out && !p.folded,
			Score:     p.score,
		}
	}
	e.actor = -1
	e.over = true
}
