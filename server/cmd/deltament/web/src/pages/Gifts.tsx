import { useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router'
import { motion } from 'motion/react'
import { CaretDownIcon, CaretLeftIcon, GiftIcon, MagnifyingGlassIcon, SlidersHorizontalIcon } from '@phosphor-icons/react'
import clsx from 'clsx'
import { giftImg, type Gift } from '../lib/data'
import { GiftArt, useStaticFrame } from '../components/GiftArt'
import { useGift, useGifts } from '../lib/market'
import { fmtDate } from '../lib/format'
import { useRequireAuth, useStore } from '../lib/store'
import { Badge, Button, Dropdown, Empty, Gram, Img, Modal, PageTitle, SearchInput, softSpring, Stagger } from '../components/ui'

type Sort = 'price-desc' | 'price-asc' | 'new'
type StatusFilter = 'all' | 'sale' | 'sold'

const ALL = '' as const

const colorHex = (v: number) => `#${((v >>> 0) & 0xffffff).toString(16).padStart(6, '0')}`

// Small static previews for the attribute filters. They reuse the same cached
// frames as the gift cards, so they cost nothing after the first render.
function PatternSwatch({ patternId, color }: { patternId: number; color: number }) {
  const { tile } = useStaticFrame(patternId > 0 ? `/api/gift-anim/pattern/${patternId}` : null, true, color, 96)
  if (!tile) return <span className="size-full" />
  return <span className="size-full" style={{ backgroundImage: `url(${tile})`, backgroundSize: 'contain', backgroundPosition: 'center', backgroundRepeat: 'no-repeat' }} />
}

function ModelSwatch({ modelId }: { modelId: number }) {
  const { tile } = useStaticFrame(modelId > 0 ? `/api/gift-anim/model/${modelId}` : null, true, undefined, 96)
  if (!tile) return <span className="size-full" />
  return <span className="size-full" style={{ backgroundImage: `url(${tile})`, backgroundSize: 'contain', backgroundPosition: 'center', backgroundRepeat: 'no-repeat' }} />
}

export function GiftsPage() {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [collection, setCollection] = useState<string | null>(null)
  const [colQuery, setColQuery] = useState('')
  const [model, setModel] = useState<string | null>(null)
  const [backdrop, setBackdrop] = useState<string | null>(null)
  const [symbol, setSymbol] = useState<string | null>(null)
  const [sort, setSort] = useState<Sort>('price-desc')
  const [status, setStatus] = useState<StatusFilter>('all')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const { data, loading } = useGifts()
  const all = useMemo(() => data ?? [], [data])

  // Collections are derived from the listings that actually exist, so the list
  // only shows collections with at least one listed gift (not the static catalog).
  const collections = useMemo(() => {
    const map = new Map<string, { slug: string; name: string; count: number }>()
    for (const g of all) {
      const cur = map.get(g.slug)
      if (cur) cur.count++
      else map.set(g.slug, { slug: g.slug, name: g.collection, count: 1 })
    }
    return Array.from(map.values()).sort((a, b) => b.count - a.count)
  }, [all])

  const visibleCollections = useMemo(
    () => collections.filter((c) => c.name.toLowerCase().includes(colQuery.trim().toLowerCase())),
    [collections, colQuery],
  )

  // Attribute options. The model is only meaningful inside a collection, so it
  // is gated on a selected collection; backdrop and symbol are global. Each
  // option keeps the artwork id it needs so the filter can render a real
  // preview (colour for backdrops, the pattern/model frame for the others).
  type AttrOpt = { name: string; color?: number; patternId?: number; modelId?: number }

  const modelOptions = useMemo(() => {
    if (!collection) return [] as AttrOpt[]
    const map = new Map<string, AttrOpt>()
    for (const g of all) if (g.slug === collection && g.model.name && !map.has(g.model.name))
      map.set(g.model.name, { name: g.model.name, modelId: g.art?.modelId ?? 0 })
    return Array.from(map.values()).sort((a, b) => a.name.localeCompare(b.name))
  }, [all, collection])

  const backdropOptions = useMemo(() => {
    const map = new Map<string, AttrOpt>()
    for (const g of all) if (g.backdrop.name && !map.has(g.backdrop.name))
      map.set(g.backdrop.name, { name: g.backdrop.name, color: g.art?.colors?.center ?? g.art?.colors?.edge ?? 0 })
    return Array.from(map.values()).sort((a, b) => a.name.localeCompare(b.name))
  }, [all])

  const symbolOptions = useMemo(() => {
    const map = new Map<string, AttrOpt>()
    for (const g of all) if (g.symbol.name && !map.has(g.symbol.name))
      map.set(g.symbol.name, { name: g.symbol.name, patternId: g.art?.patternId ?? 0, color: g.art?.colors?.pattern ?? g.art?.colors?.text ?? 0 })
    return Array.from(map.values()).sort((a, b) => a.name.localeCompare(b.name))
  }, [all])

  const gifts = useMemo(() => {
    const q = query.trim().toLowerCase()
    return all
      .filter((g) => !collection || g.slug === collection)
      .filter((g) => status === 'all' || g.status === status)
      .filter((g) => !model || g.model.name === model)
      .filter((g) => !backdrop || g.backdrop.name === backdrop)
      .filter((g) => !symbol || g.symbol.name === symbol)
      .filter((g) => !q || `${g.collection} #${g.num} ${g.model.name} ${g.backdrop.name} ${g.symbol.name}`.toLowerCase().includes(q))
      .sort((a, b) => (sort === 'price-asc' ? a.price - b.price : sort === 'new' ? b.date - a.date : b.price - a.price))
  }, [all, query, collection, sort, status, model, backdrop, symbol])

  const activeCollection = collections.find((c) => c.slug === collection)

  if (all.length === 0) {
    return (
      <div className="mx-auto max-w-[760px]">
        <PageTitle title="Подарки" subtitle="Коллекционные подарки Telegram." />
        <div className="mt-8">
          <Empty icon={<GiftIcon className="size-6" />} title={loading ? 'Загружаем подарки…' : 'Пока нет подарков'} text={loading ? 'Секунду' : 'Новые подарки появятся здесь.'} />
        </div>
      </div>
    )
  }

  const pickCollection = (slug: string | null) => {
    setCollection(slug)
    // Model belongs to a collection: reset it when the collection changes.
    setModel(null)
    setFiltersOpen(false)
  }

  const sidebar = (
    <div className="space-y-3">
      <SearchInput value={query} onChange={setQuery} placeholder="Название или атрибут" />
      <div className="card overflow-hidden rounded-[12px]">
        <div className="flex items-center justify-between px-4 pt-4 pb-2 text-[14px] font-semibold">
          Коллекции <span className="text-[12px] font-medium text-subtle">{collections.length}</span>
        </div>
        <div className="px-2 pb-2">
          <input
            value={colQuery}
            onChange={(e) => setColQuery(e.target.value)}
            placeholder="Поиск коллекций"
            className="mb-1.5 h-9 w-full rounded-full bg-white/6 px-4 text-[13px] outline-none placeholder:text-subtle focus:bg-white/8"
          />
          <div className="max-h-[440px] overflow-y-auto">
            <CollectionRow active={!collection} onClick={() => pickCollection(null)} label="Все коллекции" count={all.length} />
            {visibleCollections.map((c) => (
              <CollectionRow key={c.slug} active={collection === c.slug} onClick={() => pickCollection(c.slug)} label={c.name} count={c.count} img={giftImg(c.slug, 1)} />
            ))}
          </div>
        </div>
      </div>
    </div>
  )

  const modelDropdown = collection ? (
    <Dropdown
      label="Модель"
      value={model ?? ALL}
      options={[{ value: ALL, label: 'Все модели' }, ...modelOptions.map((o) => ({ value: o.name, label: o.name, swatch: <ModelSwatch modelId={o.modelId ?? 0} /> }))]}
      onChange={(v) => setModel(v === ALL ? null : v)}
    />
  ) : (
    <button type="button" disabled className="flex h-9 items-center gap-1.5 rounded-[8px] border border-line bg-transparent px-3.5 text-[12px] font-medium tracking-[0.04em] text-muted/70">
      Модель <CaretDownIcon weight="bold" className="size-3.5" />
    </button>
  )

  return (
    <div className="grid gap-6 lg:grid-cols-[264px_1fr]">
      <aside className="hidden lg:block">
        <div className="sticky top-24">{sidebar}</div>
      </aside>

      <section>
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <motion.h1 key={collection ?? 'all'} initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }} className="display text-[22px] leading-none sm:text-[30px]">
              {activeCollection ? activeCollection.name : 'Топ подарков'}
            </motion.h1>
            <p className="mt-0.5 text-sm text-muted">{gifts.length} предметов</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="glass" size="sm" className="h-9 lg:hidden" onClick={() => setFiltersOpen(true)}>
              <SlidersHorizontalIcon className="size-4" /> Коллекции
            </Button>
            {modelDropdown}
            <Dropdown
              label="Фон"
              value={backdrop ?? ALL}
              options={[{ value: ALL, label: 'Все фоны' }, ...backdropOptions.map((o) => ({ value: o.name, label: o.name, swatch: <span className="size-full rounded-full" style={{ background: colorHex(o.color ?? 0) }} /> }))]}
              onChange={(v) => setBackdrop(v === ALL ? null : v)}
            />
            <Dropdown
              label="Символ"
              value={symbol ?? ALL}
              options={[{ value: ALL, label: 'Все символы' }, ...symbolOptions.map((o) => ({ value: o.name, label: o.name, swatch: <PatternSwatch patternId={o.patternId ?? 0} color={o.color ?? 0} /> }))]}
              onChange={(v) => setSymbol(v === ALL ? null : v)}
            />
            <Dropdown
              label="Статус"
              value={status}
              options={[
                { value: 'all', label: 'Все подарки' },
                { value: 'sale', label: 'Продаются' },
                { value: 'sold', label: 'Проданы' },
              ]}
              onChange={setStatus}
            />
            <Dropdown
              label="Сортировка"
              value={sort}
              options={[
                { value: 'price-desc', label: 'Сначала дорогие' },
                { value: 'price-asc', label: 'Сначала дешёвые' },
                { value: 'new', label: 'Сначала новые' },
              ]}
              onChange={setSort}
            />
          </div>
        </div>

        {gifts.length === 0 ? (
          <Empty icon={<MagnifyingGlassIcon className="size-6" />} title="Ничего не найдено" text="Попробуйте другой запрос или коллекцию" />
        ) : (
          <motion.div
            key={`${collection}-${status}-${sort}-${query}-${model}-${backdrop}-${symbol}`}
            initial="hidden"
            animate="show"
            variants={{ show: { transition: { staggerChildren: 0.025 } } }}
            className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4"
          >
            {gifts.map((g) => (
              <motion.div key={g.id} variants={{ hidden: { opacity: 0, y: 16, scale: 0.97 }, show: { opacity: 1, y: 0, scale: 1, transition: softSpring } }}>
                <GiftCard gift={g} onClick={() => navigate(`/gifts/${g.id}`)} />
              </motion.div>
            ))}
          </motion.div>
        )}
      </section>

      <Modal open={filtersOpen} onClose={() => setFiltersOpen(false)}>
        <div className="p-5 pt-14">{sidebar}</div>
      </Modal>
    </div>
  )
}

function GiftCard({ gift, onClick }: { gift: Gift; onClick: () => void }) {
  return (
    <motion.button
      onClick={onClick}
      whileHover={{ y: -3 }}
      whileTap={{ scale: 0.97 }}
      transition={{ type: 'spring', bounce: 0.3, duration: 0.4 }}
      className="card group block w-full overflow-hidden rounded-[14px] text-left"
    >
      <div className="relative overflow-hidden">
        <GiftArt art={gift.art} slug={gift.slug} num={gift.num} title={gift.collection} className="aspect-square transition-transform duration-700 ease-out group-hover:scale-[1.06]" />
        <span className="absolute top-2 left-2 flex flex-col items-start gap-1">
          {gift.unique && <Badge tone="success">NFT</Badge>}
        </span>
      </div>
      <div className="p-3">
        <p className="truncate text-[14px] font-semibold">
          {gift.collection} <span className="text-muted">#{gift.num}</span>
        </p>
        <p className="mt-0.5 truncate text-[11px] text-subtle">{fmtDate(gift.date)}</p>
        <div className="mt-2 flex items-center justify-between gap-2">
          <Gram value={gift.price} className="text-[14px] font-bold" />
          {gift.status === 'sold' ? <Badge tone="danger">Продан</Badge> : <Badge tone="success">В продаже</Badge>}
        </div>
      </div>
    </motion.button>
  )
}

function CollectionRow({ active, onClick, label, count, img }: { active: boolean; onClick: () => void; label: string; count: number; img?: string }) {
  return (
    <button
      onClick={onClick}
      className={clsx('relative flex w-full items-center gap-2.5 rounded-[14px] px-2.5 py-2 text-left text-[13px] transition-colors', active ? 'bg-accent/15 font-semibold text-link' : 'font-medium hover:bg-white/5')}
    >
      {img ? <Img src={img} alt="" className="relative size-7 rounded-[9px]" /> : <span className="relative size-7 rounded-[9px] bg-white/10" />}
      <span className="relative flex-1 truncate">{label}</span>
      <span className={clsx('relative text-xs tabular', active ? 'text-link' : 'text-subtle')}>{count}</span>
    </button>
  )
}

export function GiftPage() {
  const { id = '' } = useParams()
  const { data: gift, loading } = useGift(id)
  if (loading && !gift) return null
  if (!gift) return <Navigate to="/gifts" replace />
  return <GiftView gift={gift} />
}

function GiftView({ gift }: { gift: Gift }) {
  const { buyLot, toast, inventory, cancelListing } = useStore()
  const { data: same } = useGifts()
  const requireAuth = useRequireAuth()
  const navigate = useNavigate()
  const owned = inventory.some((i) => i.kind === 'gift' && i.slug === gift.slug && i.num === gift.num)
  const similar = (same ?? []).filter((g) => g.slug === gift.slug && g.id !== gift.id).slice(0, 4)
  const isMine = gift.mine === true

  const buy = () =>
    requireAuth(() => {
      buyLot(gift.id).then((item) => item && toast(`${gift.collection} #${gift.num} в вашем инвентаре`, 'success'))
    })

  const attrs = [
    { label: 'Модель', ...gift.model },
    { label: 'Фон', ...gift.backdrop },
    { label: 'Символ', ...gift.symbol },
  ]

  return (
    <div className="mx-auto max-w-[920px]">
      <Link to="/gifts" className="mb-5 inline-flex items-center gap-1 text-[14px] font-semibold text-link transition-opacity hover:opacity-80">
        <CaretLeftIcon className="size-4" /> Подарки
      </Link>

      <div className="grid gap-6 md:grid-cols-[1fr_1fr]">
        <Stagger i={0}>
          <div className="relative overflow-hidden rounded-[32px]">
            <motion.div initial={{ scale: 1.08 }} animate={{ scale: 1 }} transition={{ type: 'spring', bounce: 0, duration: 1.2 }}>
              <GiftArt art={gift.art} slug={gift.slug} num={gift.num} title={gift.collection} className="aspect-square" priority animate />
            </motion.div>
          </div>
        </Stagger>

        <div className="flex flex-col">
          <Stagger i={1}>
            <p className="text-[13px] font-semibold text-muted">Коллекционный подарок</p>
            <h1 className="display mt-1 text-[24px] leading-tight break-words sm:text-[34px]">
              {gift.collection} <span className="text-muted">#{gift.num}</span>
            </h1>
            <div className="mt-2 flex flex-wrap gap-2">
              {gift.status === 'sale' ? <Badge tone="success">В продаже</Badge> : <Badge tone="danger">Продан</Badge>}
              {gift.unique && <Badge tone="success">NFT</Badge>}
              {isMine && <Badge tone="accent">Ваш лот</Badge>}
              {owned && <Badge tone="accent">У вас в инвентаре</Badge>}
            </div>
            <div className="mt-3 flex flex-wrap gap-1.5">
              {attrs.map((a) =>
                a.name ? (
                  <span key={a.label} className="rounded-full border border-line bg-white/[0.03] px-2.5 py-1 text-[11px] font-medium text-muted">
                    <span className="text-subtle">{a.label}:</span> {a.name}
                  </span>
                ) : null,
              )}
            </div>
          </Stagger>

          <Stagger i={2} className="card mt-6 overflow-hidden rounded-[12px]">
            {attrs.map((a, i) => (
              <div key={a.label} className={clsx('flex items-center justify-between gap-3 px-5 py-3.5 text-[14px]', i > 0 && 'border-t border-white/5')}>
                <span className="font-semibold">{a.label}</span>
                <span className="flex min-w-0 items-center gap-2 font-medium text-link">
                  <span className="truncate">{a.name || '—'}</span>
                  {a.rarity > 0 && <span className="rounded-full bg-white/8 px-2 py-0.5 text-[11px] font-semibold text-muted tabular">{a.rarity}%</span>}
                </span>
              </div>
            ))}
            <div className="flex items-center justify-between border-t border-white/5 px-5 py-3.5 text-[14px]">
              <span className="font-semibold">{gift.status === 'sold' ? 'Продан' : 'Выставлен'}</span>
              <span className="font-medium text-muted">{fmtDate(gift.date)}</span>
            </div>
          </Stagger>

          <Stagger i={3} className="mt-auto pt-6">
            <div className="card flex items-center justify-between rounded-[12px] p-5">
              <div>
                <p className="text-[13px] font-semibold text-muted">{gift.status === 'sold' ? 'Продан за' : 'Цена'}</p>
                <Gram value={gift.price} usd className="mt-1.5 text-[22px] font-bold" size="size-5" />
              </div>
              {gift.status === 'sale' &&
                (isMine ? (
                  <Button
                    size="lg"
                    variant="glass"
                    className="px-8"
                    onClick={() => cancelListing(gift.id).then((ok) => ok && toast('Лот снят с продажи', 'success'))}
                  >
                    Снять с продажи
                  </Button>
                ) : (
                  <Button size="lg" onClick={buy} className="px-8">
                    Купить
                  </Button>
                ))}
            </div>
          </Stagger>
        </div>
      </div>

      {similar.length > 0 && (
        <Stagger i={4} className="mt-12">
          <h2 className="display mb-4 px-1 text-[20px] leading-none sm:text-[24px]">Ещё из коллекции</h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {similar.map((g) => (
              <GiftCard key={g.id} gift={g} onClick={() => navigate(`/gifts/${g.id}`)} />
            ))}
          </div>
        </Stagger>
      )}
    </div>
  )
}
