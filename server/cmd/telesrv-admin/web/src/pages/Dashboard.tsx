import {
  Activity,
  AlertTriangle,
  BadgeCheck,
  Bot,
  Cpu,
  Database,
  Film,
  Flag,
  HardDrive,
  MemoryStick,
  Radio,
  Smile,
  Sticker,
  TrendingUp,
  Users,
  UsersRound
} from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api";
import { Alert } from "../components/ui";
import { useI18n, type TFunction } from "../i18n";
import { formatBytes, formatDate, formatQuantity } from "../lib/format";
import type { Navigate } from "../routing";
import type { DashboardResponse, StorageStatsResponse } from "../types";

const peakRanges = ["24h", "7d", "30d", "90d", "all"] as const;
type PeakRange = (typeof peakRanges)[number];

export function Dashboard({ navigate }: { navigate: Navigate }) {
  const { t } = useI18n();
  const [data, setData] = useState<DashboardResponse | null>(null);
  const [error, setError] = useState("");
  const [peakRange, setPeakRange] = useState<PeakRange>("24h");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const res = await api.dashboard(peakRange);
        if (!cancelled) {
          setData(res);
          setError("");
        }
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : t("dashboard.loadError"));
      }
    }
    void load();
    const timer = window.setInterval(() => void load(), 15000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [t, peakRange]);

  const counts = data?.counts;
  const storage = data?.storage;
  const host = data?.host;
  const peak = data?.peakOnline;

  return (
    <div className="dashboard-layout">
      {error && <Alert>{error}</Alert>}

      <Section title={t("dashboard.needsAttention")}>
        <StatTile
          icon={<Flag />}
          label={t("dashboard.pendingReports")}
          value={counts ? formatQuantity(String(counts.PendingReports)) : "…"}
          tone={counts && counts.PendingReports > 0 ? "warn" : "good"}
          href="/moderation"
          navigate={navigate}
        />
        <StatTile
          icon={<BadgeCheck />}
          label={t("dashboard.verificationRequests")}
          value={counts ? formatQuantity(String(counts.PendingVerifications)) : "…"}
          tone={counts && counts.PendingVerifications > 0 ? "warn" : "good"}
          href="/verification"
          navigate={navigate}
        />
      </Section>

      <Section
        title={t("dashboard.peopleChats")}
        action={
          <label className="dashboard-range">
            <span>{t("dashboard.rangeLabel")}</span>
            <select value={peakRange} onChange={(event) => setPeakRange(event.target.value as PeakRange)}>
              {peakRanges.map((value) => (
                <option key={value} value={value}>
                  {t(`dashboard.range.${value}`)}
                </option>
              ))}
            </select>
          </label>
        }
      >
        <StatTile icon={<Users />} label={t("dashboard.users")} value={counts ? formatQuantity(String(counts.Users)) : "…"} href="/accounts" navigate={navigate} />
        <StatTile
          icon={<Activity />}
          label={t("dashboard.onlineNow")}
          value={counts ? formatQuantity(String(counts.OnlineUsers)) : "…"}
          sub={t("dashboard.lastFiveMinutes")}
          href="/accounts"
          navigate={navigate}
        />
        <StatTile
          icon={<TrendingUp />}
          label={t("dashboard.peakOnline")}
          value={peak ? formatQuantity(String(peak.value)) : "…"}
          sub={
            peak && peak.at
              ? t("dashboard.peakAt", { time: formatDate(peak.at) })
              : t("dashboard.peakNoData")
          }
          href="/accounts"
          navigate={navigate}
        />
        <StatTile icon={<Bot />} label={t("dashboard.bots")} value={counts ? formatQuantity(String(counts.Bots)) : "…"} href="/bots" navigate={navigate} />
        <StatTile
          icon={<Radio />}
          label={t("dashboard.channels")}
          value={counts ? formatQuantity(String(counts.BroadcastChannels)) : "…"}
          href="/channels"
          navigate={navigate}
        />
        <StatTile
          icon={<UsersRound />}
          label={t("dashboard.supergroups")}
          value={counts ? formatQuantity(String(counts.Supergroups)) : "…"}
          href="/channels"
          navigate={navigate}
        />
      </Section>

      <Section title={t("dashboard.content")}>
        <StatTile
          icon={<Sticker />}
          label={t("dashboard.stickerPacks")}
          value={counts ? formatQuantity(String(counts.StickerSets)) : "…"}
          href="/stickers"
          navigate={navigate}
        />
        <StatTile
          icon={<Smile />}
          label={t("dashboard.emojiPacks")}
          value={counts ? formatQuantity(String(counts.EmojiSets)) : "…"}
          href="/emoji"
          navigate={navigate}
        />
        <StatTile
          icon={<Film />}
          label={t("dashboard.gifs")}
          value={counts ? formatQuantity(String(counts.Gifs)) : "…"}
          sub={t("dashboard.savedByUsers")}
          href="/gif-catalog"
          navigate={navigate}
        />
        <StatTile
          icon={<Database />}
          label={t("dashboard.mediaStorageUsed")}
          value={storage ? formatBytes(storage.PhysicalBytes) : "…"}
          sub={storage ? storageBackendLabel(storage, t) : undefined}
          href="/storage"
          navigate={navigate}
        />
      </Section>

      <Section title={t("dashboard.serverHealth")} hint={host?.Ready ? undefined : t("dashboard.waitingForSample")}>
        <UsageTile
          icon={<Cpu />}
          label={t("dashboard.cpuLoad")}
          percent={host?.Ready ? host.CPUPercent : undefined}
          valueText={host?.Ready ? `${host.CPUPercent.toFixed(0)}%` : "…"}
        />
        <UsageTile
          icon={<MemoryStick />}
          label={t("dashboard.ramUsed")}
          percent={host?.Ready && host.MemTotalBytes > 0 ? (host.MemUsedBytes / host.MemTotalBytes) * 100 : undefined}
          valueText={host?.Ready ? formatBytes(String(host.MemUsedBytes)) : "…"}
          sub={host?.Ready ? t("dashboard.ofTotal", { total: formatBytes(String(host.MemTotalBytes)) }) : undefined}
        />
        <UsageTile
          icon={<HardDrive />}
          label={t("dashboard.diskFree")}
          percent={
            host?.Ready && host.DiskTotalBytes > 0
              ? ((host.DiskTotalBytes - host.DiskFreeBytes) / host.DiskTotalBytes) * 100
              : undefined
          }
          valueText={host?.Ready ? formatBytes(String(host.DiskFreeBytes)) : "…"}
          sub={host?.Ready ? t("dashboard.ofTotal", { total: formatBytes(String(host.DiskTotalBytes)) }) : undefined}
          warnAbove={85}
        />
      </Section>
    </div>
  );
}

function storageBackendLabel(storage: StorageStatsResponse, t: TFunction): string {
  const backends = storage.Backends ?? [];
  if (backends.length === 1) return t("dashboard.singleBackend", { backend: backends[0].Backend });
  if (backends.length > 1) return t("dashboard.multipleBackends", { count: backends.length });
  return t("dashboard.noMediaObjects");
}

function Section({ title, hint, action, children }: { title: string; hint?: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div className="dashboard-section">
      <div className="dashboard-section-title">
        {title}
        {hint && <span>{hint}</span>}
        {action && <div className="dashboard-section-action">{action}</div>}
      </div>
      <div className="dashboard-grid">{children}</div>
    </div>
  );
}

type Tone = "neutral" | "good" | "warn" | "danger";

function StatTile({
  icon,
  label,
  value,
  sub,
  tone = "neutral",
  href,
  navigate
}: {
  icon: ReactNode;
  label: string;
  value: string;
  sub?: string;
  tone?: Tone;
  href?: string;
  navigate?: Navigate;
}) {
  const toneClass = tone === "neutral" ? "" : ` ${tone}`;
  const body = (
    <>
      <div className="stat-tile-head">
        <span className="stat-tile-icon">{icon}</span>
        {tone === "warn" && <AlertTriangle size={15} className="stat-tile-open" />}
      </div>
      <div className="stat-tile-value">{value}</div>
      <div className="stat-tile-label">{label}</div>
      {sub && <div className="stat-tile-sub">{sub}</div>}
    </>
  );
  if (href && navigate) {
    return (
      <a
        className={`stat-tile clickable${toneClass}`}
        href={href}
        onClick={(event) => {
          event.preventDefault();
          navigate(href);
        }}
      >
        {body}
      </a>
    );
  }
  return <div className={`stat-tile${toneClass}`}>{body}</div>;
}

function UsageTile({
  icon,
  label,
  percent,
  valueText,
  sub,
  warnAbove = 90
}: {
  icon: ReactNode;
  label: string;
  percent?: number;
  valueText: string;
  sub?: string;
  warnAbove?: number;
}) {
  const clamped = percent === undefined ? 0 : Math.max(0, Math.min(100, percent));
  const tone: Tone = percent === undefined ? "neutral" : percent >= warnAbove ? "danger" : percent >= warnAbove - 15 ? "warn" : "neutral";
  const toneClass = tone === "neutral" ? "" : ` ${tone}`;
  return (
    <div className={`stat-tile${toneClass}`}>
      <div className="stat-tile-head">
        <span className="stat-tile-icon">{icon}</span>
      </div>
      <div className="stat-tile-value">{valueText}</div>
      <div className="stat-tile-label">{label}</div>
      {sub && <div className="stat-tile-sub">{sub}</div>}
      <div className="stat-tile-bar">
        <span style={{ width: `${clamped}%` }} />
      </div>
    </div>
  );
}
