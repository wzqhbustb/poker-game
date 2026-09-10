import { useEffect, useReducer, useRef } from 'react'
import { PokerSocket, wsURL } from '../net/ws'
import type { ActionRequest, HandEnd, ServerMsg, State, TableConfig } from '../types'

export interface TableState {
  connected: boolean
  config: TableConfig | null
  table: State | null
  actionReq: ActionRequest | null
  actionReqAt: number // 收到 action_request 的本地时间，用于倒计时
  log: string[]
  handEnd: HandEnd | null
  error: string | null
}

type Msg =
  | { kind: 'status'; connected: boolean }
  | { kind: 'server'; msg: ServerMsg }
  | { kind: 'clear-error' }

const initial: TableState = {
  connected: false,
  config: null,
  table: null,
  actionReq: null,
  actionReqAt: 0,
  log: [],
  handEnd: null,
  error: null,
}

const ACTION_TEXT: Record<string, string> = {
  fold: '弃牌',
  check: '过牌',
  call: '跟注',
  bet: '下注',
  raise: '加注',
  blind: '盲注',
}

function pushLog(log: string[], line: string): string[] {
  const next = [...log, line]
  return next.length > 60 ? next.slice(next.length - 60) : next
}

function seatName(t: State | null, seat: number): string {
  return t?.seats[seat]?.name ?? `座位${seat}`
}

function reduce(st: TableState, m: Msg): TableState {
  if (m.kind === 'status') {
    return { ...st, connected: m.connected, error: m.connected ? null : st.error }
  }
  if (m.kind === 'clear-error') {
    return { ...st, error: null }
  }
  const msg = m.msg
  switch (msg.type) {
    case 'welcome':
      return { ...st, config: msg.config }
    case 'state': {
      // 轮到自己行动的请求在新的 state 表明不再轮到自己时失效
      const me = msg.seats[msg.you]
      const actionReq = st.actionReq && me && me.to_act ? st.actionReq : null
      return { ...st, table: msg, actionReq }
    }
    case 'action_request':
      return { ...st, actionReq: msg, actionReqAt: Date.now() }
    case 'hand_event': {
      let log = st.log
      if (msg.kind === 'action' && msg.action) {
        const a = ACTION_TEXT[msg.action] ?? msg.action
        const amt = msg.amount && msg.amount > 0 ? ` ${msg.amount}` : ''
        log = pushLog(log, `${seatName(st.table, msg.seat ?? -1)} ${a}${amt}`)
      } else if (msg.kind === 'street' && msg.street) {
        const names: Record<string, string> = { flop: '翻牌', turn: '转牌', river: '河牌' }
        log = pushLog(log, `—— ${names[msg.street] ?? msg.street}: ${(msg.board ?? []).join(' ')}`)
      } else if (msg.kind === 'hand_start') {
        log = pushLog(log, `▶ 第 ${msg.hand_no} 手开始`)
      }
      return { ...st, log }
    }
    case 'hand_end': {
      let log = st.log
      for (const r of msg.results) {
        if (r.won > 0) {
          log = pushLog(log, `${seatName(st.table, r.seat)} 赢得 ${r.won}`)
        }
      }
      return { ...st, log, handEnd: msg, actionReq: null }
    }
    case 'error':
      return { ...st, error: msg.message }
  }
}

export interface TableApi {
  state: TableState
  sendAction: (action: string, amount?: number) => void
  rebuy: () => void
  clearError: () => void
}

// useTable 建立并维持与后端的 WS 连接（含重连与断线自动入座），返回牌桌状态。
export function useTable(name: string): TableApi {
  const [state, dispatch] = useReducer(reduce, initial)
  const sockRef = useRef<PokerSocket | null>(null)

  useEffect(() => {
    const sock = new PokerSocket(
      wsURL(),
      name,
      (msg) => dispatch({ kind: 'server', msg }),
      (connected) => dispatch({ kind: 'status', connected }),
    )
    sockRef.current = sock
    sock.start()
    return () => {
      sockRef.current = null
      sock.close()
    }
  }, [name])

  return {
    state,
    sendAction: (action, amount) =>
      sockRef.current?.send({ type: 'action', action, amount: amount ?? 0 }),
    rebuy: () => sockRef.current?.send({ type: 'rebuy' }),
    clearError: () => dispatch({ kind: 'clear-error' }),
  }
}
