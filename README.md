# 德州扑克练习场

本地 9 人桌德州扑克练习平台：你（浏览器）+ 8 个风格各异的 AI Bot。
用于练习牌技、学习德州扑克策略。

## 功能

- **9 人现金局**：盲注 1/2，买入 200（100bb），你坐 seat 0，其余 8 个 AI Bot
- **8 种牌风**：紧凶 TAG、松凶 LAG、紧弱 Nit、跟注站、疯狂 Maniac、岩石、松被动 Fish、基准 Baseline
- **手牌复盘**：上帝视角逐条街回放所有手牌（含 Bot 底牌）
- **个人统计**：VPIP / PFR / AF / 盈亏 / bb-100，附参考范围
- **LLM 教练点评**：对任意一手牌生成中文复盘点评（需配置 API key，见下）

## 快速开始

要求：Go 1.24+、Node 18+。

```bash
./dev.sh          # 首次运行先在 web/ 下 npm install
```

然后浏览器打开 <http://localhost:5173>。

手动分步启动：

```bash
cd web && npm install && npm run dev          # 前端 :5173（代理 /ws、/api 到 :8080）
cd server && go run ./cmd/pokerd              # 后端 :8080
```

生产模式（后端直接托管前端静态文件）：

```bash
cd web && npm run build
cd server && go run ./cmd/pokerd -web ../web/dist
# 打开 http://localhost:8080
```

## LLM 教练点评（可选）

复盘页每手牌有"教练点评"按钮。配置 OpenAI 兼容 API 后启用：

```bash
export POKER_LLM_API_KEY=sk-...
export POKER_LLM_BASE_URL=https://api.openai.com/v1   # 可换任何兼容端点
export POKER_LLM_MODEL=gpt-4o-mini                    # 默认 gpt-4o-mini
```

## 架构

```
web/      React + Vite + TS 前端（牌桌 / 复盘 / 统计三个页面）
server/   Go 后端
  internal/cards    牌表示 + 7选5 牌力评估器
  internal/engine   单手牌状态机（盲注/下注轮/边池/结算）
  internal/bot      蒙特卡洛权益估算 + 策略核心 + 8 个风格 persona
  internal/table    牌桌编排（goroutine + channel，无锁）
  internal/store    SQLite 手牌历史与统计
  internal/review   LLM 教练点评
  cmd/pokerd        服务端入口（WS + REST）
  cmd/sim           Bot 互打基准模拟：go run ./cmd/sim -hands 20000
```

## 测试

```bash
cd server && go test ./...
cd web && npm run build
```
