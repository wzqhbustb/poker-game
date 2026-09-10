package table

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"pocker/server/internal/bot"
	"pocker/server/internal/proto"
	"pocker/server/internal/store"
)

// recv 带超时从客户端 channel 收一条消息。
func recv(t *testing.T, c *Client) any {
	t.Helper()
	select {
	case m := <-c.Send:
		return m
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for server message")
		return nil
	}
}

// TestPlayHands 集成测试：假人类打若干完整手，断言协议消息流与落盘。
func TestPlayHands(t *testing.T) {
	bot.EquityIters = 100 // 测试提速
	db, err := store.Open(filepath.Join(t.TempDir(), "poker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tbl := New(Config{Fast: true, Seed: 42, ActionTimeout: 5 * time.Second}, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	c := &Client{Name: "tester", Send: make(chan any, 65536)}
	tbl.Attach(c)

	// 第一条应是 welcome
	m := recv(t, c)
	if w, ok := m.(proto.Welcome); !ok || w.Seat != HumanSeat || w.Config.Seats != Seats {
		t.Fatalf("first message = %+v, want welcome seat 0", m)
	}

	handsDone := 0
	gotActionRequest := false
	for handsDone < 3 {
		m := recv(t, c)
		switch msg := m.(type) {
		case proto.ActionRequest:
			gotActionRequest = true
			// 能过牌则过牌，否则弃牌
			if msg.Legal.CanCheck {
				tbl.SubmitAction(c, "check", 0)
			} else {
				tbl.SubmitAction(c, "fold", 0)
			}
		case proto.State:
			// 隐私：手牌进行中其他座位不得暴露底牌
			if msg.InHand {
				for i, s := range msg.Seats {
					if i != HumanSeat && len(s.Hole) > 0 {
						t.Fatalf("state leaks seat %d hole %v mid-hand", i, s.Hole)
					}
				}
			}
		case proto.HandEnd:
			handsDone++
			if msg.HandID <= 0 {
				t.Error("hand_end missing hand_id")
			}
		}
	}
	if !gotActionRequest {
		t.Error("human never received action_request")
	}

	// 落盘验证
	list, err := db.ListHands(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 3 {
		t.Fatalf("only %d hands persisted", len(list))
	}
	full, err := db.GetHand(list[0].ID)
	if err != nil || full == nil {
		t.Fatalf("GetHand: %v %v", full, err)
	}
	if len(full.Actions) == 0 || len(full.Players) == 0 {
		t.Errorf("persisted hand incomplete: %d actions, %d players", len(full.Actions), len(full.Players))
	}
	// 筹码守恒
	sum := 0
	for _, p := range full.Players {
		sum += p.Net
	}
	if sum != 0 {
		t.Errorf("persisted hand nets sum to %d, want 0", sum)
	}
}

// TestTimeoutAutoAction 人类不行动时超时自动 check/fold，手牌照常推进。
func TestTimeoutAutoAction(t *testing.T) {
	bot.EquityIters = 100
	tbl := New(Config{Fast: true, Seed: 7, ActionTimeout: 200 * time.Millisecond}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	c := &Client{Name: "slow", Send: make(chan any, 65536)}
	tbl.Attach(c)
	if _, ok := recv(t, c).(proto.Welcome); !ok {
		t.Fatal("want welcome")
	}
	// 不提交任何动作，等两手结束
	done := 0
	for done < 2 {
		if _, ok := recv(t, c).(proto.HandEnd); ok {
			done++
		}
	}
}

// TestRebuy 人类筹码不足时 rebuy 下一手补回买入。
func TestRebuy(t *testing.T) {
	bot.EquityIters = 100
	tbl := New(Config{Fast: true, Seed: 3, ActionTimeout: 50 * time.Millisecond}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tbl.Run(ctx)

	c := &Client{Name: "r", Send: make(chan any, 65536)}
	tbl.Attach(c)
	recv(t, c) // welcome
	tbl.Rebuy(c)

	// 等到下一手开始后的 state，人类筹码应为买入 200
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		m := recv(t, c)
		if s, ok := m.(proto.State); ok && s.InHand {
			if s.Seats[HumanSeat].Stack+s.Seats[HumanSeat].Bet+s.Seats[HumanSeat].Bet == 0 {
				t.Fatal("human not seated")
			}
			// 起始 200，无论是否下盲注 stack+bet 应反映从 200 开始
			if s.Seats[HumanSeat].Stack+s.Seats[HumanSeat].Bet >= 195 {
				return
			}
		}
	}
	t.Fatal("human stack not topped up")
}
