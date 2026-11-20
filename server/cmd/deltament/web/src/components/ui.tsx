import { useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AnimatePresence, motion, type Transition } from 'motion/react'
import { CaretDownIcon, CheckIcon, GiftIcon, MagnifyingGlassIcon, XIcon } from '@phosphor-icons/react'
import clsx from 'clsx'
import { fmt, fmtUsd } from '../lib/format'

export const spring: Transition = { type: 'spring', bounce: 0.2, duration: 0.5 }
export const softSpring: Transition = { type: 'spring', bounce: 0, duration: 0.5 }
export const ease: [number, number, number, number] = [0.22, 1, 0.36, 1]

// логотип и валюта

export function Logo({ className }: { className?: string }) {
  return (
    <span className={clsx('flex items-center gap-2.5', className)}>
      <img src="/logo-mark.png" alt="" className="size-[26px] select-none object-contain" draggable={false} />
      <span className="font-brand text-[20px] leading-none font-extrabold tracking-[-0.04em] sm:text-[23px]">Deltament</span>
    </span>
  )
}

// значок GRAM — фирменная дельта Deltament
export function GramIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 56 56" className={clsx('shrink-0', className)} aria-label="GRAM" role="img">
      <circle cx="28" cy="28" r="28" fill="#0098EA" />
      <path
        fill="#fff"
        d="M37.56 15.63H18.44c-3.52 0-5.75 3.79-3.98 6.86l11.8 20.45c.77 1.34 2.7 1.34 3.47 0l11.8-20.45c1.77-3.06-.46-6.86-3.97-6.86ZM26.25 36.81l-2.57-4.98-6.2-11.09c-.41-.71.09-1.62.95-1.62h7.82v17.69Zm3.49 0V19.12h7.82c.86 0 1.36.91.95 1.62l-8.77 16.07Z"
      />
    </svg>
  )
}

export function Gram({ value, className, size = 'size-3.5', usd }: { value: number; className?: string; size?: string; usd?: boolean }) {
  return (
    <span className={clsx('inline-flex flex-col', className)}>
      <span className="inline-flex items-baseline gap-1.5 tabular">
        <GramIcon className={clsx(size, 'self-center')} />
        {fmt(value)}
        <span className="text-[0.62em] font-bold tracking-[0.08em] text-muted">GRAM</span>
      </span>
      {usd && <span className="mt-0.5 text-[11px] font-medium text-subtle tabular">~ {fmtUsd(value)}</span>}
    </span>
  )
}

export function StarIcon({ className }: { className?: string }) {
  const id = useId()
  return (
    <svg viewBox="0 0 24 24" className={clsx('shrink-0', className)} aria-hidden>
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#ffd45e" />
          <stop offset="1" stopColor="#f5a234" />
        </linearGradient>
      </defs>
      <path
        fill={`url(#${id})`}
        d="M12 2.6c.4 0 .76.23.93.6l2.3 4.86 5.3.7c.84.1 1.18 1.14.56 1.72l-3.88 3.68.98 5.26c.16.83-.72 1.47-1.46 1.06L12 17.94l-4.73 2.54c-.74.4-1.62-.23-1.46-1.06l.98-5.26L2.9 10.48c-.62-.58-.28-1.61.56-1.72l5.3-.7 2.3-4.86c.17-.37.53-.6.94-.6Z"
      />
    </svg>
  )
}

// кнопки

type BtnProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onDrag' | 'onDragStart' | 'onDragEnd' | 'onAnimationStart'> & {
  variant?: 'primary' | 'glass' | 'ghost' | 'danger'
  size?: 'sm' | 'md' | 'lg'
}

export function Button({ variant = 'primary', size = 'md', className, ...rest }: BtnProps) {
  return (
    <motion.button
      whileTap={rest.disabled ? undefined : { scale: 0.975 }}
      whileHover={rest.disabled ? undefined : { y: -1 }}
      transition={{ type: 'spring', bounce: 0.2, duration: 0.35 }}
      {...rest}
      className={clsx(
        'inline-flex items-center justify-center gap-2 rounded-[8px] text-[13px] font-semibold tracking-[0.01em] whitespace-nowrap transition-colors duration-300 disabled:opacity-40',
        size === 'sm' && 'h-8 px-3.5',
        size === 'md' && 'h-10 px-5',
        size === 'lg' && 'h-[52px] px-6 text-[14px]',
        variant === 'primary' && 'shine bg-accent text-white hover:bg-accent-hover',
        variant === 'glass' && 'border border-line bg-white/[0.04] text-fg hover:border-line-strong hover:bg-white/[0.07]',
        variant === 'ghost' && 'text-muted hover:bg-white/[0.05] hover:text-fg',
        variant === 'danger' && 'border border-danger/30 bg-danger/10 text-danger hover:bg-danger/16',
        className,
      )}
    />
  )
}

// поля ввода

export function SearchInput({ value, onChange, placeholder, className }: { value: string; onChange: (v: string) => void; placeholder: string; className?: string }) {
  return (
    <label className={clsx('group flex h-12 items-center gap-3 border-b border-line bg-transparent px-0 transition-colors duration-300 focus-within:border-accent', className)}>
      <MagnifyingGlassIcon weight="bold" className="size-[18px] text-subtle transition-colors duration-300 group-focus-within:text-accent" />
      <input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="h-full min-w-0 flex-1 bg-transparent text-[15px] outline-none placeholder:text-subtle" />
      {value && (
        <button onClick={() => onChange('')} className="grid size-6 place-items-center rounded-[6px] bg-white/[0.08] text-muted transition-colors hover:bg-white/12 hover:text-fg" aria-label="Очистить">
          <XIcon weight="bold" className="size-3" />
        </button>
      )}
    </label>
  )
}

export function Dropdown<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: string; swatch?: ReactNode }[]; onChange: (v: T) => void; label?: string }) {
  const [open, setOpen] = useState(false)
  const [mobile, setMobile] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  // A `position: fixed` sheet only works when it is not rendered inside a
  // transformed page wrapper. On phones we mount the sheet on <body> instead.
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 639px)')
    const sync = () => setMobile(mq.matches)
    sync()
    mq.addEventListener('change', sync)
    return () => mq.removeEventListener('change', sync)
  }, [])

  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => {
      const t = e.target as Node
      if (!ref.current?.contains(t) && !menuRef.current?.contains(t)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const menu = (
    <AnimatePresence>
      {open && (
        <>
          {/* затемнение под мобильным листом */}
          <motion.button
            type="button"
            tabIndex={-1}
            aria-hidden
            onClick={() => setOpen(false)}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="fixed inset-0 z-40 bg-bg/70 backdrop-blur-[2px] sm:hidden"
          />
          <motion.div
            ref={menuRef}
            role="listbox"
            initial={{ opacity: 0, y: 10, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 10, scale: 0.98 }}
            transition={{ duration: 0.2, ease: [0.22, 1, 0.36, 1] }}
            className={clsx(
              'glass z-50 origin-top-right rounded-[12px] p-1.5',
              mobile
                ? // на телефоне — лист снизу, ограниченный высотой экрана и прокручиваемый
                  'fixed inset-x-3 bottom-[max(16px,env(safe-area-inset-bottom))] max-h-[70dvh] overflow-y-auto overscroll-contain rounded-[16px] p-2'
                : 'absolute right-0 mt-2 min-w-52 max-h-64 overflow-y-auto overscroll-contain',
            )}
          >
            {label && <div className="mono px-3 pt-2 pb-1.5 text-[10px] font-medium tracking-[0.18em] text-subtle uppercase sm:hidden">{label}</div>}
            {options.map((o) => (
              <button
                key={o.value}
                type="button"
                role="option"
                aria-selected={o.value === value}
                onClick={() => {
                  onChange(o.value)
                  setOpen(false)
                }}
                className="flex w-full items-center justify-between gap-4 rounded-[8px] px-3 py-2.5 text-left text-[13px] font-medium transition-colors hover:bg-white/[0.06] max-sm:py-3.5 max-sm:text-[15px]"
              >
                <span className="flex min-w-0 items-center gap-2.5">
                  {o.swatch && <span className="grid size-4 shrink-0 place-items-center rounded-[5px] bg-white/10 ring-1 ring-white/15">{o.swatch}</span>}
                  <span className="truncate">{o.label}</span>
                </span>
                {o.value === value && <CheckIcon weight="bold" className="size-4 shrink-0 text-accent" />}
              </button>
            ))}
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className="flex h-9 items-center gap-1.5 rounded-[8px] border border-line bg-transparent px-3.5 text-[12px] font-medium tracking-[0.04em] transition-colors duration-300 hover:border-line-strong hover:bg-white/[0.04]"
      >
        {options.find((o) => o.value === value)?.label}
        <CaretDownIcon weight="bold" className={clsx('size-3.5 text-muted transition-transform duration-300', open && 'rotate-180')} />
      </button>
      {mobile ? createPortal(menu, document.body) : menu}
    </div>
  )
}

export function Segmented<T extends string>({ value, options, onChange, className }: { value: T; options: { value: T; label: ReactNode }[]; onChange: (v: T) => void; className?: string }) {
  return (
    <div className={clsx('flex gap-1 rounded-[8px] border border-line p-1', className)}>
      {options.map((o) => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          className={clsx(
            'h-8 flex-1 rounded-[6px] px-3.5 text-[12px] font-medium tracking-[0.03em] whitespace-nowrap transition-colors duration-300',
            o.value === value ? 'bg-accent text-white' : 'text-muted hover:text-fg',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

// модалка снизу

let scrollLocks = 0

export function Modal({ open, onClose, children, className }: { open: boolean; onClose: () => void; children: ReactNode; className?: string }) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    // Reference-counted lock: nested or quickly re-mounted sheets used to
    // leave body overflow hidden, which froze page scrolling on phones.
    scrollLocks += 1
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      scrollLocks = Math.max(0, scrollLocks - 1)
      if (scrollLocks === 0) document.body.style.overflow = ''
    }
  }, [open, onClose])

  return createPortal(
    <AnimatePresence>
      {open && (
        <div className="fixed inset-0 z-50 flex items-end justify-center sm:items-center sm:p-6">
          <motion.div
            className="absolute inset-0 bg-[#100a0c]/70 backdrop-blur-sm"
            onClick={onClose}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
          />
          <motion.div
            role="dialog"
            initial={{ opacity: 0, y: 28, scale: 0.985 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 18, scale: 0.99 }}
            transition={{ duration: 0.45, ease: [0.22, 1, 0.36, 1] }}
            className={clsx(
              'relative max-h-[92dvh] w-full overflow-y-auto rounded-t-[18px] bg-surface shadow-[0_32px_80px_-24px_rgba(0,0,0,0.9)] ring-1 ring-line sm:rounded-[18px]',
              className?.includes('max-w') ? className : ['sm:max-w-[420px]', className],
            )}
          >
            <div className="mx-auto mt-2.5 h-1 w-9 rounded-full bg-white/15 sm:hidden" />
            <button onClick={onClose} className="absolute top-4 right-4 z-10 grid size-8 place-items-center rounded-[7px] bg-white/[0.06] text-muted transition-colors hover:bg-white/10 hover:text-fg" aria-label="Закрыть">
              <XIcon weight="bold" className="size-3.5" />
            </button>
            {children}
          </motion.div>
        </div>
      )}
    </AnimatePresence>,
    document.body,
  )
}

// всякое мелкое

export function Badge({ tone = 'default', children }: { tone?: 'default' | 'accent' | 'success' | 'danger'; children: ReactNode }) {
  return (
    <span
      className={clsx(
        'mono inline-flex h-[22px] items-center rounded-[4px] border px-2 text-[10px] font-medium tracking-[0.1em] uppercase whitespace-nowrap',
        tone === 'default' && 'border-line bg-white/[0.03] text-muted',
        tone === 'accent' && 'border-accent/30 bg-accent/10 text-accent',
        tone === 'success' && 'border-success/30 bg-success/10 text-success',
        tone === 'danger' && 'border-danger/30 bg-danger/10 text-danger',
      )}
    >
      {children}
    </span>
  )
}

export function PageTitle({ title, subtitle }: { title: string; subtitle?: ReactNode }) {
  return (
    <div className="relative mb-8 border-b border-line pb-7 sm:mb-10 sm:pb-8">
      <div className="mb-4 flex items-center gap-3">
        <span className="line-in h-px w-10 bg-accent" />
        <span className="mono text-[10px] font-medium tracking-[0.22em] text-accent uppercase">Deltament / витрина</span>
      </div>
      <h1 className="display max-w-[760px] text-[28px] leading-[1] text-balance sm:text-[48px] sm:leading-[0.96]">{title}</h1>
      {subtitle && <p className="mt-4 max-w-xl text-[14px] leading-relaxed text-muted sm:text-[15px]">{subtitle}</p>}
    </div>
  )
}

export function Empty({ icon, title, text, action }: { icon: ReactNode; title: string; text: string; action?: ReactNode }) {
  return (
    <div className="panel flex flex-col items-center rounded-[12px] px-6 py-16 text-center">
      <div className="mb-4 grid size-12 place-items-center rounded-[8px] border border-line text-muted">{icon}</div>
      <p className="text-[17px] font-medium">{title}</p>
      <p className="mt-1 max-w-xs text-sm text-muted">{text}</p>
      {action && <div className="mt-6">{action}</div>}
    </div>
  )
}

export function Img({ src, alt, className }: { src: string; alt: string; className?: string }) {
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setLoaded(false)
    setFailed(false)
  }, [src])

  return (
    <span className={clsx('relative block overflow-hidden', className)}>
      {!loaded && !failed && <span className="skeleton absolute inset-0" />}
      {failed ? (
        <span className="grid size-full place-items-center bg-white/[0.04] text-subtle" aria-label={alt}>
          <GiftIcon className="size-7" />
        </span>
      ) : (
        <img
          src={src}
          alt={alt}
          loading="lazy"
          decoding="async"
          draggable={false}
          onLoad={() => setLoaded(true)}
          onError={() => setFailed(true)}
          className={clsx('size-full object-cover transition-opacity duration-500 select-none', loaded ? 'opacity-100' : 'opacity-0')}
        />
      )}
    </span>
  )
}

function Digit({ d }: { d: string }) {
  return (
    <span className="relative grid h-9 w-7 place-items-center overflow-hidden rounded-[10px] bg-surface text-[15px] font-bold tabular ring-1 ring-line">
      <AnimatePresence initial={false}>
        <motion.span
          key={d}
          className="absolute"
          initial={{ y: -16, opacity: 0 }}
          animate={{ y: 0, opacity: 1 }}
          exit={{ y: 16, opacity: 0 }}
          transition={{ duration: 0.3, ease: [0.16, 1, 0.3, 1] }}
        >
          {d}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}

// таймер как на фрагменте, цифры переворачиваются
export function Countdown({ ms }: { ms: number }) {
  const s = Math.max(0, Math.floor(ms / 1000))
  const d = Math.floor(s / 86400)
  const parts = [Math.floor((s % 86400) / 3600), Math.floor((s % 3600) / 60), s % 60].map((n) => String(n).padStart(2, '0'))
  return (
    <div className="flex items-center gap-1">
      {d > 0 && <span className="grid h-9 place-items-center rounded-[10px] bg-surface px-2.5 text-[15px] font-bold whitespace-nowrap ring-1 ring-line">{d} д</span>}
      {parts.map((p, i) => (
        <span key={i} className="flex items-center gap-1">
          {(i > 0 || d > 0) && <span className="px-0.5 font-bold text-subtle">:</span>}
          <Digit d={p[0]} />
          <Digit d={p[1]} />
        </span>
      ))}
    </div>
  )
}

export function Stagger({ i, children, className }: { i: number; children: ReactNode; className?: string }) {
  return (
    <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.6, ease, delay: Math.min(i, 10) * 0.045 }} className={className}>
      {children}
    </motion.div>
  )
}
