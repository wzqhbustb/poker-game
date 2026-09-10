// sim 让 8 个内置 persona 互打 N 手，输出各 persona 的统计指标用于验证风格与强度。
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"time"

	"pocker/server/internal/bot"
	"pocker/server/internal/engine"
)

type stat struct {
	hands      int // 参与的手数
	vpip       int
	pfr        int
	betsRaises int // 翻后下注+加注
	calls      int // 翻后跟注
	net        int // 累计净盈亏（筹码）
}

func main() {
	handsN := flag.Int("hands", 1000, "手数")
	iters := flag.Int("iters", 300, "每次权益估算的蒙特卡洛迭代次数")
	seed := flag.Int64("seed", 42, "随机种子")
	buyin := flag.Int("buyin", 200, "起始/补充筹码（100bb）")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))
	personas := bot.Personas()
	n := len(personas)
	stats := make([]stat, n)
	stacks := make([]int, n)
	for i := range stacks {
		stacks[i] = *buyin
	}
	bot.EquityIters = *iters
	button := 0
	start := time.Now()

	for h := 0; h < *handsN; h++ {
		// 每手重置筹码到起始买入：隔离手间运气传导（深筹雪球效应），
		// 使每手盈亏可独立比较，这是 AI 基准的标准做法。
		for i := range stacks {
			stacks[i] = *buyin
		}
		button = engine.NextButton(button, stacks)
		e := engine.New(stacks, button, engine.Config{}, rng)

		// 记录本手每座位的翻前/翻后动作，结束后计入统计
		type marks struct {
			vpip, pfr bool
		}
		mk := make([]marks, n)
		played := make([]bool, n)

		for !e.Over() {
			seat := e.CurrentActor()
			played[seat] = true
			v := bot.ViewFromEngine(e, seat)
			a := bot.Decide(v, personas[seat].Style, rng)
			if v.Street == engine.Preflop {
				switch a.Type {
				case engine.ActionCall, engine.ActionBet, engine.ActionRaise:
					mk[seat].vpip = true
				}
				if a.Type == engine.ActionBet || a.Type == engine.ActionRaise {
					mk[seat].pfr = true
				}
			} else {
				switch a.Type {
				case engine.ActionBet, engine.ActionRaise:
					stats[seat].betsRaises++
				case engine.ActionCall:
					stats[seat].calls++
				}
			}
			if err := e.Act(a); err != nil {
				fmt.Fprintf(os.Stderr, "hand %d: %v\n", h, err)
				os.Exit(1)
			}
		}

		for i, r := range e.Results() {
			stats[i].net += r.Net
		}
		for i := 0; i < n; i++ {
			if played[i] {
				stats[i].hands++
				if mk[i].vpip {
					stats[i].vpip++
				}
				if mk[i].pfr {
					stats[i].pfr++
				}
			}
		}
		stacks = e.FinalStacks()
	}

	el := time.Since(start)
	fmt.Printf("simulated %d hands in %s (%.1f hands/s)\n\n", *handsN, el.Round(time.Millisecond), float64(*handsN)/el.Seconds())
	fmt.Printf("%-10s %-12s %6s %6s %6s %8s %9s\n", "ID", "风格", "VPIP", "PFR", "AF", "bb/100", "总盈亏")
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return stats[order[i]].net > stats[order[j]].net })
	for _, i := range order {
		s := stats[i]
		af := 0.0
		if s.calls > 0 {
			af = float64(s.betsRaises) / float64(s.calls)
		}
		bb100 := 0.0
		if s.hands > 0 {
			bb100 = float64(s.net) / float64(s.hands) * 100 / 2
		}
		fmt.Printf("%-10s %-12s %5.1f%% %5.1f%% %6.2f %8.1f %9d\n",
			personas[i].ID, personas[i].StyleTag,
			float64(s.vpip)/float64(max(s.hands, 1))*100,
			float64(s.pfr)/float64(max(s.hands, 1))*100,
			af, bb100, s.net)
	}
}
