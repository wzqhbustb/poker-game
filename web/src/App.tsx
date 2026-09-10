import { useState } from 'react'
import { useTable } from './state/useTable'
import { TablePage } from './pages/TablePage'
import { ReviewPage } from './pages/ReviewPage'
import { StatsPage } from './pages/StatsPage'

type Tab = 'table' | 'review' | 'stats'

export default function App() {
  const [tab, setTab] = useState<Tab>('table')
  // useTable 提到 App 层，切换 tab 不断线
  const api = useTable(localStorage.getItem('pokerName') ?? '你')

  return (
    <div className="app">
      <header className="app-header">
        <span className="app-title">德州扑克练习场</span>
        <nav>
          {(
            [
              ['table', '牌桌'],
              ['review', '复盘'],
              ['stats', '统计'],
            ] as [Tab, string][]
          ).map(([key, label]) => (
            <button
              key={key}
              className={`tab ${tab === key ? 'tab-active' : ''}`}
              onClick={() => setTab(key)}
            >
              {label}
            </button>
          ))}
        </nav>
        <span className={`conn ${api.state.connected ? 'conn-ok' : 'conn-bad'}`}>
          {api.state.connected ? '已连接' : '未连接'}
        </span>
      </header>
      {tab === 'table' && <TablePage api={api} />}
      {tab === 'review' && <ReviewPage />}
      {tab === 'stats' && <StatsPage />}
    </div>
  )
}
