import type { GameSummary } from '../api/client'

interface Props {
  game: GameSummary
  highlight?: boolean
}

const GameCard = ({ game, highlight = false }: Props) => {
  const displayScore = game.score ? game.score.toFixed(1) : '—'

  return (
    <article className={`game-card ${highlight ? 'game-card--highlight' : ''}`}>
      <div
        className="game-card__cover"
        style={{
          backgroundImage: game.coverImage ? `url(${game.coverImage})` : undefined,
        }}
        aria-hidden="true"
      />
      <div className="game-card__body">
        <header>
          <p className="game-card__genre">{game.genres.join(' / ') || '未知类型'}</p>
          <h3>{game.title}</h3>
        </header>
        <p className="game-card__description">
          {game.description ?? '尚未提供详细介绍，敬请期待。'}
        </p>
        <div className="game-card__meta">
          <span>{game.platforms.join(', ') || '多平台'}</span>
          <span className="game-card__score">{displayScore}</span>
        </div>
        {game.tags?.length ? (
          <div className="game-card__tags">
            {game.tags.slice(0, 3).map((tag) => (
              <span key={tag}>{tag}</span>
            ))}
          </div>
        ) : null}
      </div>
    </article>
  )
}

export default GameCard
