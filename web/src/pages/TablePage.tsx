import type { TableApi } from '../state/useTable'
import { ActionPanel } from '../components/ActionPanel'
import { PokerTable } from '../components/PokerTable'

// TablePage 实时牌桌页。
export function TablePage({ api }: { api: TableApi }) {
  const { state, sendAction, rebuy, clearError } = api
  const me = state.table?.seats[state.table.you]
  const broke = me !== undefined && me.stack === 0 && !me.allin

  return (
    <div className="table-page">
      {!state.connected && <div className="banner">与服务器断开，正在重连…</div>}
      {state.error && (
        <div className="banner banner-error" onClick={clearError}>
          {state.error}（点击关闭）
        </div>
      )}
      <PokerTable table={state.table} handEnd={state.handEnd} />
      {broke && (
        <div className="rebuy-bar">
          筹码不足
          <button className="btn" onClick={rebuy}>
            补码到买入
          </button>
        </div>
      )}
      {state.actionReq && (
        <ActionPanel req={state.actionReq} receivedAt={state.actionReqAt} onAction={sendAction} />
      )}
      <div className="event-log">
        {state.log.slice(-12).map((line, i) => (
          <div key={i}>{line}</div>
        ))}
      </div>
    </div>
  )
}
