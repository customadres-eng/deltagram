import { useEffect, useState } from 'react'
import { api, withServer } from './api'
import { useStore } from './store'
import type { Gift, GiftArt, Lot, Server } from './data'

type Remote<T> = { data: T | null; loading: boolean; failed: boolean }

// грузим с бэка и перезапрашиваем после входа или покупки
function useRemote<T>(path: string | null): Remote<T> {
  const { version } = useStore()
  const [state, setState] = useState<Remote<T>>({ data: null, loading: true, failed: false })

  useEffect(() => {
    if (!path) return
    let alive = true
    setState((s) => ({ ...s, loading: true }))
    api<T>(withServer(path))
      .then((data) => alive && setState({ data, loading: false, failed: false }))
      .catch(() => alive && setState({ data: null, loading: false, failed: true }))
    return () => {
      alive = false
    }
  }, [path, version])

  return state
}

export const useServers = () => useRemote<Server[]>('/api/servers')
export type AssetInventory = {
  channels: { id: string; title: string; username: string }[]
  usernames: { id: string; username: string }[]
  listings: Lot[]
  gifts: GiftAsset[]
}

export type GiftTrait = { name: string; rarity: number }

// Real gift owned by a DeltaGram account, returned by /api/inventory and
// /api/my-gifts. The collection is a catalog revision title; model/backdrop/
// symbol come from unique_star_gifts attributes.
export type GiftAsset = {
  rowId: string
  giftId: string
  title: string
  slug: string
  num: number
  date: number
  unique: boolean
  model: GiftTrait
  backdrop: GiftTrait
  symbol: GiftTrait
  art: GiftArt
}

export const useInventory = (enabled = true) => useRemote<AssetInventory>(enabled ? '/api/inventory' : null)
export const useMyGifts = (enabled = true) => useRemote<{ gifts: GiftAsset[] }>(enabled ? '/api/my-gifts' : null)
export const useLots = (kind: Lot['kind']) => useRemote<Lot[]>(`/api/lots?kind=${kind}`)
export const useLot = (kind: Lot['kind'], slug: string) => useRemote<Lot>(`/api/lots/${kind}/${encodeURIComponent(slug)}`)
export const useGifts = () => useRemote<Gift[]>('/api/gifts')
export const useGift = (id: string) => useRemote<Gift>(`/api/gifts/${encodeURIComponent(id)}`)
