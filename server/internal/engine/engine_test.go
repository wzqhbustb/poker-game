package engine

import (
	"math/rand"
	"testing"

	"pocker/server/internal/cards"
)

func mustAct(t *testing.T, e *Engine, a Action) {
	t.Helper()
	if err := e.Act(a); err != nil {
		t.Fatalf("Act(%v %d): %v", a.Type, a.Amount, err)
	}
}

func fold(e *Engine)  { _ = e.Act(Action{Type: ActionFold}) }
func check(e *Engine) { _ = e.Act(Action{Type: ActionCheck}) }
func call(e *Engine)  { _ = e.Act(Action{Type: ActionCall}) }

func assertNets(t *testing.T, e *Engine, want []int) {
	t.Helper()
	if !e.Over() {
		t.Fatal("hand should be over")
	}
	res := e.Results()
	if len(res) != len(want) {
		t.Fatalf("results len = %d, want %d", len(res), len(want))
	}
	sum := 0
	for i, r := range res {
		if r.Net != want[i] {
			t.Fatalf("seat %d net = %d, want %d (all results: %+v)", i, r.Net, want[i], res)
		}
		sum += r.Net
	}
	if sum != 0 {
		t.Fatalf("nets sum = %d, want 0", sum)
	}
}

// 翻前全员弃牌到大盲，大盲直接收池。
func TestFoldToBigBlind(t *testing.T) {
	e := New([]int{100, 100, 100}, 0, Config{}, rand.New(rand.NewSource(1)))
	// button=0, SB=1, BB=2, UTG=0
	if e.CurrentActor() != 0 {
		t.Fatalf("first actor = %d, want 0", e.CurrentActor())
	}
	mustAct(t, e, Action{Type: ActionFold})
	mustAct(t, e, Action{Type: ActionFold}) // SB 弃牌
	assertNets(t, e, []int{0, -1, 1})
	if got := e.FinalStacks(); got[0] != 100 || got[1] != 99 || got[2] != 101 {
		t.Fatalf("final stacks = %v", got)
	}
	// 流水：两条盲注 + 两条 fold
	if len(e.Log()) != 4 {
		t.Fatalf("log len = %d, want 4", len(e.Log()))
	}
	if e.Log()[0].Type != ActionBlind || e.Log()[1].Type != ActionBlind {
		t.Fatal("first two records should be blinds")
	}
}

// 构造四人全下，验证主池 + 两条边池的分配。
// 牌堆布局（发牌顺序见 NewWithDeck 文档）：
// seat0 SB(50): As Ah  seat1 BB(100): Ks Kh  seat2(200): Qs Qh  seat3 BTN(200): Ts Th
// 公共牌：2s 7d Jc 4h 9s
// 池：主池 200（四人各 50，seat0 赢），边池 150（seat1/2/3 各 50，seat1 赢），边池 200（seat2/3 各 100，seat2 的 QQ 赢）
func TestAllInSidePots(t *testing.T) {
	deck := cards.ParseList("As Ks Qs Ts Ah Kh Qh Th 3c 2s 7d Jc 5c 4h 6c 9s")
	e := NewWithDeck([]int{50, 100, 200, 200}, 3, Config{}, deck)

	mustAct(t, e, Action{Type: ActionRaise, Amount: 200}) // seat2 UTG 全下
	mustAct(t, e, Action{Type: ActionCall})               // seat3 按钮跟注全下
	mustAct(t, e, Action{Type: ActionCall})               // seat0 SB 跟注全下(49)
	mustAct(t, e, Action{Type: ActionCall})               // seat1 BB 跟注全下(98)

	if !e.Over() {
		t.Fatal("hand should be over after all-in runout")
	}
	if len(e.Board()) != 5 {
		t.Fatalf("board = %v, want 5 cards", e.Board())
	}
	assertNets(t, e, []int{150, 50, 0, -200})

	pots := e.Pots()
	if len(pots) != 3 {
		t.Fatalf("pots = %+v, want 3", pots)
	}
	want := []Pot{
		{Amount: 200, Winners: []int{0}},
		{Amount: 150, Winners: []int{1}},
		{Amount: 200, Winners: []int{2}},
	}
	for i, w := range want {
		if pots[i].Amount != w.Amount || len(pots[i].Winners) != 1 || pots[i].Winners[0] != w.Winners[0] {
			t.Fatalf("pot %d = %+v, want %+v", i, pots[i], w)
		}
	}
	for _, r := range e.Results() {
		if !r.Showdown {
			t.Fatalf("seat %d should have shown down", r.Seat)
		}
	}
}

// 公共牌成皇家同花顺，所有人玩板面分池；奇数零头给按钮位后第一个赢家。
// button=0：seat0 跟注 2，seat1(SB) 弃牌（损失 1），seat2(BB) 过牌，之后全过。
// 底池 5，seat0/seat2 平分 → 各 2 余 1，按钮后顺序为 seat1(fold)、seat2 → 零头给 seat2。
func TestSplitPotOddChip(t *testing.T) {
	deck := cards.ParseList("4h 6h 2h 5d 7d 3d 8h As Ks Qs 9h Js Tc Ts")
	e := NewWithDeck([]int{100, 100, 100}, 0, Config{}, deck)

	mustAct(t, e, Action{Type: ActionCall})  // seat0
	mustAct(t, e, Action{Type: ActionFold})  // seat1 SB
	mustAct(t, e, Action{Type: ActionCheck}) // seat2 BB
	for i := 0; i < 6; i++ {                 // flop/turn/river 双方过牌
		mustAct(t, e, Action{Type: ActionCheck})
	}

	if !e.Over() {
		t.Fatal("hand should be over")
	}
	assertNets(t, e, []int{0, -1, 1})
	// SB 弃牌的 1 筹码使底池分两层：3（三人各 1）+ 2（seat0/seat2 各 1）
	pots := e.Pots()
	if len(pots) != 2 || pots[0].Amount+pots[1].Amount != 5 {
		t.Fatalf("pots = %+v, want total 5", pots)
	}
	for _, p := range pots {
		if len(p.Winners) != 2 {
			t.Fatalf("winners = %v, want 2", p.Winners)
		}
	}
}

// 最小加注规则：翻前 BB=2，最小 raise-to 为 4；加到 4 后最小 raise-to 为 6。
func TestMinRaiseEnforced(t *testing.T) {
	e := New([]int{100, 100, 100}, 0, Config{}, rand.New(rand.NewSource(2)))
	if err := e.Act(Action{Type: ActionRaise, Amount: 3}); err == nil {
		t.Fatal("raise to 3 should be rejected (min 4)")
	}
	if err := e.Act(Action{Type: ActionRaise, Amount: 101}); err == nil {
		t.Fatal("raise above stack should be rejected")
	}
	mustAct(t, e, Action{Type: ActionRaise, Amount: 4})
	la := e.LegalActions() // seat1 SB
	if la.MinRaiseTo != 6 {
		t.Fatalf("MinRaiseTo = %d, want 6", la.MinRaiseTo)
	}
	if la.CallAmount != 3 {
		t.Fatalf("CallAmount = %d, want 3", la.CallAmount)
	}
	mustAct(t, e, Action{Type: ActionFold})
	mustAct(t, e, Action{Type: ActionFold}) // BB 弃牌，seat0 收池
	assertNets(t, e, []int{3, -1, -2})
}

// 翻后下注吓退所有人，直接收池不摊牌。
func TestFlopBetTakeDown(t *testing.T) {
	deck := cards.ParseList("2h 4h 6h 8d Td 3d 5h 7s 9s Js Qc 2d 3c 4c")
	e := NewWithDeck([]int{100, 100, 100}, 0, Config{}, deck)
	call(e)  // seat0
	call(e)  // seat1 SB
	check(e) // seat2 BB
	if e.Street() != Flop {
		t.Fatalf("street = %v, want flop", e.Street())
	}
	// 翻后从按钮后第一个存活玩家开始：seat1
	if e.CurrentActor() != 1 {
		t.Fatalf("flop first actor = %d, want 1", e.CurrentActor())
	}
	mustAct(t, e, Action{Type: ActionBet, Amount: 10}) // seat1
	mustAct(t, e, Action{Type: ActionFold})            // seat2
	mustAct(t, e, Action{Type: ActionFold})            // seat0
	if !e.Over() {
		t.Fatal("hand should be over")
	}
	assertNets(t, e, []int{-2, 4, -2})
	if e.Results()[1].Showdown {
		t.Fatal("winner by fold should not be marked showdown")
	}
}

// 双人桌全下后直接发完公共牌摊牌，不再产生行动。
func TestHeadsUpAllInRunout(t *testing.T) {
	// seat0 BTN/SB: As Ah；seat1 BB: Ks Kh；公共牌 2s 7d Jc 4h 9s
	deck := cards.ParseList("As Ks Ah Kh 3c 2s 7d Jc 5c 4h 6c 9s")
	e := NewWithDeck([]int{50, 50}, 0, Config{}, deck)
	if e.CurrentActor() != 0 {
		t.Fatalf("heads-up preflop first actor = %d, want button 0", e.CurrentActor())
	}
	mustAct(t, e, Action{Type: ActionRaise, Amount: 50}) // 全下
	mustAct(t, e, Action{Type: ActionCall})
	if !e.Over() {
		t.Fatal("hand should be over")
	}
	if len(e.Board()) != 5 {
		t.Fatalf("board should be run out, got %v", e.Board())
	}
	assertNets(t, e, []int{50, -50})
	if err := e.Act(Action{Type: ActionFold}); err == nil {
		t.Fatal("acting after hand over should fail")
	}
}

// sub-minimum 全下不重新开放下注：已行动过的玩家只能跟注/弃牌。
func TestSubMinAllInDoesNotReopen(t *testing.T) {
	// 三人桌 stacks [100, 3, 100]，button=0。seat1 SB 短筹码（注 1 余 2）。
	e := New([]int{100, 3, 100}, 0, Config{}, rand.New(rand.NewSource(3)))
	mustAct(t, e, Action{Type: ActionCall}) // seat0
	// seat1 SB 全下到 3：增量 1 < 最小加注 2，属 sub-minimum
	la := e.LegalActions()
	if la.MaxRaiseTo != 3 {
		t.Fatalf("SB MaxRaiseTo = %d, want 3", la.MaxRaiseTo)
	}
	if la.MinRaiseTo != 4 {
		t.Fatalf("SB MinRaiseTo = %d, want 4", la.MinRaiseTo)
	}
	mustAct(t, e, Action{Type: ActionRaise, Amount: 3})
	// seat2 BB 尚未行动，可以再加注
	if !e.LegalActions().CanRaise {
		t.Fatal("BB has not acted, should be able to raise")
	}
	mustAct(t, e, Action{Type: ActionFold})
	// seat0 已行动过，面对 sub-minimum 全下只能跟注/弃牌
	la = e.LegalActions()
	if la.CanRaise {
		t.Fatal("seat0 already acted; sub-min all-in should not reopen")
	}
	if la.CallAmount != 1 {
		t.Fatalf("seat0 CallAmount = %d, want 1", la.CallAmount)
	}
	mustAct(t, e, Action{Type: ActionFold})
	assertNets(t, e, []int{-2, 4, -2})
}

// 筹码不足时的全下盲注不阻塞流程。
func TestAllInBlinds(t *testing.T) {
	e := New([]int{100, 1, 2}, 0, Config{}, rand.New(rand.NewSource(4)))
	// SB=1 全下 1，BB=2 全下 2，只剩 seat0 可行动
	if e.CurrentActor() != 0 {
		t.Fatalf("actor = %d, want 0", e.CurrentActor())
	}
	mustAct(t, e, Action{Type: ActionCall})
	if !e.Over() {
		t.Fatal("hand should run out and finish")
	}
	sum := 0
	for _, r := range e.Results() {
		sum += r.Net
	}
	if sum != 0 {
		t.Fatalf("nets sum = %d", sum)
	}
}

// 随机合法动作 smoke test：数千手，验证筹码守恒、状态机收敛、无 panic。
func TestSmokeRandomHands(t *testing.T) {
	const hands = 5000
	for seed := int64(0); seed < hands; seed++ {
		rng := rand.New(rand.NewSource(seed))
		n := 2 + rng.Intn(8) // 2..9 人
		stacks := make([]int, n)
		active := 0
		for i := range stacks {
			if rng.Intn(10) == 0 && active >= 2 && i > 1 {
				stacks[i] = 0 // 偶尔空手座位
				continue
			}
			stacks[i] = 5 + rng.Intn(296) // 5..300，覆盖短筹码全下
			active++
		}
		button := NextButton(rng.Intn(n), stacks)
		total := 0
		for _, s := range stacks {
			total += s
		}

		e := New(stacks, button, Config{SmallBlind: 1, BigBlind: 2}, rng)
		steps := 0
		for !e.Over() {
			steps++
			if steps > 10000 {
				t.Fatalf("seed %d: hand did not terminate", seed)
			}
			la := e.LegalActions()
			act := randomLegalAction(rng, e, la)
			if err := e.Act(act); err != nil {
				t.Fatalf("seed %d: illegal action %+v (la=%+v): %v", seed, act, la, err)
			}
		}
		final := e.FinalStacks()
		sum := 0
		for _, s := range final {
			if s < 0 {
				t.Fatalf("seed %d: negative stack", seed)
			}
			sum += s
		}
		if sum != total {
			t.Fatalf("seed %d: chips not conserved: before=%d after=%d", seed, total, sum)
		}
		nsum := 0
		for _, r := range e.Results() {
			nsum += r.Net
		}
		if nsum != 0 {
			t.Fatalf("seed %d: nets sum = %d", seed, nsum)
		}
	}
}

func randomLegalAction(rng *rand.Rand, e *Engine, la LegalActions) Action {
	var opts []Action
	if la.CanCheck {
		opts = append(opts, Action{Type: ActionCheck})
	} else {
		opts = append(opts, Action{Type: ActionFold})
	}
	if la.CallAmount > 0 {
		opts = append(opts, Action{Type: ActionCall})
	}
	if la.CanRaise {
		minTo, maxTo := la.MinRaiseTo, la.MaxRaiseTo
		if minTo > maxTo {
			minTo = maxTo
		}
		to := minTo
		if maxTo > minTo {
			to = minTo + rng.Intn(maxTo-minTo+1)
		}
		typ := ActionRaise
		if e.CurrentBet() == 0 {
			typ = ActionBet
		}
		opts = append(opts, Action{Type: typ, Amount: to})
	}
	return opts[rng.Intn(len(opts))]
}
