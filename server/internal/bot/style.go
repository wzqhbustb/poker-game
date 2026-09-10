package bot

// Style 风格参数，全部归一化到 [0,1]。
// 参数不直接决定动作，而是对策略核心的标准打法做倾向性偏移：
// 同一局面下风格决定倾向，实际动作仍由 rng 采样，避免机械化。
type Style struct {
	// VPIP 自愿入池率倾向。以 0.25 为中性值，放宽/收紧翻前各位置的入池范围档位。
	VPIP float64
	// PFR 翻前加注倾向。入池时以加注（而非跛入/跟注）进入的概率参考为 PFR/VPIP。
	PFR float64
	// ThreeBet 面对加注时的再加注（3bet+）频率，扩大/收窄价值 3bet 范围。
	ThreeBet float64
	// Aggression 翻后激进度：主动下注/加注 vs 过牌/跟注的倾向，并偏向更大下注尺度。
	Aggression float64
	// BluffFreq 诈唬频率：纯诈唬与半诈唬（听牌）下注/加注的倾向。
	BluffFreq float64
	// CallTendency 跟注倾向：把边缘 fold 转成 call、把边缘 raise 转成 call 的程度。
	CallTendency float64
}

// clamped 防御性地把各参数收敛到 [0,1]。
func (s Style) clamped() Style {
	s.VPIP = clamp01(s.VPIP)
	s.PFR = clamp01(s.PFR)
	s.ThreeBet = clamp01(s.ThreeBet)
	s.Aggression = clamp01(s.Aggression)
	s.BluffFreq = clamp01(s.BluffFreq)
	s.CallTendency = clamp01(s.CallTendency)
	return s
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
