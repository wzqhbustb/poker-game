package bot

// Persona 一个内置 Bot 人设：名字、风格标签、风格参数与思考延时范围（毫秒）。
type Persona struct {
	ID       string
	Name     string
	StyleTag string // 供前端展示的中文风格标签
	Style    Style
	ThinkMin int // 思考延时下限（毫秒），由桌子层使用
	ThinkMax int // 思考延时上限（毫秒）
}

// Personas 返回 8 个内置人设，对应 9 人桌除人类玩家外的 8 个座位。
func Personas() []Persona {
	return []Persona{
		{
			ID: "baseline", Name: "Baseline", StyleTag: "基准",
			Style:    Style{VPIP: 0.25, PFR: 0.20, ThreeBet: 0.06, Aggression: 0.55, BluffFreq: 0.30, CallTendency: 0.35},
			ThinkMin: 500, ThinkMax: 1500,
		},
		{
			ID: "tag", Name: "Taylor", StyleTag: "紧凶 TAG",
			Style:    Style{VPIP: 0.22, PFR: 0.18, ThreeBet: 0.07, Aggression: 0.65, BluffFreq: 0.25, CallTendency: 0.25},
			ThinkMin: 600, ThinkMax: 1800,
		},
		{
			ID: "lag", Name: "Logan", StyleTag: "松凶 LAG",
			Style:    Style{VPIP: 0.35, PFR: 0.28, ThreeBet: 0.10, Aggression: 0.75, BluffFreq: 0.40, CallTendency: 0.30},
			ThinkMin: 500, ThinkMax: 1600,
		},
		{
			ID: "nit", Name: "Nina", StyleTag: "紧弱 Nit",
			Style:    Style{VPIP: 0.12, PFR: 0.09, ThreeBet: 0.04, Aggression: 0.35, BluffFreq: 0.10, CallTendency: 0.20},
			ThinkMin: 400, ThinkMax: 1200,
		},
		{
			ID: "station", Name: "Stella", StyleTag: "跟注站",
			Style:    Style{VPIP: 0.45, PFR: 0.08, ThreeBet: 0.02, Aggression: 0.15, BluffFreq: 0.05, CallTendency: 0.90},
			ThinkMin: 300, ThinkMax: 900,
		},
		{
			ID: "maniac", Name: "Max", StyleTag: "疯狂 Maniac",
			Style:    Style{VPIP: 0.55, PFR: 0.45, ThreeBet: 0.18, Aggression: 0.95, BluffFreq: 0.65, CallTendency: 0.45},
			ThinkMin: 200, ThinkMax: 800,
		},
		{
			ID: "rock", Name: "Rocky", StyleTag: "岩石",
			Style:    Style{VPIP: 0.16, PFR: 0.12, ThreeBet: 0.05, Aggression: 0.45, BluffFreq: 0.15, CallTendency: 0.25},
			ThinkMin: 600, ThinkMax: 2000,
		},
		{
			ID: "fish", Name: "Finn", StyleTag: "松被动 Fish",
			Style:    Style{VPIP: 0.50, PFR: 0.12, ThreeBet: 0.03, Aggression: 0.30, BluffFreq: 0.15, CallTendency: 0.70},
			ThinkMin: 300, ThinkMax: 1000,
		},
	}
}
