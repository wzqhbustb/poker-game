package table

import (
	"log"
	"strings"
	"time"

	"pocker/server/internal/cards"
	"pocker/server/internal/engine"
	"pocker/server/internal/proto"
	"pocker/server/internal/store"
)

func cardStrings(cs []cards.Card) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.String()
	}
	return out
}

func (t *Table) send(v any) {
	if t.human != nil {
		t.human.send(v)
	}
}

// drainEvents 把引擎日志中的新动作与新发的街转为 hand_event 推给人类客户端。
func (t *Table) drainEvents() {
	e := t.cur
	if e == nil {
		return
	}
	log := e.Log()
	board := e.Board()
	if t.human == nil {
		// 无人接收也要推进游标，避免重连后旧事件重放
		t.logLen = len(log)
		t.boardLen = len(board)
		return
	}
	for ; t.logLen < len(log); t.logLen++ {
		r := log[t.logLen]
		t.send(proto.HandEvent{
			Type: proto.SHandEvent, HandNo: t.handNo, Kind: proto.EvAction,
			Street: r.Street.String(), Seat: r.Seat, Action: r.Type.String(),
			Amount: r.Amount, To: r.To, Pot: e.Pot(),
		})
	}
	// 全下 runout 会一次发多条街，逐条边界各发一个事件（3=flop 4=turn 5=river）
	streetNames := map[int]string{3: "flop", 4: "turn", 5: "river"}
	for t.boardLen < len(board) {
		switch t.boardLen {
		case 0:
			t.boardLen = 3
		default:
			t.boardLen++
		}
		if t.boardLen > len(board) {
			t.boardLen = len(board)
		}
		t.send(proto.HandEvent{
			Type: proto.SHandEvent, HandNo: t.handNo, Kind: proto.EvStreet,
			Street: streetNames[t.boardLen], Board: cardStrings(board[:t.boardLen]), Pot: e.Pot(),
		})
	}
}

// pushState 向人类客户端推送按其视角裁剪后的牌桌快照。
func (t *Table) pushState() {
	e := t.cur
	if e == nil || t.human == nil {
		return
	}
	ps := e.Players()
	actor := e.CurrentActor()
	seats := make([]proto.SeatState, Seats)
	for i := 0; i < Seats; i++ {
		ss := proto.SeatState{
			Seat:     i,
			Name:     t.seatName(i),
			IsBot:    i != HumanSeat,
			Stack:    t.stacks[i],
			ToAct:    i == actor,
			Thinking: i == t.thinking,
		}
		if i != HumanSeat {
			ss.StyleTag = t.personas[i-1].StyleTag
		}
		if i < len(ps) {
			ss.Stack = ps[i].Stack
			ss.Bet = ps[i].Bet
			ss.Folded = ps[i].Folded
			ss.AllIn = ps[i].AllIn
			ss.Out = ps[i].Out
			// 底牌裁剪：只发接收者本人的；摊牌后才发其他未弃牌者的
			if i == HumanSeat && len(ps[i].Hole) > 0 {
				ss.Hole = cardStrings(ps[i].Hole)
			} else if h, ok := t.reveal[i]; ok {
				ss.Hole = h
			}
		}
		seats[i] = ss
	}
	t.send(proto.State{
		Type:   proto.SState,
		HandNo: t.handNo,
		InHand: !e.Over(),
		Seats:  seats,
		Board:  cardStrings(e.Board()),
		Pot:    e.Pot(),
		Street: e.Street().String(),
		Button: e.Button(),
		You:    HumanSeat,
		Ts:     time.Now().UnixMilli(),
	})
}

func (t *Table) seatName(seat int) string {
	if seat == HumanSeat {
		if t.human != nil && t.human.Name != "" {
			return t.human.Name
		}
		return "You"
	}
	return t.personas[seat-1].Name
}

// finishHand 结算：更新筹码、落盘、推 hand_end 与揭示底牌后的最终 state。
func (t *Table) finishHand(e *engine.Engine, seed int64, startedAt time.Time, startStacks []int) {
	results := e.Results()
	final := e.FinalStacks()

	var revealed []proto.RevealedHole
	ps := e.Players()
	for _, r := range results {
		if r.Showdown {
			hole := cardStrings(ps[r.Seat].Hole)
			t.reveal[r.Seat] = hole
			if r.Seat != HumanSeat {
				revealed = append(revealed, proto.RevealedHole{Seat: r.Seat, Hole: hole})
			}
		}
	}

	// 落盘
	var handID int64
	if t.st != nil {
		h := &store.Hand{
			StartedAt:  startedAt,
			Button:     e.Button(),
			SmallBlind: t.cfg.SmallBlind,
			BigBlind:   t.cfg.BigBlind,
			Seed:       seed,
			Board:      strings.Join(cardStrings(e.Board()), " "),
		}
		for i, p := range ps {
			if p.Out {
				continue
			}
			h.Players = append(h.Players, store.PlayerRecord{
				Seat:       i,
				Name:       t.seatName(i),
				IsBot:      i != HumanSeat,
				StartStack: startStacks[i],
				EndStack:   final[i],
				Net:        results[i].Net,
				Hole:       strings.Join(cardStrings(p.Hole), " "),
			})
		}
		for i, r := range e.Log() {
			h.Actions = append(h.Actions, store.ActionRecord{
				Seq: i, Street: r.Street.String(), Seat: r.Seat,
				Type: r.Type.String(), Amount: r.Amount, To: r.To,
			})
		}
		id, err := t.st.SaveHand(h)
		if err != nil {
			log.Printf("table: save hand: %v", err)
		} else {
			handID = id
		}
	}

	he := proto.HandEnd{Type: proto.SHandEnd, HandNo: t.handNo, HandID: handID, RevealedHoles: revealed}
	for _, r := range results {
		he.Results = append(he.Results, proto.ResultInfo{
			Seat: r.Seat, Won: r.Won, Net: r.Net, Showdown: r.Showdown,
		})
	}
	for _, p := range e.Pots() {
		he.Pots = append(he.Pots, proto.PotInfo{Amount: p.Amount, Winners: p.Winners})
	}
	t.send(he)

	t.stacks = final
	t.pushState() // 终局快照：含摊牌揭示的底牌
}
