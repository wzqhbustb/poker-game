// Package table 单桌编排层：9 座位，seat 0 为人类（WS 客户端），1-8 为 bot。
//
// 并发模型：一张桌一个 run 协程拥有全部可变状态（筹码/当前引擎/人类连接），
// 外部输入（join/leave/action/rebuy）全部走 channel 汇入该协程，
// 人类行动等待期间 select 超时定时器，超时自动 check/fold，因此无需任何锁。
package table

import (
	"context"
	"log"
	"math/rand"
	"time"

	"pocker/server/internal/bot"
	"pocker/server/internal/engine"
	"pocker/server/internal/proto"
	"pocker/server/internal/store"
)

// Seats 固定座位数。
const Seats = 9

// HumanSeat 人类固定座位。
const HumanSeat = 0

// Config 牌桌配置。
type Config struct {
	SmallBlind    int
	BigBlind      int
	BuyIn         int
	ActionTimeout time.Duration // 人类行动超时，超时自动 check/fold
	Fast          bool          // bot 不 sleep（测试用）
	Seed          int64         // 0 表示随机
}

func (c Config) withDefaults() Config {
	if c.SmallBlind <= 0 {
		c.SmallBlind = 1
	}
	if c.BigBlind <= 0 {
		c.BigBlind = 2
	}
	if c.BuyIn <= 0 {
		c.BuyIn = 200
	}
	if c.ActionTimeout <= 0 {
		c.ActionTimeout = 60 * time.Second
	}
	return c
}

// Client 人类 WS 连接的桌内句柄。Send 由桌子协程写入（非阻塞，满则丢弃），
// 由 WS 写协程消费后发给浏览器。
type Client struct {
	Name string
	Send chan any // proto 消息
}

func (c *Client) send(v any) {
	select {
	case c.Send <- v:
	default:
		log.Printf("table: client %q send buffer full, dropping %T", c.Name, v)
	}
}

// sendCritical 用于不可丢弃的消息（welcome/action_request/hand_end/info）：
// 缓冲满时最多等 5 秒，仍发不出去才放弃（此时连接多半已死，断线逻辑会兜底）。
func (c *Client) sendCritical(v any) {
	select {
	case c.Send <- v:
	case <-time.After(5 * time.Second):
		log.Printf("table: client %q critical send timed out, dropping %T", c.Name, v)
	}
}

type actionMsg struct {
	c      *Client
	action string
	amount int
}

// Table 单张牌桌。构造后调用 Run 启动游戏循环。
type Table struct {
	cfg Config
	st  *store.DB
	rng *rand.Rand

	joinCh  chan *Client
	leaveCh chan *Client
	actCh   chan actionMsg
	rebuyCh chan *Client

	// 以下字段仅 run 协程访问
	personas     []bot.Persona // seats 1..8
	human        *Client
	stacks       []int
	button       int
	handNo       int64
	thinking     int // 正在思考的 bot 座位，-1 无
	rebuyPending bool
	cur          *engine.Engine // 当前手，nil 表示手间间隙
	reveal       map[int][]string
	logLen       int
	boardLen     int
}

// New 创建桌子。st 为 nil 时手牌不落盘。
func New(cfg Config, st *store.DB) *Table {
	cfg = cfg.withDefaults()
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Table{
		cfg:      cfg,
		st:       st,
		rng:      rand.New(rand.NewSource(seed)),
		joinCh:   make(chan *Client, 4),
		leaveCh:  make(chan *Client, 4),
		actCh:    make(chan actionMsg, 16),
		rebuyCh:  make(chan *Client, 4),
		personas: bot.Personas(),
		thinking: -1,
	}
}

// Blinds 返回牌桌的（小盲, 大盲）。
func (t *Table) Blinds() (int, int) { return t.cfg.SmallBlind, t.cfg.BigBlind }

// Attach 人类客户端入座（hello）。重复调用会顶替旧连接。
func (t *Table) Attach(c *Client) { t.joinCh <- c }

// Detach 人类客户端离座（断线）。游戏不暂停，人类行动按超时处理。
func (t *Table) Detach(c *Client) { t.leaveCh <- c }

// SubmitAction 提交人类动作（fold|check|call|raise，amount 为 raise-to）。
func (t *Table) SubmitAction(c *Client, action string, amount int) {
	select {
	case t.actCh <- actionMsg{c: c, action: action, amount: amount}:
	default:
	}
}

// Rebuy 请求把人类座位筹码补充到买入，下一手生效。
func (t *Table) Rebuy(c *Client) {
	select {
	case t.rebuyCh <- c:
	default:
	}
}

// Run 游戏主循环，阻塞直到 ctx 取消。每手：破产 bot 自动补码、按钮轮转、
// 轮流行动（bot 随机思考延时，人类 WS 输入带超时），结束后落盘并广播。
func (t *Table) Run(ctx context.Context) {
	t.stacks = make([]int, Seats)
	for i := range t.stacks {
		t.stacks[i] = t.cfg.BuyIn
	}
	t.button = Seats - 1 // 第一手 NextButton 后按钮落在 seat 0
	for ctx.Err() == nil {
		t.playHand(ctx)
	}
}

// playHand 打完整一手。
func (t *Table) playHand(ctx context.Context) {
	t.poll() // fast 模式下 join/leave/rebuy 只在显式点消费，每手开头先处理
	// 破产 bot 自动补码；人类等 rebuy 消息
	for i := 1; i < Seats; i++ {
		if t.stacks[i] <= 0 {
			t.stacks[i] = t.cfg.BuyIn
		}
	}
	if t.rebuyPending {
		if t.stacks[HumanSeat] < t.cfg.BuyIn {
			t.stacks[HumanSeat] = t.cfg.BuyIn
		}
		t.rebuyPending = false
	}
	t.button = engine.NextButton(t.button, t.stacks)
	seed := t.rng.Int63()
	e := engine.New(t.stacks, t.button,
		engine.Config{SmallBlind: t.cfg.SmallBlind, BigBlind: t.cfg.BigBlind},
		rand.New(rand.NewSource(seed)))
	t.cur = e
	t.handNo++
	t.thinking = -1
	t.reveal = map[int][]string{}
	t.logLen = 0
	t.boardLen = 0
	startedAt := time.Now()
	startStacks := append([]int(nil), t.stacks...)

	t.send(proto.HandEvent{
		Type: proto.SHandEvent, HandNo: t.handNo, Kind: proto.EvHandStart,
		Street: e.Street().String(), Seat: e.Button(), Pot: e.Pot(),
	})
	t.drainEvents()
	t.pushState()

	for !e.Over() && ctx.Err() == nil {
		t.poll()
		seat := e.CurrentActor()
		if seat == HumanSeat {
			t.humanTurn(ctx, e)
		} else {
			t.botTurn(ctx, e, seat)
		}
		t.thinking = -1
		t.drainEvents()
		t.pushState()
	}
	if ctx.Err() != nil {
		t.cur = nil
		return // 服务关闭，半途手牌不落盘
	}
	t.finishHand(e, seed, startedAt, startStacks)
	t.cur = nil
}

// poll 非阻塞地处理积压在控制 channel 上的 join/leave/rebuy。
// bot 行动等待与人类行动等待内也会消费这些 channel；poll 覆盖 fast 模式下
// 无等待的间隙，保证断线/重连总能被及时处理。
func (t *Table) poll() {
	for {
		select {
		case c := <-t.joinCh:
			t.handleJoin(c)
		case c := <-t.leaveCh:
			t.handleLeave(c)
		case c := <-t.rebuyCh:
			t.handleRebuy(c)
		default:
			return
		}
	}
}

// humanTurn 等人类输入；超时或未连接自动 check/fold。
func (t *Table) humanTurn(ctx context.Context, e *engine.Engine) {
	la := e.LegalActions()
	if t.human == nil {
		e.Act(defaultAction(la)) // 掉线视为超时
		return
	}
	deadline := time.Now().Add(t.cfg.ActionTimeout)
	t.sendCritical(proto.ActionRequest{
		Type:     proto.SActionRequest,
		Deadline: deadline.UnixMilli(),
		Legal: proto.LegalInfo{
			CanFold:    la.CanFold,
			CanCheck:   la.CanCheck,
			CallAmount: la.CallAmount,
			CanRaise:   la.CanRaise,
			MinRaiseTo: la.MinRaiseTo,
			MaxRaiseTo: la.MaxRaiseTo,
		},
		Pot:        e.Pot(),
		CurrentBet: e.CurrentBet(),
	})
	timer := time.NewTimer(t.cfg.ActionTimeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			e.Act(defaultAction(la))
			return
		case c := <-t.joinCh:
			t.handleJoin(c)
		case c := <-t.leaveCh:
			t.handleLeave(c)
		case c := <-t.rebuyCh:
			t.handleRebuy(c)
		case a := <-t.actCh:
			if a.c != t.human {
				continue
			}
			act, err := toEngineAction(a, e)
			if err == nil {
				err = e.Act(act)
			}
			if err != nil {
				t.sendCritical(proto.ErrorMsg{Type: proto.SError, Message: err.Error()})
				continue
			}
			return
		case <-timer.C:
			e.Act(defaultAction(la))
			return
		}
	}
}

// botTurn bot 行动：先随机思考延时（期间处理连接事件），再 Decide。
func (t *Table) botTurn(ctx context.Context, e *engine.Engine, seat int) {
	p := t.personas[seat-1]
	t.thinking = seat
	t.pushState()
	if !t.cfg.Fast {
		lo, hi := p.ThinkMin, p.ThinkMax
		delay := time.Duration(lo) * time.Millisecond
		if hi > lo {
			delay += time.Duration(t.rng.Intn(hi-lo)) * time.Millisecond
		}
		t.wait(ctx, delay)
	}
	v := bot.ViewFromEngine(e, seat)
	if err := e.Act(bot.Decide(v, p.Style, t.rng)); err != nil {
		log.Printf("table: bot seat %d illegal action: %v", seat, err)
		e.Act(defaultAction(e.LegalActions()))
	}
}

// wait 睡眠 d，期间处理 join/leave/rebuy；非人类回合的 action 直接丢弃。
func (t *Table) wait(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case c := <-t.joinCh:
			t.handleJoin(c)
		case c := <-t.leaveCh:
			t.handleLeave(c)
		case c := <-t.rebuyCh:
			t.handleRebuy(c)
		case <-t.actCh: // 非人类回合，丢弃
		}
	}
}

func (t *Table) handleJoin(c *Client) {
	if t.human != nil && t.human != c {
		t.human.sendCritical(proto.ErrorMsg{Type: proto.SError, Message: "已被新连接顶替"})
	}
	t.human = c
	c.sendCritical(proto.Welcome{
		Type: proto.SWelcome,
		Seat: HumanSeat,
		Config: proto.TableConfig{
			Seats:           Seats,
			SmallBlind:      t.cfg.SmallBlind,
			BigBlind:        t.cfg.BigBlind,
			BuyIn:           t.cfg.BuyIn,
			ActionTimeoutMs: t.cfg.ActionTimeout.Milliseconds(),
		},
	})
	t.pushState()
}

func (t *Table) handleLeave(c *Client) {
	if t.human == c {
		t.human = nil
	}
}

func (t *Table) handleRebuy(c *Client) {
	if c != t.human {
		return
	}
	t.rebuyPending = true
	c.sendCritical(proto.InfoMsg{Type: proto.SInfo, Message: "补码将在下一手生效"})
}

// defaultAction 超时/掉线默认动作：能过牌则过牌，否则弃牌。
func defaultAction(la engine.LegalActions) engine.Action {
	if la.CanCheck {
		return engine.Action{Type: engine.ActionCheck}
	}
	return engine.Action{Type: engine.ActionFold}
}

// toEngineAction 把客户端动作转成引擎动作。currentBet 为 0 时 raise 实为 bet。
func toEngineAction(a actionMsg, e *engine.Engine) (engine.Action, error) {
	switch a.action {
	case "fold":
		return engine.Action{Type: engine.ActionFold}, nil
	case "check":
		return engine.Action{Type: engine.ActionCheck}, nil
	case "call":
		return engine.Action{Type: engine.ActionCall}, nil
	case "raise":
		if e.CurrentBet() == 0 {
			return engine.Action{Type: engine.ActionBet, Amount: a.amount}, nil
		}
		return engine.Action{Type: engine.ActionRaise, Amount: a.amount}, nil
	}
	return engine.Action{}, errUnknownAction(a.action)
}

type errUnknownAction string

func (e errUnknownAction) Error() string { return "unknown action: " + string(e) }
