import { useCallback, useEffect, useState } from 'react'
import type { Stats } from '../types'

// 9 人桌常见参考值，帮助用户对照改进。
const REFERENCES = [
  { key: 'vpip', name: 'VPIP 自愿入池率', ref: '15% ~ 20%', hint: '太高说明入池太松，太低会错失牌局参与感与信息' },
  { key: 'pfr', name: 'PFR 翻前加注率', ref: '12% ~ 18%', hint: '应尽量接近 VPIP，多数牌加注入池而非跟注' },
  { key: 'af', name: 'AF 翻后激进度', ref: '2 ~ 3.5', hint: '(下注+加注)/跟注；低于 1 偏被动，过高易被针对' },
] as const

// StatsPage 个人数据统计。
export function StatsPage() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [err, setErr] = useState('')

  const load = useCallback(async () => {
    try {
      const res = await fetch('/api/stats')
      setStats(await res.json())
      setErr('')
    } catch {
      setErr('无法连接服务器')
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  if (err) return <div className="banner banner-error">{err}</div>
  if (!stats) return <div className="stats-page">加载中…</div>

  const bb = stats.big_blind > 0 ? stats.big_blind : 2
  const bb100 =
    stats.hands > 0 ? (stats.net_profit / stats.hands) * (100 / bb) : 0

  return (
    <div className="stats-page">
      <h3>我的数据（累计 {stats.hands} 手）</h3>
      <table className="stats-table">
        <thead>
          <tr>
            <th>指标</th>
            <th>我的值</th>
            <th>参考范围</th>
            <th>说明</th>
          </tr>
        </thead>
        <tbody>
          {REFERENCES.map((r) => (
            <tr key={r.key}>
              <td>{r.name}</td>
              <td>
                {r.key === 'af'
                  ? stats.af.toFixed(2)
                  : `${(stats[r.key] * 100).toFixed(1)}%`}
              </td>
              <td>{r.ref}</td>
              <td className="hint">{r.hint}</td>
            </tr>
          ))}
          <tr>
            <td>总盈亏</td>
            <td className={stats.net_profit >= 0 ? 'net-pos' : 'net-neg'}>
              {stats.net_profit >= 0 ? '+' : ''}
              {stats.net_profit}
            </td>
            <td colSpan={2} className="hint">
              每百手 {bb100 >= 0 ? '+' : ''}
              {bb100.toFixed(1)} bb/100（大盲 {bb}）
            </td>
          </tr>
          <tr>
            <td>翻后下注/加注 / 跟注</td>
            <td>
              {stats.bets_raises} / {stats.calls}
            </td>
            <td colSpan={2}></td>
          </tr>
        </tbody>
      </table>
      <button className="btn" onClick={load}>
        刷新
      </button>
    </div>
  )
}
