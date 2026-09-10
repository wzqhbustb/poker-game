import { useEffect, useMemo, useState } from 'react'
import type { HandDetail, HandSummary } from '../types'
import { CardList } from '../components/PlayingCard'

const STREETS = ['preflop', 'flop', 'turn', 'river']
const STREET_NAMES: Record<string, string> = {
  preflop: '翻前',
  flop: '翻牌',
  turn: '转牌',
  river: '河牌',
}
const ACTION_TEXT: Record<string, string> = {
  blind: '盲注',
  fold: '弃牌',
  check: '过牌',
  call: '跟注',
  bet: '下注',
  raise: '加注到',
}

interface ReplaySeat {
  seat: number
  name: string
  isBot: boolean
  hole: string[]
  stack: number // 当前剩余（起始 - 累计投入）
  streetBet: number // 当前街已下注
  folded: boolean
  lastAction: string
  net: number
}

interface ReplayState {
  step: number // 已应用的动作数
  street: string
  board: string[]
  pot: number
  seats: ReplaySeat[]
  lastActor: number
}

// buildReplay 应用前 step 个动作得到回放状态。
function buildReplay(hand: HandDetail, step: number): ReplayState {
  const boardAll = hand.board ? hand.board.split(' ').filter(Boolean) : []
  const seats: ReplaySeat[] = hand.players.map((p) => ({
    seat: p.seat,
    name: p.name,
    isBot: p.is_bot,
    hole: p.hole ? p.hole.split(' ').filter(Boolean) : [],
    stack: p.start_stack,
    streetBet: 0,
    folded: false,
    lastAction: '',
    net: p.net,
  }))
  const actions = hand.actions ?? []
  let pot = 0
  let street = 'preflop'
  let lastActor = -1
  for (let i = 0; i < step && i < actions.length; i++) {
    const a = actions[i]
    const s = seats.find((x) => x.seat === a.seat)
    if (!s) continue
    if (a.street !== street) {
      street = a.street
      for (const x of seats) x.streetBet = 0
    }
    pot += a.amount
    s.stack -= a.amount
    s.streetBet = a.to
    s.lastAction = ACTION_TEXT[a.type] + (a.amount > 0 ? ` ${a.amount}` : '')
    if (a.type === 'fold') s.folded = true
    lastActor = a.seat
  }
  const boardCount = street === 'flop' ? 3 : street === 'turn' ? 4 : street === 'river' ? 5 : 0
  return { step, street, board: boardAll.slice(0, boardCount), pot, seats, lastActor }
}

// ReviewPage 手牌复盘：左侧列表，右侧上帝视角逐条街回放。
export function ReviewPage() {
  const [hands, setHands] = useState<HandSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<HandDetail | null>(null)
  const [step, setStep] = useState(0)
  const [review, setReview] = useState<{ content: string; enabled: boolean } | null>(null)
  const [reviewLoading, setReviewLoading] = useState(false)
  const [reviewErr, setReviewErr] = useState('')

  const load = async (offset: number) => {
    setLoading(true)
    try {
      const res = await fetch(`/api/hands?limit=30&offset=${offset}`)
      const batch: HandSummary[] = await res.json()
      setHands((prev) => (offset === 0 ? batch : [...prev, ...batch]))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(0)
  }, [])

  const openHand = async (id: number) => {
    const res = await fetch(`/api/hands/${id}`)
    const detail: HandDetail = await res.json()
    setSelected(detail)
    setStep((detail.actions ?? []).length) // 默认停在终局
    setReview(null)
    setReviewErr('')
    try {
      const r = await fetch(`/api/review/${id}`)
      setReview(await r.json())
    } catch {
      setReview(null)
    }
  }

  const requestReview = async () => {
    if (!selected) return
    setReviewLoading(true)
    setReviewErr('')
    try {
      const res = await fetch(`/api/review/${selected.id}`, { method: 'POST' })
      const body = await res.json()
      if (!res.ok) {
        setReviewErr(body.message ?? body ?? '生成失败')
      } else {
        setReview(body)
      }
    } catch {
      setReviewErr('无法连接服务器')
    } finally {
      setReviewLoading(false)
    }
  }

  const replay = useMemo(
    () => (selected ? buildReplay(selected, step) : null),
    [selected, step],
  )
  const totalSteps = selected?.actions?.length ?? 0
  const streetStart = useMemo(() => {
    const map: Record<string, number> = {}
    ;(selected?.actions ?? []).forEach((a, i) => {
      if (!(a.street in map)) map[a.street] = i
    })
    return map
  }, [selected])

  return (
    <div className="review-page">
      <div className="hand-list">
        <h3>手牌历史</h3>
        {hands.map((h) => {
          const me = h.players.find((p) => !p.is_bot)
          return (
            <div
              key={h.id}
              className={`hand-item ${selected?.id === h.id ? 'hand-selected' : ''}`}
              onClick={() => openHand(h.id)}
            >
              <div>
                #{h.id} {new Date(h.started_at).toLocaleString()}
              </div>
              <div className="hand-item-sub">
                {me && (
                  <span className={me.net >= 0 ? 'net-pos' : 'net-neg'}>
                    {me.hole} {me.net >= 0 ? '+' : ''}
                    {me.net}
                  </span>
                )}
              </div>
            </div>
          )
        })}
        <button className="btn" disabled={loading} onClick={() => load(hands.length)}>
          {loading ? '加载中…' : '加载更多'}
        </button>
      </div>
      <div className="replay">
        {!selected || !replay ? (
          <div className="replay-empty">选择左侧一手牌开始复盘</div>
        ) : (
          <>
            <div className="replay-board">
              <CardList cards={replay.board} big />
              <span className="replay-pot">
                {STREET_NAMES[replay.street]} · 底池 {replay.pot}
              </span>
            </div>
            <div className="replay-seats">
              {replay.seats.map((s) => (
                <div
                  key={s.seat}
                  className={[
                    'replay-seat',
                    s.folded ? 'seat-folded' : '',
                    s.seat === replay.lastActor ? 'seat-active' : '',
                  ].join(' ')}
                >
                  <div className="seat-name">
                    {s.name}
                    {selected.button === s.seat && <span className="dealer-btn">D</span>}
                    {step === totalSteps && s.net !== 0 && (
                      <span className={s.net > 0 ? 'net-pos' : 'net-neg'}>
                        {' '}
                        {s.net > 0 ? '+' : ''}
                        {s.net}
                      </span>
                    )}
                  </div>
                  <CardList cards={s.hole} />
                  <div className="seat-stack">剩余 {s.stack}</div>
                  {s.streetBet > 0 && <div className="seat-bet">{s.streetBet}</div>}
                  {s.lastAction && <div className="replay-action">{s.lastAction}</div>}
                </div>
              ))}
            </div>
            <div className="replay-controls">
              <button className="btn" disabled={step <= 0} onClick={() => setStep(step - 1)}>
                ← 上一步
              </button>
              <input
                type="range"
                min={0}
                max={totalSteps}
                value={step}
                onChange={(e) => setStep(Number(e.target.value))}
              />
              <button
                className="btn"
                disabled={step >= totalSteps}
                onClick={() => setStep(step + 1)}
              >
                下一步 →
              </button>
            </div>
            <div className="replay-streets">
              {STREETS.filter((s) => s in streetStart).map((s) => (
                <button key={s} className="btn btn-small" onClick={() => setStep(streetStart[s])}>
                  {STREET_NAMES[s]}
                </button>
              ))}
              <button className="btn btn-small" onClick={() => setStep(totalSteps)}>
                终局
              </button>
            </div>
            <div className="coach-section">
              {review?.enabled === false ? (
                <div className="hint">教练点评未启用（服务端需配置 POKER_LLM_API_KEY）</div>
              ) : (
                <>
                  <button className="btn" disabled={reviewLoading} onClick={requestReview}>
                    {reviewLoading ? '教练分析中…' : review?.content ? '重新点评' : '教练点评'}
                  </button>
                  {reviewErr && <div className="banner banner-error">{reviewErr}</div>}
                  {review?.content && <div className="coach-content">{review.content}</div>}
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
