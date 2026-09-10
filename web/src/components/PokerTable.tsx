import type { HandEnd, State } from '../types'
import { CardList } from './PlayingCard'

// 9 个座位在椭圆桌上的位置（百分比）：seat 0 在底部中央，其余均匀环绕。
const POSITIONS = Array.from({ length: 9 }, (_, i) => {
  const theta = ((90 + i * 40) * Math.PI) / 180
  return { left: `${50 + 47 * Math.cos(theta)}%`, top: `${50 + 44 * Math.sin(theta)}%` }
})

interface Props {
  table: State | null
  handEnd: HandEnd | null
}

// PokerTable 椭圆桌 + 9 座位 + 中央公共牌与底池。
export function PokerTable({ table, handEnd }: Props) {
  const seats = table?.seats ?? []
  const currentHandNo = table?.hand_no ?? 0
  const showEnd = handEnd && handEnd.hand_no === currentHandNo && table && !table.in_hand

  return (
    <div className="poker-table">
      <div className="table-felt">
        <div className="community">
          {table && <CardList cards={table.board} big />}
        </div>
        <div className="pot">底池 {table?.pot ?? 0}</div>
        {showEnd && (
          <div className="hand-result">
            {handEnd.results
              .filter((r) => r.won > 0)
              .map((r) => (
                <div key={r.seat}>
                  {seats[r.seat]?.name ?? `座位${r.seat}`} 赢得 {r.won}
                  {r.net !== 0 && `（净 ${r.net > 0 ? '+' : ''}${r.net}）`}
                </div>
              ))}
          </div>
        )}
      </div>
      {seats.map((s) => (
        <div
          key={s.seat}
          className={[
            'seat',
            s.to_act ? 'seat-active' : '',
            s.folded ? 'seat-folded' : '',
            s.out ? 'seat-out' : '',
            s.seat === table?.you ? 'seat-you' : '',
          ].join(' ')}
          style={POSITIONS[s.seat]}
        >
          <div className="seat-name">
            {s.name}
            {table?.button === s.seat && <span className="dealer-btn">D</span>}
          </div>
          {s.style_tag && <div className="seat-style">{s.style_tag}</div>}
          <div className="seat-stack">
            {s.allin ? 'ALL-IN ' : ''}
            {s.stack}
          </div>
          {s.bet > 0 && <div className="seat-bet">{s.bet}</div>}
          {s.thinking && <div className="seat-thinking">思考中…</div>}
          {s.hole && <CardList cards={s.hole} big={s.seat === table?.you} />}
        </div>
      ))}
    </div>
  )
}
