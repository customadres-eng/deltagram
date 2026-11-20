import { ArrowLeft, ChevronRight, Loader2, Smartphone, UserSearch, Users } from "lucide-react";
import { useState } from "react";
import { api, errorMessage } from "../api";
import { Avatar } from "../components/Avatar";
import { Alert, Badge, EmptyRow, Metric, PageFrame, SectionHead } from "../components/ui";
import { useI18n } from "../i18n";
import { displayName, displayPhone, displayUsername, formatDate } from "../lib/format";
import type { Navigate } from "../routing";
import type { TwinAccount, TwinReport } from "../types";

// TwinCheckPage runs the whole multi-account scan: every account that shares an
// IP with another account, and every device fingerprint used from more than one
// IP. Both are leads rather than verdicts -- IPs collide behind NAT/CGNAT and
// device strings are reported by the client.
export function TwinCheckPage({ navigate }: { navigate: Navigate }) {
  const { t } = useI18n();
  const [report, setReport] = useState<TwinReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function scan() {
    setBusy(true);
    setError("");
    try {
      setReport(await api.twins());
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const ipGroups = report?.IPGroups ?? [];
  const deviceGroups = report?.DeviceGroups ?? [];
  const ipAccounts = ipGroups.reduce((sum, group) => sum + group.Accounts.length, 0);
  const deviceAccounts = deviceGroups.reduce((sum, group) => sum + group.Accounts.length, 0);

  return (
    <PageFrame
      title={t("twins.title")}
      eyebrow={t("twins.eyebrow")}
      actions={
        <>
          <button className="btn icon-text" type="button" onClick={() => navigate("/accounts")}>
            <ArrowLeft size={15} /> {t("twins.back")}
          </button>
          <button className="btn primary icon-text" type="button" onClick={() => scan()} disabled={busy}>
            {busy ? <Loader2 size={15} className="spin" /> : <UserSearch size={15} />} {t("twins.scan")}
          </button>
        </>
      }
    >
      {error && <Alert>{error}</Alert>}
      <p className="about-text">{t("twins.intro")}</p>

      {!report && !busy && (
        <div className="table-wrap">
          <table className="data-table">
            <tbody>
              <tr>
                <td className="empty-cell">{t("twins.idle")}</td>
              </tr>
            </tbody>
          </table>
        </div>
      )}

      {report && (
        <>
          <div className="metric-row">
            <Metric label={t("twins.scanned")} value={String(report.ScannedAccounts)} />
            <Metric label={t("twins.ipGroups")} value={String(ipGroups.length)} tone="warn" />
            <Metric label={t("twins.ipAccounts")} value={String(ipAccounts)} tone="warn" />
            <Metric label={t("twins.deviceGroups")} value={String(deviceGroups.length)} tone="danger" />
            <Metric label={t("twins.deviceAccounts")} value={String(deviceAccounts)} tone="danger" />
          </div>

          <section className="section-block">
            <SectionHead
              title={t("twins.ipSection")}
              text={t("twins.ipSectionText")}
              action={<Badge tone="warn"><Users size={12} /> {t("twins.groups", { count: ipGroups.length })}</Badge>}
            />
            {report.IPGroupsTruncated && <Alert>{t("twins.truncated")}</Alert>}
            <div className="stacked-sections">
              {ipGroups.map((group) => (
                <section className="section-block" key={`ip-${group.IP}`}>
                  <SectionHead
                    title={group.IP}
                    text={t("twins.ipGroupText", { count: group.Accounts.length, date: formatDate(group.LastActiveAt) })}
                    action={<Badge tone="warn">{t("twins.accounts", { count: group.AccountCount })}</Badge>}
                  />
                  <AccountTable rows={group.Accounts} navigate={navigate} showDevice />
                </section>
              ))}
              {ipGroups.length === 0 && <EmptyTable colSpan={9} />}
            </div>
          </section>

          <section className="section-block">
            <SectionHead
              title={t("twins.deviceSection")}
              text={t("twins.deviceSectionText")}
              action={<Badge tone="danger"><Smartphone size={12} /> {t("twins.groups", { count: deviceGroups.length })}</Badge>}
            />
            {report.DeviceGroupsTruncated && <Alert>{t("twins.truncated")}</Alert>}
            <div className="stacked-sections">
              {deviceGroups.map((group) => (
                <section
                  className="section-block"
                  key={`device-${group.DeviceModel}|${group.SystemVersion}|${group.Platform}`}
                >
                  <SectionHead
                    title={group.DeviceModel || t("twins.unknownDevice")}
                    text={t("twins.deviceGroupText", {
                      system: `${group.Platform} ${group.SystemVersion}`.trim(),
                      ips: group.IPCount,
                      date: formatDate(group.LastActiveAt)
                    })}
                    action={<Badge tone="danger">{t("twins.accounts", { count: group.AccountCount })}</Badge>}
                  />
                  <AccountTable rows={group.Accounts} navigate={navigate} showDevice />
                </section>
              ))}
              {deviceGroups.length === 0 && <EmptyTable colSpan={9} />}
            </div>
          </section>
        </>
      )}
    </PageFrame>
  );
}

function AccountTable({
  rows,
  navigate,
  showDevice
}: {
  rows: TwinAccount[];
  navigate: Navigate;
  showDevice: boolean;
}) {
  const { t } = useI18n();
  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th className="avatar-col"></th>
            <th>{t("twins.userID")}</th>
            <th>{t("twins.phone")}</th>
            <th>{t("twins.username")}</th>
            <th>{t("twins.name")}</th>
            <th>{t("twins.ip")}</th>
            {showDevice && <th>{t("twins.device")}</th>}
            <th>{t("twins.lastActive")}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((account) => (
            <tr key={`${account.UserID}-${account.IP}`}>
              <td className="avatar-col">
                <button
                  className="avatar-link"
                  type="button"
                  onClick={() => navigate(`/accounts/${account.UserID}`)}
                  aria-label={t("twins.openAccount", { id: account.UserID })}
                >
                  <Avatar id={account.UserID} kind="user" label={displayName(account)} />
                </button>
              </td>
              <td className="mono">{account.UserID}</td>
              <td>{displayPhone(account.Phone)}</td>
              <td>{displayUsername(account.Username) || account.Username || "-"}</td>
              <td>{displayName(account) || "-"}</td>
              <td className="mono">{account.IP || "-"}</td>
              {showDevice && (
                <td>{[account.DeviceModel, account.Platform, account.SystemVersion].filter(Boolean).join(" · ") || "-"}</td>
              )}
              <td>{formatDate(account.ActiveAt)}</td>
              <td>
                <button className="row-link" type="button" onClick={() => navigate(`/accounts/${account.UserID}`)}>
                  {t("twins.detail")} <ChevronRight size={14} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EmptyTable({ colSpan }: { colSpan: number }) {
  return (
    <div className="table-wrap">
      <table className="data-table">
        <tbody>
          <EmptyRow colSpan={colSpan} />
        </tbody>
      </table>
    </div>
  );
}
