// 与服务端 proto 包对应的 WS / REST 类型定义。

export interface TableConfig {
  seats: number
  small_blind: number
  big_blind: number
  buyin: number
  action_timeout_ms: number
}

export interface Welcome {
  type: 'welcome'
  seat: number
  config: TableConfig
}

export interface SeatState {
  seat: number
  name: string
  style_tag?: string
  is_bot: boolean
  stack: number
  bet: number
  folded: boolean
  allin: boolean
  out: boolean
  to_act: boolean
  thinking: boolean
  hole?: string[]
}

export interface State {
  type: 'state'
  hand_no: number
  in_hand: boolean
  seats: SeatState[]
  board: string[]
  pot: number
  street: string
  button: number
  you: number
  ts: number
}

export interface LegalInfo {
  can_fold: boolean
  can_check: boolean
  call_amount: number
  can_raise: boolean
  min_raise_to: number
  max_raise_to: number
}

export interface ActionRequest {
  type: 'action_request'
  deadline: number
  legal: LegalInfo
  pot: number
  current_bet: number
}

export interface HandEvent {
  type: 'hand_event'
  hand_no: number
  kind: 'hand_start' | 'action' | 'street'
  street?: string
  seat?: number
  action?: string
  amount?: number
  to?: number
  board?: string[]
  pot: number
}

export interface ResultInfo {
  seat: number
  won: number
  net: number
  showdown: boolean
}

export interface HandEnd {
  type: 'hand_end'
  hand_no: number
  hand_id: number
  results: ResultInfo[]
  pots: { amount: number; winners: number[] }[]
  revealed_holes?: { seat: number; hole: string[] }[]
}

export interface ErrorMsg {
  type: 'error'
  message: string
}

export interface InfoMsg {
  type: 'info'
  message: string
}

export type ServerMsg = Welcome | State | ActionRequest | HandEvent | HandEnd | ErrorMsg | InfoMsg

// REST /api/hands
export interface PlayerRecord {
  seat: number
  name: string
  is_bot: boolean
  start_stack: number
  end_stack: number
  net: number
  hole: string
}

export interface ActionRecord {
  seq: number
  street: string
  seat: number
  type: 'blind' | 'fold' | 'check' | 'call' | 'bet' | 'raise'
  amount: number
  to: number
}

export interface HandSummary {
  id: number
  started_at: string
  button: number
  small_blind: number
  big_blind: number
  seed: number
  board: string
  players: PlayerRecord[]
}

export interface HandDetail extends HandSummary {
  actions?: ActionRecord[]
}

// REST /api/stats
export interface Stats {
  hands: number
  vpip: number
  pfr: number
  af: number
  bets_raises: number
  calls: number
  net_profit: number
  big_blind: number
}
