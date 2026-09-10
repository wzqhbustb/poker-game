import { useEffect, useState } from 'react'
import type { ActionRequest } from '../types'

interface Props {
  req: ActionRequest
  receivedAt: number
  onAction: (action: string, amount?: number) => void
}

// ActionPanel 人类行动面板：弃牌/过牌/跟注 + 加注滑块与常用尺度 + 倒计时。
export function ActionPanel({ req, receivedAt, onAction }: Props) {
  const { legal, pot, current_bet: curBet } = req
  const [raiseTo, setRaiseTo] = useState(legal.min_raise_to)
  const [now, setNow] = useState(Date.now())

  useEffect(() => {
    setRaiseTo(legal.min_raise_to)
  }, [req])

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 200)
    return () => clearInterval(t)
  }, [])

  const total = req.deadline - receivedAt
  const remain = Math.max(0, req.deadline - now)
  const pct = total > 0 ? (remain / total) * 100 : 0
  const urgent = remain < 5000

  const clamp = (v: number) => Math.min(legal.max_raise_to, Math.max(legal.min_raise_to, v))
  // 池底尺度换算成 raise-to：当前最高注 + 池的 frac 倍
  const potSize = (frac: number) => clamp(curBet + Math.round(pot * frac))

  return (
    <div className="action-panel">
      <div className={`timer-bar ${urgent ? 'timer-urgent' : ''}`}>
        <div className="timer-fill" style={{ width: `${pct}%` }} />
      </div>
      <div className="action-buttons">
        {legal.can_fold && (
          <button className="btn btn-fold" onClick={() => onAction('fold')}>
            弃牌
          </button>
        )}
        {legal.can_check ? (
          <button className="btn btn-check" onClick={() => onAction('check')}>
            过牌
          </button>
        ) : (
          legal.call_amount > 0 && (
            <button className="btn btn-call" onClick={() => onAction('call')}>
              跟注 {legal.call_amount}
            </button>
          )
        )}
        {legal.can_raise && (
          <div className="raise-group">
            <input
              type="range"
              min={legal.min_raise_to}
              max={legal.max_raise_to}
              value={raiseTo}
              onChange={(e) => setRaiseTo(Number(e.target.value))}
            />
            <div className="raise-quick">
              <button className="btn btn-small" onClick={() => setRaiseTo(potSize(0.5))}>1/2池</button>
              <button className="btn btn-small" onClick={() => setRaiseTo(potSize(2 / 3))}>2/3池</button>
              <button className="btn btn-small" onClick={() => setRaiseTo(potSize(1))}>满池</button>
              <button className="btn btn-small" onClick={() => setRaiseTo(legal.max_raise_to)}>All-in</button>
            </div>
            <button className="btn btn-raise" onClick={() => onAction('raise', raiseTo)}>
              {curBet > 0 ? '加注到' : '下注'} {raiseTo}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
