package bot

import (
	"math/rand"
	"testing"

	"pocker/server/internal/cards"
	"pocker/server/internal/engine"
)

func TestEquityKnownSpots(t *testing.T) {
	cases := []struct {
		name   string
		hole   string
		board  string
		opp    int
		lo, hi float64
	}{
		{"AA preflop vs 1", "As Ad", "", 1, 0.80, 0.90},
		{"AA preflop vs 8", "As Ad", "", 8, 0.22, 0.45},
		{"72o preflop vs 1", "7h 2d", "", 1, 0.25, 0.45},
		{"flush draw on flop", "Ah Kh", "Th 7h 2d", 1, 0.55, 0.75}, // 两高张+同花听
		{"set on dry flop", "8s 8d", "8h Kd 2c", 1, 0.90, 1.0},
		{"pair vs overcards flop", "9s 9d", "7h 2d 3c", 1, 0.75, 0.95},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(42))
			var board []cards.Card
			if tc.board != "" {
				board = cards.ParseList(tc.board)
			}
			eq := EquityN(cards.ParseList(tc.hole), board, tc.opp, 3000, rng)
			if eq < tc.lo || eq > tc.hi {
				t.Errorf("equity %.3f not in [%.2f, %.2f]", eq, tc.lo, tc.hi)
			}
		})
	}
}

func TestPercentileOrdering(t *testing.T) {
	aa := Percentile(cards.ParseList("As Ad"))
	seven2 := Percentile(cards.ParseList("7h 2d"))
	ako := Percentile(cards.ParseList("As Kd"))
	if aa < 0.99 {
		t.Errorf("AA percentile = %.3f, want >= 0.99", aa)
	}
	if seven2 > 0.10 {
		t.Errorf("72o percentile = %.3f, want <= 0.10", seven2)
	}
	if !(aa > ako && ako > seven2) {
		t.Errorf("ordering wrong: AA %.3f AKo %.3f 72o %.3f", aa, ako, seven2)
	}
}

func TestClassifyPosition(t *testing.T) {
	// 9 人桌：rel 0=BTN 1=SB 2=BB 3=UTG ... 7=HJ 8=CO
	want := map[int]positionClass{
		0: posButton, 1: posSmallBlind, 2: posBigBlind,
		3: posEarly, 4: posEarly, 5: posEarly,
		6: posEarly, 7: posHijack, 8: posCutoff,
	}
	for rel, pc := range want {
		if got := classifyPosition(rel, 9); got != pc {
			t.Errorf("classifyPosition(%d,9) = %v, want %v", rel, got, pc)
		}
	}
}

// TestDecideLegalFuzz 随机风格全程互打，断言每个决策都合法、每手都能收敛。
func TestDecideLegalFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	personas := Personas()
	for hand := 0; hand < 300; hand++ {
		stacks := make([]int, 9)
		for i := range stacks {
			stacks[i] = 50 + rng.Intn(250)
		}
		e := engine.New(stacks, rng.Intn(9), engine.Config{}, rng)
		steps := 0
		for !e.Over() {
			steps++
			if steps > 10000 {
				t.Fatal("hand did not converge")
			}
			seat := e.CurrentActor()
			// 随机风格（不限于内置 persona）
			st := personas[rng.Intn(len(personas))].Style
			if hand%3 == 0 {
				st = Style{VPIP: rng.Float64(), PFR: rng.Float64(), ThreeBet: rng.Float64(),
					Aggression: rng.Float64(), BluffFreq: rng.Float64(), CallTendency: rng.Float64()}
			}
			v := ViewFromEngine(e, seat)
			a := Decide(v, st, rng)
			if err := e.Act(a); err != nil {
				t.Fatalf("hand %d step %d seat %d: illegal action %+v: %v (legal=%+v view=%+v)",
					hand, steps, seat, a, err, v.Legal, v)
			}
		}
	}
}

func TestDetectDraw(t *testing.T) {
	if got := detectDraw(cards.ParseList("Ah Kh"), cards.ParseList("Th 7d 2h")); got != flushDraw {
		t.Errorf("flush draw not detected: %v", got)
	}
	if got := detectDraw(cards.ParseList("9s 8d"), cards.ParseList("7h 6c 2d")); got != straightDraw {
		t.Errorf("straight draw not detected: %v", got)
	}
	if got := detectDraw(cards.ParseList("As Kd"), cards.ParseList("Qh 7c 2d")); got != noDraw {
		t.Errorf("expected no draw, got %v", got)
	}
	if got := detectDraw(cards.ParseList("Ah Kh"), cards.ParseList("Th 7h 2h 3d 4c")); got != noDraw {
		t.Errorf("river should have no draw, got %v", got)
	}
}

func TestViewFromEngineNoLeaks(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	stacks := []int{100, 100, 100, 100, 100, 100, 100, 100, 100}
	e := engine.New(stacks, 0, engine.Config{}, rng)
	seat := e.CurrentActor()
	v := ViewFromEngine(e, seat)
	if len(v.Hole) != 2 {
		t.Fatalf("hole len = %d", len(v.Hole))
	}
	if v.NumOpponents != 8 || v.NumActive != 9 {
		t.Errorf("opp=%d active=%d, want 8/9", v.NumOpponents, v.NumActive)
	}
	if v.BigBlind != 2 {
		t.Errorf("bb = %d, want 2", v.BigBlind)
	}
}
