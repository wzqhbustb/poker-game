package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func sampleHand() *Hand {
	return &Hand{
		Button: 3, SmallBlind: 1, BigBlind: 2, Seed: 42, Board: "As Kd 7c 2h 9s",
		Players: []PlayerRecord{
			{Seat: 0, Name: "You", IsBot: false, StartStack: 200, EndStack: 210, Net: 10, Hole: "Ah Kh"},
			{Seat: 1, Name: "Taylor", IsBot: true, StartStack: 200, EndStack: 190, Net: -10, Hole: "Qc Jc"},
		},
		Actions: []ActionRecord{
			{Street: "preflop", Seat: 0, Type: "raise", Amount: 4, To: 6},
			{Street: "preflop", Seat: 1, Type: "call", Amount: 4, To: 6},
			{Street: "flop", Seat: 1, Type: "check", Amount: 0, To: 0},
			{Street: "flop", Seat: 0, Type: "bet", Amount: 8, To: 8},
			{Street: "flop", Seat: 1, Type: "call", Amount: 8, To: 8},
		},
	}
}

func TestSaveAndGetRoundtrip(t *testing.T) {
	db := openTemp(t)
	h := sampleHand()
	id, err := db.SaveHand(h)
	if err != nil {
		t.Fatalf("SaveHand: %v", err)
	}
	got, err := db.GetHand(id)
	if err != nil {
		t.Fatalf("GetHand: %v", err)
	}
	if got == nil {
		t.Fatal("GetHand returned nil")
	}
	if got.Board != h.Board || got.Button != h.Button || got.Seed != 42 {
		t.Errorf("hand fields mismatch: %+v", got)
	}
	if len(got.Players) != 2 || got.Players[0].Hole != "Ah Kh" {
		t.Errorf("players mismatch: %+v", got.Players)
	}
	if len(got.Actions) != 5 || got.Actions[1].Type != "call" || got.Actions[4].To != 8 {
		t.Errorf("actions mismatch: %+v", got.Actions)
	}
}

func TestListHandsOrdering(t *testing.T) {
	db := openTemp(t)
	for i := 0; i < 3; i++ {
		if _, err := db.SaveHand(sampleHand()); err != nil {
			t.Fatal(err)
		}
	}
	list, err := db.ListHands(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d hands", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].ID < list[i].ID {
			t.Error("ListHands not in descending id order")
		}
	}
	if len(list[0].Actions) != 0 {
		t.Error("ListHands should not include actions")
	}
}

func TestGetHandMissing(t *testing.T) {
	db := openTemp(t)
	h, err := db.GetHand(999)
	if err != nil || h != nil {
		t.Errorf("want (nil,nil), got (%v,%v)", h, err)
	}
}

func TestStats(t *testing.T) {
	db := openTemp(t)
	// 手 1：人类翻前 raise + 翻后 bet 被跟（VPIP=是, PFR=是）
	if _, err := db.SaveHand(sampleHand()); err != nil {
		t.Fatal(err)
	}
	// 手 2：人类翻前 fold，不参与统计中的 VPIP
	h2 := sampleHand()
	h2.Players[0].Net = -1 // 大盲损失
	h2.Players[1].Net = 1
	h2.Actions = []ActionRecord{
		{Street: "preflop", Seat: 0, Type: "fold"},
		{Street: "flop", Seat: 1, Type: "bet", Amount: 4, To: 4},
	}
	if _, err := db.SaveHand(h2); err != nil {
		t.Fatal(err)
	}

	s, err := db.Stats(0)
	if err != nil {
		t.Fatal(err)
	}
	if s.Hands != 2 {
		t.Errorf("Hands = %d, want 2", s.Hands)
	}
	if s.VPIP != 0.5 {
		t.Errorf("VPIP = %v, want 0.5", s.VPIP)
	}
	if s.PFR != 0.5 {
		t.Errorf("PFR = %v, want 0.5", s.PFR)
	}
	if s.BetsRaises != 1 || s.Calls != 0 {
		t.Errorf("bets/raises=%d calls=%d, want 1/0", s.BetsRaises, s.Calls)
	}
	if s.NetProfit != 9 {
		t.Errorf("NetProfit = %d, want 9", s.NetProfit)
	}
}
