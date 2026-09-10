const SUIT_SYMBOL: Record<string, string> = { s: '♠', h: '♥', d: '♦', c: '♣' }

// PlayingCard 渲染一张牌（"As" 格式）；红桃方块为红色。
export function PlayingCard({ card, big }: { card: string; big?: boolean }) {
  if (!card || card.length < 2) return null
  const rank = card[0]
  const suit = SUIT_SYMBOL[card[1]] ?? card[1]
  const red = card[1] === 'h' || card[1] === 'd'
  return (
    <span className={`card ${red ? 'card-red' : 'card-black'} ${big ? 'card-big' : ''}`}>
      <span className="card-rank">{rank}</span>
      <span className="card-suit">{suit}</span>
    </span>
  )
}

export function CardList({ cards, big }: { cards: string[]; big?: boolean }) {
  return (
    <span className="card-list">
      {cards.map((c, i) => (
        <PlayingCard key={i} card={c} big={big} />
      ))}
    </span>
  )
}
