// Gift art is proxied and cached by our own origin. Mobile clients used to
// hotlink nft.fragment.com, which was slow or blocked in some networks.
export const giftImg = (slug: string, num: number) => `/api/gift-art/${slug}-${num}.webp`

// подарки

export type Collection = { slug: string; name: string; supply: number; floor: number }

export const COLLECTIONS: Collection[] = [
  { slug: 'plushpepe', name: 'Plush Pepe', supply: 2852, floor: 5200 },
  { slug: 'heroichelmet', name: 'Heroic Helmet', supply: 4410, floor: 2400 },
  { slug: 'durovscap', name: "Durov's Cap", supply: 3232, floor: 2100 },
  { slug: 'preciouspeach', name: 'Precious Peach', supply: 2986, floor: 1600 },
  { slug: 'heartlocket', name: 'Heart Locket', supply: 1880, floor: 1400 },
  { slug: 'swisswatch', name: 'Swiss Watch', supply: 8140, floor: 115 },
  { slug: 'signetring', name: 'Signet Ring', supply: 7120, floor: 105 },
  { slug: 'astralshard', name: 'Astral Shard', supply: 2982, floor: 95 },
  { slug: 'genielamp', name: 'Genie Lamp', supply: 6540, floor: 75 },
  { slug: 'ionicdryer', name: 'Ionic Dryer', supply: 9800, floor: 50 },
  { slug: 'magicpotion', name: 'Magic Potion', supply: 5210, floor: 48 },
  { slug: 'vintagecigar', name: 'Vintage Cigar', supply: 7600, floor: 40 },
  { slug: 'kissedfrog', name: 'Kissed Frog', supply: 11200, floor: 35 },
  { slug: 'eternalrose', name: 'Eternal Rose', supply: 5745, floor: 28 },
  { slug: 'diamondring', name: 'Diamond Ring', supply: 8606, floor: 24 },
  { slug: 'electricskull', name: 'Electric Skull', supply: 4352, floor: 25 },
  { slug: 'crystalball', name: 'Crystal Ball', supply: 8412, floor: 16 },
  { slug: 'toybear', name: 'Toy Bear', supply: 21400, floor: 14 },
  { slug: 'lovepotion', name: 'Love Potion', supply: 12100, floor: 12 },
  { slug: 'skullflower', name: 'Skull Flower', supply: 14800, floor: 9 },
  { slug: 'evileye', name: 'Evil Eye', supply: 18555, floor: 8 },
  { slug: 'deskcalendar', name: 'Desk Calendar', supply: 34131, floor: 5 },
  { slug: 'bdaycandle', name: 'B-Day Candle', supply: 24359, floor: 5 },
  { slug: 'lunarsnake', name: 'Lunar Snake', supply: 41000, floor: 4 },
]

export const BACKDROPS = [
  { name: 'Onyx Black', color: '#1d1f24' },
  { name: 'Midnight Blue', color: '#1f2c4d' },
  { name: 'Emerald', color: '#1f4a3c' },
  { name: 'Coral Red', color: '#5a2a2c' },
  { name: 'Lavender', color: '#3d3266' },
  { name: 'Steel Grey', color: '#30363f' },
  { name: 'Sapphire', color: '#203a63' },
  { name: 'Moonstone', color: '#3c4552' },
] as const

// Real artwork metadata of a DeltaGram gift. Upgraded (NFT) gifts compose a
// model + pattern Lottie over the backdrop palette; regular gifts use the
// catalog animation keyed by giftId.
export type GiftColors = { center: number; edge: number; pattern: number; text: number }

export type GiftArt = {
  giftId: number
  modelId: number
  backdropId: number
  patternId: number
  unique: boolean
  colors: GiftColors
}

export const EMPTY_ART: GiftArt = {
  giftId: 0,
  modelId: 0,
  backdropId: 0,
  patternId: 0,
  unique: false,
  colors: { center: 0, edge: 0, pattern: 0, text: 0 },
}

export type Gift = {
  id: string
  slug: string
  collection: string
  num: number
  price: number
  startPrice: number
  minBid: number
  status: 'auction' | 'sale' | 'sold'
  date: number
  endsAt: number
  mine?: boolean
  unique: boolean
  bids: { user: string; amount: number; ts: number }[]
  model: { name: string; rarity: number }
  backdrop: { name: string; rarity: number }
  symbol: { name: string; rarity: number }
  art: GiftArt
}

export const backdropColor = (name: string) => BACKDROPS.find((b) => b.name === name)?.color ?? '#1d1f24'

// юзернеймы и номера

export type LotStatus = 'auction' | 'sale' | 'sold'

export type Lot = {
  id: string
  kind: 'username' | 'number' | 'gift'
  name: string
  slug: string
  price: number
  startPrice: number
  minBid: number
  status: LotStatus
  endsAt: number
  mine?: boolean
  bids: { user: string; amount: number; ts: number }[]
}

export const lotUrl = (lot: Lot) =>
  lot.kind === 'username' ? `/usernames/${lot.slug}` : lot.kind === 'gift' ? `/gifts/${lot.slug}` : `/numbers/${lot.slug}`

// приватка, к которой привязан маркет
export type Server = { slug: string; name: string; topup: boolean; starsPerGram: number }

export type Country = { name: string; code: string; mask: string; min?: number }

export const COUNTRIES: Country[] = [
  // анонимные номера DeltaGram: код 888 и 4–12 цифр после него
  { name: 'Анонимный +888', code: '888', mask: 'XXXX XXXX XXXX', min: 4 },
  { name: 'Россия', code: '7', mask: 'XXX XXX XX XX' },
  { name: 'Казахстан', code: '7', mask: 'XXX XXX XX XX' },
  { name: 'Беларусь', code: '375', mask: 'XX XXX XX XX' },
  { name: 'Украина', code: '380', mask: 'XX XXX XX XX' },
  { name: 'Узбекистан', code: '998', mask: 'XX XXX XX XX' },
  { name: 'Германия', code: '49', mask: 'XXX XXXXXXXX' },
  { name: 'США', code: '1', mask: 'XXX XXX XXXX' },
]

export const DEFAULT_COUNTRY = COUNTRIES.find((c) => c.name === 'Россия') ?? COUNTRIES[0]
