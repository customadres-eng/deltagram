import lottie from 'lottie-web/build/player/lottie_light_canvas'
import { useEffect, useMemo, useRef, useState } from 'react'
import clsx from 'clsx'
import { giftImg, type GiftArt as GiftArtMeta } from '../lib/data'
import { Img } from './ui'

// Real DeltaGram gift artwork.
//
// Telegram draws an upgraded (NFT) gift as three layers: the backdrop
// gradient, the pattern Lottie and the model Lottie. Regular catalog gifts
// use a single catalog animation. Fragment's per-number .webp files are not
// representative for DeltaGram-specific gifts, so they are only a fallback
// when animation metadata is missing.
//
// The model/pattern Lottie files are large (up to ~700 KB each), so loading
// one per card makes the gift grid crawl on phones. Instead each unique
// (animation, tint) is rendered ONCE to a PNG and reused by every card that
// shares it; the live animation only plays on the gift detail page.

const animText = new Map<string, Promise<string>>()

function loadAnimationText(url: string): Promise<string> {
  let pending = animText.get(url)
  if (!pending) {
    pending = fetch(url, { credentials: 'same-origin' }).then((res) => {
      if (!res.ok) throw new Error(`gift animation ${res.status}`)
      return res.text()
    })
    animText.set(url, pending)
    pending.catch(() => animText.delete(url))
  }
  return pending
}

const hex = (value: number) => `#${((value >>> 0) & 0xffffff).toString(16).padStart(6, '0')}`

function tint(node: unknown, rgb: [number, number, number]) {
  if (!node || typeof node !== 'object') return
  if (Array.isArray(node)) {
    for (const item of node) tint(item, rgb)
    return
  }
  const obj = node as Record<string, unknown>
  if ((obj.ty === 'fl' || obj.ty === 'st') && obj.c && typeof obj.c === 'object') {
    const color = obj.c as { k?: unknown }
    if (Array.isArray(color.k)) color.k = [...rgb, 1]
  }
  for (const key of Object.keys(obj)) tint(obj[key], rgb)
}

// Live Lottie player for the gift detail page, where the model spins.
function useLottie(url: string | null, visible: boolean, tintColor?: number) {
  const host = useRef<HTMLDivElement>(null)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!visible || !url || !host.current) return
    let alive = true
    let animation: ReturnType<typeof lottie.loadAnimation> | null = null
    setReady(false)
    setFailed(false)

    loadAnimationText(url)
      .then((text) => {
        if (!alive || !host.current) return
        const data = JSON.parse(text)
        if (tintColor !== undefined) tint(data, [(tintColor >> 16) & 255, (tintColor >> 8) & 255, tintColor & 255])
        animation = lottie.loadAnimation({
          container: host.current,
          renderer: 'canvas',
          loop: true,
          autoplay: true,
          animationData: data,
          rendererSettings: { preserveAspectRatio: 'xMidYMid meet', clearCanvas: true },
        })
        animation.addEventListener('DOMLoaded', () => alive && setReady(true))
      })
      .catch(() => alive && setFailed(true))

    return () => {
      alive = false
      animation?.destroy()
    }
  }, [url, visible, tintColor])

  return { host, ready, failed }
}

// Renders a Lottie animation once at a fixed size, stops on the canonical
// "pose" frame, captures the canvas as a PNG and caches it. Every card that
// shares the same (url, tint, size) reuses the PNG, so a grid of 50 gifts with
// a handful of unique models costs a handful of renders instead of 50.
const staticFrameCache = new Map<string, string>()

export function useStaticFrame(url: string | null, visible: boolean, tintColor?: number, size = 128) {
  const [tile, setTile] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!visible || !url) return
    const key = `${url}#${tintColor ?? 0}#${size}`
    const cached = staticFrameCache.get(key)
    if (cached) {
      setTile(cached)
      return
    }
    let alive = true
    let animation: ReturnType<typeof lottie.loadAnimation> | null = null
    let host: HTMLDivElement | null = null
    setTile(null)
    setFailed(false)

    loadAnimationText(url)
      .then((text) => {
        if (!alive) return
        const data = JSON.parse(text)
        if (tintColor !== undefined) tint(data, [(tintColor >> 16) & 255, (tintColor >> 8) & 255, tintColor & 255])
        host = document.createElement('div')
        host.style.cssText = `position:fixed;left:-9999px;top:0;width:${size}px;height:${size}px;pointer-events:none;`
        document.body.appendChild(host)
        animation = lottie.loadAnimation({
          container: host,
          renderer: 'canvas',
          loop: false,
          autoplay: false,
          animationData: data,
          rendererSettings: { preserveAspectRatio: 'xMidYMid meet', clearCanvas: true },
        })
        animation.addEventListener('DOMLoaded', () => {
          if (!alive || !animation || !host) return
          const frame = Math.max(0, Math.min(animation.totalFrames - 1, Math.round(animation.totalFrames * 0.42)))
          animation.goToAndStop(frame, true)
          requestAnimationFrame(() => {
            if (!alive || !host) return
            const canvas = host.querySelector('canvas')
            if (canvas) {
              const dataURL = canvas.toDataURL('image/png')
              staticFrameCache.set(key, dataURL)
              setTile(dataURL)
            } else {
              setFailed(true)
            }
            if (animation) animation.destroy()
            animation = null
            host.remove()
            host = null
          })
        })
      })
      .catch(() => alive && setFailed(true))

    return () => {
      alive = false
      if (animation) animation.destroy()
      if (host) host.remove()
    }
  }, [url, visible, tintColor, size])

  return { tile, failed }
}

// Deterministic, non-overlapping pattern positions. Symbols share one small
// size and are placed on a jittered 3x3 grid, so they never overlap each other
// or run off the edge while still looking scattered rather than a strict grid.
function hashSeed(slug: string, num: number): number {
  const s = `${slug}-${num}`
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

function seededPositions(seed: number) {
  let s = seed >>> 0
  const rnd = () => {
    s = (s + 0x6d2b79f5) >>> 0
    let t = s
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  const count = 5 + (seed % 3) // 5, 6 or 7 symbols
  const size = 16 // uniform, small
  const centers: { x: number; y: number }[] = []
  for (let r = 0; r < 3; r++) for (let c = 0; c < 3; c++) centers.push({ x: 16.7 + c * 33.3, y: 16.7 + r * 33.3 })
  for (let i = centers.length - 1; i > 0; i--) {
    const j = Math.floor(rnd() * (i + 1))
    ;[centers[i], centers[j]] = [centers[j], centers[i]]
  }
  const jitter = 5
  return centers.slice(0, count).map((c) => ({
    left: c.x - size / 2 + (rnd() * 2 - 1) * jitter,
    top: c.y - size / 2 + (rnd() * 2 - 1) * jitter,
    size,
    rotate: Math.round(rnd() * 360 - 180),
    opacity: 0.2 + rnd() * 0.22,
  }))
}

export function GiftArt({
  art,
  slug,
  num,
  title,
  className,
  priority,
  animate,
}: {
  art?: GiftArtMeta | null
  slug: string
  num: number
  title: string
  className?: string
  priority?: boolean
  animate?: boolean
}) {
  const wrap = useRef<HTMLSpanElement>(null)
  const [visible, setVisible] = useState(!!priority)

  useEffect(() => {
    if (priority || visible) return
    const el = wrap.current
    if (!el || typeof IntersectionObserver === 'undefined') {
      setVisible(true)
      return
    }
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setVisible(true)
          io.disconnect()
        }
      },
      { rootMargin: '320px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [priority, visible])

  const unique = !!art && (art.unique || art.modelId > 0)
  const catalogURL = !unique && art && art.giftId > 0 ? `/api/gift-anim/catalog/${art.giftId}` : null
  const modelURL = unique && art && art.modelId > 0 ? `/api/gift-anim/model/${art.modelId}` : null
  const patternURL = unique && art && art.patternId > 0 ? `/api/gift-anim/pattern/${art.patternId}` : null
  const patternColor = art?.colors.pattern ?? 0
  const patternTint = patternColor !== 0 ? patternColor : (art?.colors.text ?? 0xffffff)

  // Static cards render each layer once (cached); the detail page animates.
  const liveModel = useLottie(animate ? modelURL : null, visible)
  const catalog = useStaticFrame(animate ? null : catalogURL, visible, undefined, 320)
  const model = useStaticFrame(animate ? null : modelURL, visible, undefined, 320)
  const pattern = useStaticFrame(patternURL, visible, patternTint, 128)

  const positions = useMemo(() => seededPositions(hashSeed(slug, num)), [slug, num])

  const colors = art?.colors
  const gradient =
    unique && colors && (colors.center || colors.edge)
      ? `radial-gradient(115% 115% at 50% 30%, ${hex(colors.center)} 0%, ${hex(colors.edge)} 78%)`
      : undefined

  const hasAnimation = !!catalogURL || !!modelURL
  const animated = (!!catalogURL && catalog.tile) || (!!modelURL && (model.tile || liveModel.ready))

  return (
    <span
      ref={wrap}
      className={clsx('relative block overflow-hidden', className)}
      style={gradient ? { background: gradient } : undefined}
    >
      {!animated && !hasAnimation && <Img src={giftImg(slug, num)} alt={title} className="size-full" />}
      {!animated && hasAnimation && <span className="skeleton absolute inset-0" />}
      {patternURL && pattern.tile && (
        <div className="pointer-events-none absolute inset-0">
          {positions.map((p, i) => (
            <span
              key={i}
              className="absolute"
              style={{
                left: `${p.left}%`,
                top: `${p.top}%`,
                width: `${p.size}%`,
                aspectRatio: '1 / 1',
                backgroundImage: `url(${pattern.tile})`,
                backgroundSize: 'contain',
                backgroundRepeat: 'no-repeat',
                backgroundPosition: 'center',
                opacity: p.opacity,
                transform: `rotate(${p.rotate}deg)`,
              }}
            />
          ))}
        </div>
      )}
      {catalogURL && catalog.tile && (
        <span
          className="absolute inset-0"
          style={{ backgroundImage: `url(${catalog.tile})`, backgroundSize: 'cover', backgroundPosition: 'center' }}
        />
      )}
      {modelURL && model.tile && (
        <span
          className="absolute inset-0"
          style={{ backgroundImage: `url(${model.tile})`, backgroundSize: 'cover', backgroundPosition: 'center' }}
        />
      )}
      {animate && modelURL && (
        <div
          ref={liveModel.host}
          className={clsx('absolute inset-0 transition-opacity duration-500', liveModel.ready ? 'opacity-100' : 'opacity-0')}
        />
      )}
    </span>
  )
}
