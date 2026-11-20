import { RefreshCw, TrendingUp } from "lucide-react";
import { useEffect, useState } from "react";
import { api, errorMessage } from "../api";
import { ActionButton } from "../components/ActionButton";
import { Alert, EmptyRow, Metric, PageFrame } from "../components/ui";
import { useI18n } from "../i18n";
import type { CasinoStatsResponse, CasinoUserStat } from "../types";

function nf(n: number | null | undefined): string {
  return Number(n ?? 0).toLocaleString("ru-RU");
}

function pct(n: number | null | undefined, digits = 1): string {
  return Number(n ?? 0).toFixed(digits).replace(".", ",") + "%";
}

function userName(u: { username: string; first_name: string; user_id: number }): string {
  if (u.username) return "@" + u.username;
  if (u.first_name) return u.first_name + " (" + u.user_id + ")";
  return String(u.user_id);
}

// Leaderboard table shared by the four "top" lists. The column set is the same
// so the operator can compare lists at a glance.
function UserTable({ title, rows, cps }: { title: string; rows: CasinoUserStat[]; cps: number }) {
  const stars = (chips: number) => Math.round((chips / cps) * 100) / 100;
  return (
    <div className="card" style={{ marginTop: 16 }}>
      <div className="card-head"><h3>{title}</h3></div>
      <div className="table-wrap">
        <table className="data-table">
          <thead>
            <tr>
              <th>Юзер</th>
              <th>Депозиты ⭐</th>
              <th>Выводы ⭐</th>
              <th>Оборот ⭐</th>
              <th>Выигрыш ⭐</th>
              <th>Профит ⭐</th>
              <th>Баланс ⭐</th>
              <th>Раунды</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((u) => (
              <tr key={u.user_id}>
                <td>{userName(u)}</td>
                <td className="mono">{nf(stars(u.deposits))}</td>
                <td className="mono">{nf(stars(u.withdrawals))}</td>
                <td className="mono">{nf(stars(u.bets))}</td>
                <td className="mono">{nf(stars(u.wins))}</td>
                <td className={"mono " + (u.net >= 0 ? "good" : "bad")}>{nf(stars(u.net))}</td>
                <td className="mono">{nf(stars(u.balance))}</td>
                <td className="mono">{nf(u.rounds)}</td>
              </tr>
            ))}
            {rows.length === 0 && <EmptyRow colSpan={8} />}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function CasinoPage() {
  const { t } = useI18n();
  const [stats, setStats] = useState<CasinoStatsResponse | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [payout, setPayout] = useState("100");

  async function refresh() {
    setBusy(true);
    setError("");
    try {
      const next = await api.casinoStats();
      setStats(next);
      setPayout(String(next.settings.payout_percent));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  const s = stats;
  const cps = s?.chips_per_star || 100;
  const stars = (chips: number | null | undefined) => Math.round((Number(chips ?? 0) / cps) * 100) / 100;
  const payoutNum = Number(payout);
  const previewRTP = Number.isFinite(payoutNum) ? 95 * payoutNum / 100 : 95;

  return (
    <PageFrame
      title="Казино"
      eyebrow={t("route.casinoSubtitle")}
      actions={
        <button className="btn icon-text" type="button" onClick={() => void refresh()} disabled={busy}>
          <RefreshCw size={15} className={busy ? "spin" : ""} /> {t("common.refresh")}
        </button>
      }
    >
      {error && <Alert>{error}</Alert>}

      <div className="card">
        <div className="card-head"><h3>Экономика и щедрость</h3></div>
        <Alert>
          Множитель выплат применяется к каждому выигрышу и вступает в силу со следующего раунда.
          100% — штатная отдача 95%, 0% — выигрыши не выплачиваются вообще, 200% — удвоение
          каждого выигрыша. Игроки видят фактическую отдачу в приложении, она пересчитывается
          автоматически.
        </Alert>
        <div className="metric-row">
          <Metric label="Текущий множитель" value={pct(s?.settings.payout_percent ?? 100, 0)} />
          <Metric label="Фактическая отдача" value={pct(s?.settings.rtp_percent ?? 95)} />
          <Metric label="Изменено" value={s?.settings.updated_by ? s.settings.updated_by : "—"} />
          <Metric label="Когда" value={s?.settings.updated_at ? new Date(s.settings.updated_at).toLocaleString("ru-RU") : "—"} />
        </div>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center", marginTop: 12 }}>
          <label className="duration-field">
            <span>Множитель выплат, %</span>
            <input
              aria-label="payout percent"
              type="number"
              min="0"
              max="500"
              value={payout}
              onChange={(event) => setPayout(event.target.value)}
            />
          </label>
          {[0, 25, 50, 75, 100, 125, 150, 200].map((preset) => (
            <button key={preset} className="btn ghost compact-btn" type="button" onClick={() => setPayout(String(preset))}>
              {preset}%
            </button>
          ))}
          <ActionButton
            label={"Применить (" + (Number.isFinite(previewRTP) ? previewRTP.toFixed(1).replace(".", ",") : "95,0") + "% отдача)"}
            icon={<TrendingUp size={15} />}
            tone="warn"
            path="/api/actions/set-casino-payout"
            payload={() => ({ payout_percent: Number(payout) })}
            disabled={!payout.trim() || !Number.isInteger(payoutNum) || payoutNum < 0 || payoutNum > 500}
            onDone={refresh}
          />
        </div>
        <div className="muted" style={{ marginTop: 8, fontSize: 12 }}>
          Предпросмотр: при множителе {Number.isFinite(payoutNum) ? payoutNum : 100}% отдача составит{" "}
          {previewRTP.toFixed(1).replace(".", ",")}%.
        </div>
      </div>

      <div className="metric-row" style={{ marginTop: 16 }}>
        <Metric label="Игроков" value={nf(s?.totals.players)} />
        <Metric label="Раундов" value={nf(s?.totals.rounds)} />
        <Metric label="Активных за 24ч" value={nf(s?.totals.active_last_24h)} />
        <Metric label="Отдача факт." value={pct(s?.totals.rtp_percent)} />
      </div>
      <div className="metric-row">
        <Metric label="Депозиты ⭐" value={nf(s?.totals.stars_deposited)} tone="good" />
        <Metric label="Выводы ⭐" value={nf(s?.totals.stars_withdrawn)} />
        <Metric label="Оборот (фишки)" value={nf(s?.totals.bets_chips)} />
        <Metric label="Выплачено (фишки)" value={nf(s?.totals.wins_chips)} />
        <Metric label="Профит казино (фишки)" value={nf(s?.totals.house_chips)} tone={Number(s?.totals.house_chips ?? 0) >= 0 ? "good" : "danger"} />
        <Metric label="Обязательства (фишки)" value={nf(s?.totals.chips_liability)} />
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-head"><h3>По играм</h3></div>
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Игра</th>
                <th>Раундов</th>
                <th>Побед</th>
                <th>Оборот (фишки)</th>
                <th>Выплаты (фишки)</th>
                <th>Отдача</th>
                <th>Макс. выигрыш (фишки)</th>
              </tr>
            </thead>
            <tbody>
              {(s?.games ?? []).map((g) => (
                <tr key={g.game}>
                  <td>{g.game}</td>
                  <td className="mono">{nf(g.rounds)}</td>
                  <td className="mono">{nf(g.won)}</td>
                  <td className="mono">{nf(g.stake)}</td>
                  <td className="mono">{nf(g.payout)}</td>
                  <td className="mono">{pct(g.rtp)}</td>
                  <td className="mono">{nf(g.max_payout)}</td>
                </tr>
              ))}
              {(s?.games ?? []).length === 0 && <EmptyRow colSpan={7} />}
            </tbody>
          </table>
        </div>
      </div>

      <UserTable title="Кто больше всех занёс (депозиты)" rows={s?.top_depositors ?? []} cps={cps} />
      <UserTable title="Кто больше всех выиграл (сумма выигрышей)" rows={s?.top_winners ?? []} cps={cps} />
      <UserTable title="Кто больше всех проиграл" rows={s?.top_losers ?? []} cps={cps} />
      <UserTable title="Кто больше всех накрутил оборота" rows={s?.top_bettors ?? []} cps={cps} />

      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-head"><h3>Самые крупные выигрыши</h3></div>
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Раунд</th>
                <th>Юзер</th>
                <th>Игра</th>
                <th>Ставка ⭐</th>
                <th>Выигрыш ⭐</th>
                <th>×</th>
                <th>Когда</th>
              </tr>
            </thead>
            <tbody>
              {(s?.big_wins ?? []).map((w) => (
                <tr key={w.round_id}>
                  <td className="mono">#{w.round_id}</td>
                  <td>{userName({ username: w.username, first_name: w.first_name, user_id: w.user_id })}</td>
                  <td>{w.game}</td>
                  <td className="mono">{nf(stars(w.stake))}</td>
                  <td className="mono">{nf(stars(w.payout))}</td>
                  <td className="mono">{Number(w.multiplier ?? 0).toFixed(2).replace(".", ",")}</td>
                  <td className="mono">{new Date(w.at).toLocaleString("ru-RU")}</td>
                </tr>
              ))}
              {(s?.big_wins ?? []).length === 0 && <EmptyRow colSpan={7} />}
            </tbody>
          </table>
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-head"><h3>Активность по дням (30 дней)</h3></div>
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>День</th>
                <th>Депозиты (фишки)</th>
                <th>Выводы (фишки)</th>
                <th>Ставки (фишки)</th>
                <th>Выплаты (фишки)</th>
                <th>Профит (фишки)</th>
              </tr>
            </thead>
            <tbody>
              {(s?.daily ?? []).map((d) => (
                <tr key={d.day}>
                  <td className="mono">{d.day}</td>
                  <td className="mono">{nf(d.deposits)}</td>
                  <td className="mono">{nf(d.withdrawals)}</td>
                  <td className="mono">{nf(d.bets)}</td>
                  <td className="mono">{nf(d.wins)}</td>
                  <td className={"mono " + (d.bets - d.wins >= 0 ? "good" : "bad")}>{nf(d.bets - d.wins)}</td>
                </tr>
              ))}
              {(s?.daily ?? []).length === 0 && <EmptyRow colSpan={6} />}
            </tbody>
          </table>
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-head"><h3>Все игроки (топ-100 по обороту)</h3></div>
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Юзер</th>
                <th>ID</th>
                <th>Депозиты ⭐</th>
                <th>Выводы ⭐</th>
                <th>Оборот ⭐</th>
                <th>Выигрыш ⭐</th>
                <th>Профит ⭐</th>
                <th>Баланс ⭐</th>
                <th>Раунды</th>
                <th>Последняя активность</th>
              </tr>
            </thead>
            <tbody>
              {(s?.users ?? []).slice(0, 100).map((u) => (
                <tr key={u.user_id}>
                  <td>{u.username ? "@" + u.username : u.first_name || "—"}</td>
                  <td className="mono">{u.user_id}</td>
                  <td className="mono">{nf(stars(u.deposits))}</td>
                  <td className="mono">{nf(stars(u.withdrawals))}</td>
                  <td className="mono">{nf(stars(u.bets))}</td>
                  <td className="mono">{nf(stars(u.wins))}</td>
                  <td className={"mono " + (u.net >= 0 ? "good" : "bad")}>{nf(stars(u.net))}</td>
                  <td className="mono">{nf(stars(u.balance))}</td>
                  <td className="mono">{nf(u.rounds)}</td>
                  <td className="mono">{u.last_action ? new Date(u.last_action).toLocaleString("ru-RU") : "—"}</td>
                </tr>
              ))}
              {(s?.users ?? []).length === 0 && <EmptyRow colSpan={10} />}
            </tbody>
          </table>
        </div>
      </div>
    </PageFrame>
  );
}
