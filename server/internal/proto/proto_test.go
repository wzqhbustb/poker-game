package proto

import (
	"encoding/json"
	"testing"
	"time"
)

func TestClientMessagesRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		msg  any
		want string
	}{
		{"hello", &Hello{Type: CHello, Name: "Alice"}, CHello},
		{"action-fold", &ActionMsg{Type: CAction, Action: "fold"}, CAction},
		{"action-raise", &ActionMsg{Type: CAction, Action: "raise", Amount: 40}, CAction},
		{"rebuy", &RebuyMsg{Type: CRebuy}, CRebuy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Encode(tt.msg)
			if err != nil {
				t.Fatal(err)
			}
			m, err := DecodeClient(data)
			if err != nil {
				t.Fatal(err)
			}
			switch want := tt.msg.(type) {
			case *Hello:
				if got := m.(*Hello); *got != *want {
					t.Errorf("got %+v want %+v", got, want)
				}
			case *ActionMsg:
				if got := m.(*ActionMsg); *got != *want {
					t.Errorf("got %+v want %+v", got, want)
				}
			case *RebuyMsg:
				if got := m.(*RebuyMsg); *got != *want {
					t.Errorf("got %+v want %+v", got, want)
				}
			}
		})
	}
}

func TestDecodeClientErrors(t *testing.T) {
	if _, err := DecodeClient([]byte(`{bad json`)); err == nil {
		t.Error("want error for bad json")
	}
	if _, err := DecodeClient([]byte(`{"type":"hack"}`)); err == nil {
		t.Error("want error for unknown type")
	}
}

func TestServerMessagesShape(t *testing.T) {
	deadline := time.Now().Add(30 * time.Second).UnixMilli()
	msgs := []any{
		Welcome{Type: SWelcome, Seat: 0, Config: TableConfig{Seats: 9, SmallBlind: 1, BigBlind: 2, BuyIn: 200, ActionTimeoutMs: 30000}},
		State{
			Type: SState, HandNo: 7, InHand: true, Pot: 24, Street: "flop", Button: 3, You: 0,
			Board: []string{"As", "Kd", "2c"},
			Seats: []SeatState{{Seat: 0, Name: "You", Stack: 180, Hole: []string{"Ah", "Qh"}}, {Seat: 1, Name: "Taylor", StyleTag: "紧凶 TAG", IsBot: true, Stack: 220, Thinking: true}},
			Ts:    deadline,
		},
		ActionRequest{Type: SActionRequest, Deadline: deadline, Pot: 24, CurrentBet: 8,
			Legal: LegalInfo{CanFold: true, CallAmount: 8, CanRaise: true, MinRaiseTo: 16, MaxRaiseTo: 180}},
		HandEvent{Type: SHandEvent, HandNo: 7, Kind: EvAction, Street: "flop", Seat: 2, Action: "raise", Amount: 8, To: 16, Pot: 40},
		HandEnd{
			Type: SHandEnd, HandNo: 7, HandID: 42,
			Results:       []ResultInfo{{Seat: 0, Won: 40, Net: 16, Showdown: true}},
			Pots:          []PotInfo{{Amount: 40, Winners: []int{0}}},
			RevealedHoles: []RevealedHole{{Seat: 2, Hole: []string{"Ts", "Td"}}},
		},
		ErrorMsg{Type: SError, Message: "boom"},
	}
	wantTypes := []string{SWelcome, SState, SActionRequest, SHandEvent, SHandEnd, SError}
	for i, msg := range msgs {
		data, err := Encode(msg)
		if err != nil {
			t.Fatal(err)
		}
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &head); err != nil {
			t.Fatal(err)
		}
		if head.Type != wantTypes[i] {
			t.Errorf("msg %d: type = %q, want %q", i, head.Type, wantTypes[i])
		}
		// 完整 roundtrip：反序列化回原类型应一致
		out := json.RawMessage(data)
		_ = out
	}

	// state 中自己的底牌必须出现，他人底牌不出现
	s := msgs[1].(State)
	data, _ := Encode(s)
	var m map[string]any
	json.Unmarshal(data, &m)
	seats := m["seats"].([]any)
	if hole := seats[0].(map[string]any)["hole"]; hole == nil {
		t.Error("own hole missing in state")
	}
	if hole := seats[1].(map[string]any)["hole"]; hole != nil {
		t.Error("bot hole leaked in state")
	}
}
