import { useCallback, useEffect, useRef, useState } from 'react'
import { motion } from 'motion/react'
import { ArrowsClockwiseIcon, CaretRightIcon, PlusIcon, StarIcon } from '@phosphor-icons/react'
import clsx from 'clsx'
import { fmt } from '../lib/format'

export type MoneyCardProps = {
  ton: number
  stars: number
  tonPending: number
  username: string
  userId: number
  onSendTON: () => void
  onSendStars: () => void
  onBuyTON: () => void
  onHistory: () => void
}

// Sparkles are decorative dots painted at fixed coordinates. Keeping them in a
// list instead of random per render makes the card stable while it animates.
const SPARKS = [
  { x: 12, y: 20, s: 2.4, d: 0 },
  { x: 34, y: 12, s: 1.6, d: 0.7 },
  { x: 57, y: 26, s: 2.8, d: 1.4 },
  { x: 78, y: 15, s: 1.8, d: 0.3 },
  { x: 88, y: 48, s: 2.2, d: 1.9 },
  { x: 22, y: 68, s: 1.5, d: 1.1 },
  { x: 49, y: 78, s: 2.6, d: 2.4 },
  { x: 68, y: 62, s: 1.7, d: 0.9 },
  { x: 92, y: 80, s: 2.1, d: 1.6 },
  { x: 8, y: 46, s: 1.4, d: 2.1 },
]

function Sparkles() {
  return (
    <span className="money-sparks" aria-hidden>
      {SPARKS.map((p, i) => (
        <i key={i} style={{ left: `${p.x}%`, top: `${p.y}%`, width: p.s, height: p.s, animationDelay: `${p.d}s` }} />
      ))}
    </span>
  )
}

// The chip is the QR-like contact pad from the reference bank cards. It is an
// SVG so it stays crisp at any size and needs no asset.
function Chip() {
  return (
    <span className="money-chip" aria-hidden>
      <svg viewBox="0 0 44 44">
        <rect x="1" y="1" width="42" height="42" rx="9" fill="#fff" />
        <rect x="7" y="7" width="9" height="9" rx="2" fill="#20242c" />
        <rect x="28" y="7" width="9" height="9" rx="2" fill="#20242c" />
        <rect x="7" y="28" width="9" height="9" rx="2" fill="#20242c" />
        <rect x="21" y="21" width="7" height="7" rx="1.5" fill="#20242c" />
        <rect x="31" y="28" width="6" height="6" rx="1.5" fill="#20242c" />
        <rect x="21" y="7" width="4" height="4" rx="1" fill="#20242c" opacity=".55" />
        <rect x="7" y="21" width="4" height="4" rx="1" fill="#20242c" opacity=".55" />
      </svg>
    </span>
  )
}

function TONMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 56 56" className={clsx('shrink-0 drop-shadow-[0_6px_16px_rgba(0,0,0,.35)]', className)} aria-label="TON" role="img">
      <circle cx="28" cy="28" r="28" fill="rgba(255,255,255,.18)" />
      <path
        fill="#fff"
        d="M37.56 15.63H18.44c-3.52 0-5.75 3.79-3.98 6.86l11.8 20.45c.77 1.34 2.7 1.34 3.47 0l11.8-20.45c1.77-3.06-.46-6.86-3.97-6.86ZM26.25 36.81l-2.57-4.98-6.2-11.09c-.41-.71.09-1.62.95-1.62h7.82v17.69Zm3.49 0V19.12h7.82c.86 0 1.36.91.95 1.62l-8.77 16.07Z"
      />
    </svg>
  )
}


export function MoneyCards(props: MoneyCardProps) {
  const { ton, stars, tonPending, username, userId } = props
  const trackRef = useRef<HTMLDivElement>(null)
  const [active, setActive] = useState(0)
  const cards = ['ton', 'stars'] as const

  const sync = useCallback(() => {
    const el = trackRef.current
    if (!el) return
    const first = el.querySelector<HTMLElement>('.money-card')
    if (!first) return
    const step = first.offsetWidth + 14
    setActive(Math.max(0, Math.min(1, Math.round(el.scrollLeft / step))))
  }, [])

  useEffect(() => {
    const el = trackRef.current
    if (!el) return
    el.addEventListener('scroll', sync, { passive: true })
    return () => el.removeEventListener('scroll', sync)
  }, [sync])

  const go = (i: number) => {
    const el = trackRef.current
    const card = el?.querySelector<HTMLElement>('.money-card')
    if (!el || !card) return
    el.scrollTo({ left: (card.offsetWidth + 14) * i, behavior: 'smooth' })
  }

  const isTON = cards[active] === 'ton'
  const rub = isTON ? ton * 0.3 : stars * 1.5

  return (
    <div className="money">
      <div className="money-head">
        <p className="money-head-label">У вас</p>
        <p className="money-head-value">
          <TONMark className="size-4" />
          <b>{fmt(ton)}</b> TON <span>в заработанных</span>
        </p>
        <button type="button" className="money-history" onClick={props.onHistory}>
          <ArrowsClockwiseIcon className="size-3.5" /> История TON
        </button>
      </div>

      <div className="money-track" ref={trackRef} role="list" aria-label="Карты баланса">
        <motion.article
          role="listitem"
          className="money-card money-ton"
          initial={{ opacity: 0, y: 16, rotateX: 6 }}
          animate={{ opacity: 1, y: 0, rotateX: 0 }}
          transition={{ duration: 0.55, ease: [0.22, 1, 0.36, 1] }}
          onClick={() => go(0)}
        >
          <Sparkles />
          <span className="money-shine" aria-hidden />
          <div className="money-card-top">
            <div className="money-balance">
              <div className="money-balance-line">
                <TONMark className="size-7 sm:size-8" />
                <span className="money-amount">{fmt(ton)}</span>
                <span className="money-unit">TON</span>
              </div>
              <p className="money-sub">≈ {fmt(rub)} ₽</p>
            </div>
            <div className="money-card-side">
              <Chip />
              <span className="money-serial">TON CARD</span>
            </div>
          </div>
          <div className="money-card-bottom">
            <div className="money-owner">
              <span>{username || 'DELTAMENT'}</span>
              <i>#{userId}</i>
            </div>
            {tonPending > 0 && <span className="money-pending">в отправке {fmt(tonPending)} TON</span>}
          </div>
        </motion.article>

        <motion.article
          role="listitem"
          className="money-card money-stars"
          initial={{ opacity: 0, y: 16, rotateX: 6 }}
          animate={{ opacity: 1, y: 0, rotateX: 0 }}
          transition={{ duration: 0.55, delay: 0.06, ease: [0.22, 1, 0.36, 1] }}
          onClick={() => go(1)}
        >
          <Sparkles />
          <span className="money-shine" aria-hidden />
          <div className="money-card-top">
            <div className="money-balance">
              <div className="money-balance-line">
                <StarIcon className="size-7 text-white drop-shadow-[0_6px_16px_rgba(0,0,0,.35)] sm:size-8" weight="fill" />
                <span className="money-amount">{fmt(stars)}</span>
                <span className="money-unit">STARS</span>
              </div>
              <p className="money-sub">≈ {fmt(rub)} ₽</p>
            </div>
            <div className="money-card-side">
              <Chip />
              <span className="money-serial">STAR CARD</span>
            </div>
          </div>
          <div className="money-card-bottom">
            <div className="money-owner">
              <span>{username || 'DELTAMENT'}</span>
              <i>#{userId}</i>
            </div>
            <span className="money-pending">заработано звёздами</span>
          </div>
        </motion.article>
      </div>

      <div className="money-dots" aria-hidden>
        {cards.map((c, i) => (
          <button key={c} type="button" className={clsx('money-dot', active === i && 'on')} onClick={() => go(i)} aria-label={c} />
        ))}
      </div>

      {isTON ? (
        <button type="button" className="money-action money-action-ton" onClick={props.onSendTON}>
          Вывести TON <CaretRightIcon className="size-4" />
        </button>
      ) : (
        <button type="button" className="money-action money-action-stars" onClick={props.onSendStars}>
          Отправить Stars <CaretRightIcon className="size-4" />
        </button>
      )}

      <div className="money-quick">
        <button type="button" onClick={props.onBuyTON}>
          <PlusIcon className="size-3.5" /> Купить TON
        </button>
        <button type="button" onClick={props.onHistory}>
          <ArrowsClockwiseIcon className="size-3.5" /> История TON
        </button>
      </div>
    </div>
  )
}
