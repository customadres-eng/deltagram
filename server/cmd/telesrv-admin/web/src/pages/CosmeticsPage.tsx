import { ChevronRight, Loader2, Palette, RefreshCw, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { api, errorMessage } from "../api";
import { FrameAction } from "../components/attributes";
import { Alert, EmptyRow, LoadingSurface, Metric, PageFrame, QueryPanel } from "../components/ui";
import { useI18n } from "../i18n";
import { displayUsername } from "../lib/format";
import type { Navigate } from "../routing";
import type { AccountRow, FramedAccountRow } from "../types";

// CosmeticsPage is the discoverable home of DeltaGram avatar frames. The frame
// editor also exists on the account detail page, but operators were not finding
// it there, so this page lists every framed account and lets them grant a frame
// to any account by id or username without opening the detail view.
export function CosmeticsPage({ navigate }: { navigate: Navigate }) {
  const { t } = useI18n();
  const [rows, setRows] = useState<FramedAccountRow[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [found, setFound] = useState<AccountRow[]>([]);
  const [searching, setSearching] = useState(false);

  async function load() {
    setBusy(true);
    setError("");
    try {
      const result = await api.cosmeticsFrames();
      setRows(result.rows ?? []);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function search(event: React.FormEvent) {
    event.preventDefault();
    const wanted = query.trim();
    if (!wanted) {
      setFound([]);
      return;
    }
    setSearching(true);
    setError("");
    try {
      const result = await api.accounts(new URLSearchParams({ q: wanted, limit: "20" }));
      setFound(result.rows ?? []);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSearching(false);
    }
  }

  return (
    <PageFrame
      title={t("cosmetics.title")}
      eyebrow={t("cosmetics.eyebrow")}
      actions={
        <button className="btn icon-text" type="button" onClick={() => void load()} disabled={busy}>
          <RefreshCw size={15} className={busy ? "spin" : ""} /> {t("common.refresh")}
        </button>
      }
    >
      {error && <Alert>{error}</Alert>}

      <div className="metric-row">
        <Metric label={t("cosmetics.withFrame")} value={String(rows.length)} tone="good" />
        <Metric label={t("cosmetics.frames")} value="8" />
      </div>

      <QueryPanel>
        <form className="toolbar" onSubmit={(event) => void search(event)}>
          <label className="searchbox">
            <Search size={15} />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t("cosmetics.searchPlaceholder")}
            />
          </label>
          <button className="btn primary icon-text" type="submit" disabled={searching}>
            {searching ? <Loader2 size={15} className="spin" /> : <Search size={15} />} {t("common.search")}
          </button>
        </form>
      </QueryPanel>

      {found.length > 0 && (
        <>
          <div className="section-head">
            <h3>{t("cosmetics.searchResults")}</h3>
          </div>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>{t("common.id")}</th>
                  <th>{t("common.username")}</th>
                  <th>{t("common.name")}</th>
                  <th>{t("attr.avatarFrame")}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {found.map((row) => (
                  <tr key={row.ID}>
                    <td className="mono">{row.ID}</td>
                    <td>{displayUsername(row.Username) || "-"}</td>
                    <td>{[row.FirstName, row.LastName].filter(Boolean).join(" ") || "-"}</td>
                    <td>
                      <FrameAction id={row.ID} current={row.AvatarFrame || ""} onDone={() => void load()} />
                    </td>
                    <td>
                      <button className="row-link" type="button" onClick={() => navigate(`/accounts/${row.ID}`)}>
                        <ChevronRight size={14} /> {t("common.detail")}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      <div className="section-head">
        <h3>
          <Palette size={15} /> {t("cosmetics.framedAccounts")}
        </h3>
      </div>
      {busy && rows.length === 0 ? (
        <LoadingSurface label={t("common.loading")} />
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>{t("common.id")}</th>
                <th>{t("common.username")}</th>
                <th>{t("common.name")}</th>
                <th>{t("attr.avatarFrame")}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <EmptyRow colSpan={5} />
              ) : (
                rows.map((row) => (
                  <tr key={row.ID}>
                    <td className="mono">{row.ID}</td>
                    <td>{displayUsername(row.Username) || "-"}</td>
                    <td>{[row.FirstName, row.LastName].filter(Boolean).join(" ") || "-"}</td>
                    <td>
                      <FrameAction id={row.ID} current={row.AvatarFrame} onDone={() => void load()} />
                    </td>
                    <td>
                      <button className="row-link" type="button" onClick={() => navigate(`/accounts/${row.ID}`)}>
                        <ChevronRight size={14} /> {t("common.detail")}
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </PageFrame>
  );
}
