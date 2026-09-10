package cards

import (
	"math/rand"
	"testing"
)

func TestCardStringParse(t *testing.T) {
	for _, s := range []string{"As", "Td", "2c", "Kh", "9s"} {
		c := MustParse(s)
		if got := c.String(); got != s {
			t.Fatalf("Parse(%q).String() = %q", s, got)
		}
	}
	if _, err := Parse("XX"); err == nil {
		t.Fatal("expected error for bad card")
	}
	if _, err := Parse("A"); err == nil {
		t.Fatal("expected error for short card")
	}
}

func TestDeckHas52Unique(t *testing.T) {
	d := Deck()
	if len(d) != 52 {
		t.Fatalf("deck size = %d", len(d))
	}
	seen := map[Card]bool{}
	for _, c := range d {
		if seen[c] {
			t.Fatalf("duplicate card %v", c)
		}
		seen[c] = true
	}
}

func TestShuffleDeterministic(t *testing.T) {
	d1, d2 := Deck(), Deck()
	Shuffle(d1, rand.New(rand.NewSource(42)))
	Shuffle(d2, rand.New(rand.NewSource(42)))
	for i := range d1 {
		if d1[i] != d2[i] {
			t.Fatal("same seed should give same shuffle")
		}
	}
	seen := map[Card]bool{}
	for _, c := range d1 {
		seen[c] = true
	}
	if len(seen) != 52 {
		t.Fatal("shuffle lost cards")
	}
}

// TestEvaluateChain 验证牌型强弱链：同花顺>四条>葫芦>同花>顺子>三条>两对>一对>高牌。
func TestEvaluateChain(t *testing.T) {
	hands := []string{
		"2c 3d 5h 7s 9c", // 高牌
		"2c 2d 5h 7s 9c", // 一对
		"2c 2d 5h 5s 9c", // 两对
		"2c 2d 2h 5s 9c", // 三条
		"2c 3d 4h 5s 6c", // 顺子
		"2s 5s 7s 9s Js", // 同花
		"2c 2d 2h 5s 5c", // 葫芦
		"2c 2d 2h 2s 9c", // 四条
		"5s 6s 7s 8s 9s", // 同花顺
		"Ts Js Qs Ks As", // 皇家同花顺
	}
	scores := make([]Score, len(hands))
	for i, h := range hands {
		scores[i] = Evaluate(ParseList(h))
	}
	for i := 0; i+1 < len(scores); i++ {
		if scores[i] >= scores[i+1] {
			t.Fatalf("%q (%v) should be weaker than %q (%v)",
				hands[i], scores[i].Category(), hands[i+1], scores[i+1].Category())
		}
	}
}

func TestEvaluateCategories(t *testing.T) {
	cases := map[string]Category{
		"As Kd Qh Jc 9s": HighCard,
		"As Ah Qd 7c 2s": Pair,
		"As Ah Qd Qc 2s": TwoPair,
		"As Ah Ad Qc 2s": Trips,
		"As Kd Qh Jc Ts": Straight,
		"As 5h 4h 3h 2h": Straight, // 轮子
		"As Ks Qs 9s 2s": Flush,
		"As Ah Ad Kc Ks": FullHouse,
		"As Ah Ad Ac Ks": Quads,
		"As Ks Qs Js Ts": StraightFlush,
	}
	for h, want := range cases {
		if got := Evaluate(ParseList(h)).Category(); got != want {
			t.Fatalf("%q category = %v, want %v", h, got, want)
		}
	}
}

func TestEvaluateKickers(t *testing.T) {
	cases := []struct {
		name           string
		weaker, strong string
	}{
		{"pair kicker", "As Ah Qd 7c 2s", "As Ah Kd 7c 2s"},
		{"pair rank", "Ks Kh Qd 7c 2s", "As Ah 3d 4c 5s"},
		{"two pair kicker", "As Ah Kd Kc 2s", "As Ah Kd Kc 3s"},
		{"two pair second pair", "As Ah Qd Qc Ks", "As Ah Kd Kc 2s"},
		{"trips kicker", "As Ah Ad Kc 2s", "As Ah Ad Kc 3s"},
		{"straight high", "2c 3d 4h 5s 6c", "3c 4d 5h 6s 7c"},
		{"wheel loses to 6-high", "As 2d 3h 4s 5c", "2c 3d 4h 5s 6c"},
		{"flush kicker", "As Ks Qs 9s 2s", "As Ks Qs Ts 2s"},
		{"full house trips first", "Ks Kh Kd Ac As", "As Ah Ad 2c 2s"},
		{"quads kicker", "As Ah Ad Ac 2s", "As Ah Ad Ac 3s"},
		{"straight flush high", "5s 6s 7s 8s 9s", "6s 7s 8s 9s Ts"},
	}
	for _, c := range cases {
		w := Evaluate(ParseList(c.weaker))
		s := Evaluate(ParseList(c.strong))
		if Compare(w, s) != -1 {
			t.Fatalf("%s: %q should lose to %q", c.name, c.weaker, c.strong)
		}
	}
}

func TestEvaluateSplit(t *testing.T) {
	a := Evaluate(ParseList("As Ah Kd Qc 2s"))
	b := Evaluate(ParseList("Ad Ac Kh Qs 2d"))
	if Compare(a, b) != 0 {
		t.Fatal("identical ranks should tie")
	}
}

func TestEvaluateSevenPicksBestFive(t *testing.T) {
	// 7 张里有顺子和一对，应选出顺子
	s := Evaluate(ParseList("As Ah 2d 3c 4s 5h 9d"))
	if s.Category() != Straight {
		t.Fatalf("expected straight, got %v", s.Category())
	}
	// 板面成葫芦，但应选更大的葫芦（葫芦先比三条的 rank）
	s = Evaluate(ParseList("As Ad Ac Kh Kd 2s 3c")) // A 葫芦
	if s.Category() != FullHouse {
		t.Fatalf("expected full house, got %v", s.Category())
	}
	worse := Evaluate(ParseList("Ks Kc Kd As Ah 2d 3c")) // K 葫芦
	if Compare(worse, s) >= 0 {
		t.Fatal("aces full of kings should beat kings full of aces")
	}
	// 6 张输入
	s6 := Evaluate(ParseList("As Ks Qs Js Ts 2d"))
	if s6.Category() != StraightFlush {
		t.Fatalf("6-card: expected straight flush, got %v", s6.Category())
	}
	// 与纯 5 张结果一致
	if s6 != Evaluate(ParseList("As Ks Qs Js Ts")) {
		t.Fatal("6-card eval should pick the best 5")
	}
}

func BenchmarkEvaluate7(b *testing.B) {
	hand := ParseList("As Kh Qd Jc Ts 9s 2h")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Evaluate(hand)
	}
}
