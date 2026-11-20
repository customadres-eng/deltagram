import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate, useOutlet } from 'react-router'
import { AnimatePresence, motion, useScroll, useSpring } from 'motion/react'
import { AtIcon, CaretDownIcon, CheckCircleIcon, GiftIcon, InfoIcon, PackageIcon, PhoneIcon, SignOutIcon, UserIcon, WalletIcon, XCircleIcon } from '@phosphor-icons/react'
import clsx from 'clsx'
import { useStore } from '../lib/store'
import { Button, Gram, Logo } from './ui'
import { AuthModal } from './AuthModal'

const NAV = [
  { to: '/usernames', label: 'Юзернеймы', short: 'Имена', icon: AtIcon },
  { to: '/numbers', label: 'Номера', short: 'Номера', icon: PhoneIcon },
  { to: '/gifts', label: 'Подарки', short: 'Подарки', icon: GiftIcon },
]

/* Expo-out: fast start, long settle. Used everywhere instead of default easing. */
export const ease: [number, number, number, number] = [0.22, 1, 0.36, 1]

function DesktopNav() {
  return (
    <nav className="hidden items-center gap-7 lg:flex">
      {NAV.map((n) => (
        <NavLink key={n.to} to={n.to} className={({ isActive }) => clsx('group relative py-2 text-[12px] font-medium tracking-[0.14em] uppercase transition-colors duration-300', isActive ? 'text-fg' : 'text-muted hover:text-fg')}>
          {({ isActive }) => (
            <>
              {n.label}
              <span className={clsx('absolute inset-x-0 -bottom-px h-px origin-left bg-accent transition-transform duration-500 ease-[cubic-bezier(0.22,1,0.36,1)]', isActive ? 'scale-x-100' : 'scale-x-0 group-hover:scale-x-40')} />
            </>
          )}
        </NavLink>
      ))}
    </nav>
  )
}

// нижнее меню для телефона
function TabBar() {
  const { user, openAuth } = useStore()
  const items = [...NAV, { to: '/profile', label: 'Профиль', short: 'Профиль', icon: UserIcon }]
  return (
    <nav className="fixed inset-x-0 bottom-0 z-40 px-3 pb-[max(12px,env(safe-area-inset-bottom))] lg:hidden">
      <div className="glass mx-auto flex w-full max-w-md items-center justify-between rounded-[14px] p-1">
        {items.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            onClick={(e) => {
              if (n.to === '/profile' && !user) {
                e.preventDefault()
                openAuth()
              }
            }}
            className={({ isActive }) => clsx('relative flex flex-1 flex-col items-center gap-1 rounded-[10px] py-2 text-[10px] font-medium tracking-wide transition-colors duration-300', isActive ? 'text-fg' : 'text-muted')}
          >
            {({ isActive }) => (
              <>
                <n.icon weight={isActive ? 'fill' : 'regular'} className={clsx('size-[21px] transition-colors', isActive && 'text-accent')} />
                {n.short}
                {isActive && <motion.span layoutId="tab-dot" className="absolute -bottom-0.5 size-1 rounded-full bg-accent" transition={{ type: 'spring', bounce: 0.2, duration: 0.5 }} />}
              </>
            )}
          </NavLink>
        ))}
      </div>
    </nav>
  )
}

function UserMenu() {
  const { user, balance, logout } = useStore()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    setOpen(false)
  }, [location.pathname])

  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  if (!user) return null

  const items = [
    { icon: UserIcon, label: 'Профиль', to: '/profile' },
    { icon: PackageIcon, label: 'Инвентарь', to: '/inventory' },
    { icon: WalletIcon, label: 'Пополнить баланс', to: '/profile?topup=1' },
  ]

  return (
    <div ref={ref} className="relative flex items-center gap-2">
      <Link to="/profile" className="flex h-9 items-center rounded-[8px] border border-line px-3 text-[13px] font-medium transition-colors hover:border-line-strong hover:bg-white/[0.04]">
        <Gram value={balance} size="size-4" />
      </Link>
      <button onClick={() => setOpen((o) => !o)} className="flex h-9 items-center gap-2 rounded-[8px] border border-line pr-2.5 pl-1 transition-colors hover:border-line-strong hover:bg-white/[0.04]">
        <span className="grid size-7 place-items-center rounded-[6px] bg-accent text-[12px] font-extrabold text-white">{user.username[0].toUpperCase()}</span>
        <span className="hidden text-[13px] font-medium md:block">{user.username}</span>
        <CaretDownIcon weight="bold" className={clsx('size-3.5 text-muted transition-transform duration-300', open && 'rotate-180')} />
      </button>
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, y: -6, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -6, scale: 0.98 }}
            transition={{ duration: 0.24, ease }}
            className="glass absolute top-12 right-0 z-40 w-60 origin-top-right rounded-[12px] p-1.5"
          >
            <div className="px-3 pt-2.5 pb-3">
              <p className="text-[15px] font-semibold">@{user.username}</p>
              <Gram value={balance} className="mt-1 text-xs font-medium text-muted" size="size-3" />
            </div>
            {items.map((i) => (
              <button key={i.label} onClick={() => navigate(i.to)} className="flex w-full items-center gap-3 rounded-[8px] px-3 py-2.5 text-[13px] font-medium transition-colors hover:bg-white/[0.06]">
                <i.icon weight="duotone" className="size-[18px] text-muted" />
                {i.label}
              </button>
            ))}
            <div className="mx-3 my-1 h-px bg-line" />
            <button
              onClick={() => {
                logout()
                navigate('/usernames')
              }}
              className="flex w-full items-center gap-3 rounded-[8px] px-3 py-2.5 text-[13px] font-medium text-danger transition-colors hover:bg-danger/10"
            >
              <SignOutIcon weight="duotone" className="size-[18px]" />
              Выйти
            </button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

function Toasts() {
  const { toasts } = useStore()
  return (
    <div className="pointer-events-none fixed inset-x-0 top-4 z-[60] flex flex-col items-center gap-2 px-4">
      <AnimatePresence>
        {toasts.map((t) => (
          <motion.div
            key={t.id}
            initial={{ opacity: 0, y: -14, scale: 0.97 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -8, scale: 0.98 }}
            transition={{ duration: 0.45, ease }}
            className="glass flex items-center gap-2.5 rounded-[10px] py-2.5 pr-5 pl-3 text-[13px] font-medium"
          >
            {t.tone === 'error' ? (
              <XCircleIcon weight="fill" className="size-5 text-danger" />
            ) : t.tone === 'success' ? (
              <CheckCircleIcon weight="fill" className="size-5 text-success" />
            ) : (
              <InfoIcon weight="fill" className="size-5 text-link" />
            )}
            {t.title}
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  )
}

// держим старую страницу пока она плавно уходит
function FrozenOutlet() {
  const outlet = useOutlet()
  const [frozen, setFrozen] = useState(outlet)

  // на первом рендере редирект может прийти раньше аутлета, подхватываем его сразу
  useEffect(() => {
    if (outlet && outlet !== frozen) setFrozen(outlet)
  }, [outlet, frozen])

  return frozen
}

export function Layout() {
  const { user, openAuth } = useStore()
  const location = useLocation()
  const { scrollYProgress } = useScroll()
  const progress = useSpring(scrollYProgress, { stiffness: 140, damping: 30, restDelta: 0.001 })

  return (
    <div className="grain relative flex min-h-dvh flex-col">
      <motion.div style={{ scaleX: progress }} className="fixed inset-x-0 top-0 z-50 h-px origin-left bg-accent" />
      <header className="sticky top-0 z-40 border-b border-line bg-bg/85 backdrop-blur-md">
        <div className="mx-auto flex h-[58px] max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
          <Link to="/" aria-label="Deltament" className="transition-opacity duration-300 hover:opacity-80">
            <Logo />
          </Link>
          <DesktopNav />
          {user ? (
            <UserMenu />
          ) : (
            <Button size="sm" onClick={openAuth} className="h-9 px-5">
              Войти
            </Button>
          )}
        </div>
      </header>

      <main className="relative z-[2] mx-auto w-full max-w-6xl flex-1 px-4 pt-12 pb-32 sm:px-6 lg:pt-16 lg:pb-24">
        <AnimatePresence mode="wait" initial={false} onExitComplete={() => window.scrollTo({ top: 0, behavior: 'instant' })}>
          <motion.div
            key={location.pathname}
            initial={{ opacity: 0, y: 14 }}
            animate={{ opacity: 1, y: 0, transition: { duration: 0.5, ease } }}
            exit={{ opacity: 0, y: -6, transition: { duration: 0.18, ease: 'easeIn' } }}
          >
            <FrozenOutlet />
          </motion.div>
        </AnimatePresence>
      </main>

      <footer className="relative z-[2] hidden border-t border-line lg:block">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-3 px-6 py-7">
          <span className="mono text-[11px] tracking-[0.14em] text-subtle uppercase">© 2026 Deltament</span>
          <div className="flex gap-6 text-[12px] font-medium tracking-wide">
            <Link to="/usernames" className="text-muted transition-colors hover:text-fg">Витрина</Link>
            <Link to="/inventory" className="text-muted transition-colors hover:text-fg">Инвентарь</Link>
          </div>
        </div>
      </footer>

      <TabBar />
      <AuthModal />
      <Toasts />
    </div>
  )
}
