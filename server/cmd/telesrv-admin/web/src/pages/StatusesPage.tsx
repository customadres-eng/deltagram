import { Tag } from "lucide-react";
import { useEffect, useState } from "react";
import { api, errorMessage } from "../api";
import { ActionButton } from "../components/ActionButton";
import { Alert, PageFrame } from "../components/ui";

export function StatusesPage() {
  const [userID, setUserID] = useState("");
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    const id = Number(userID);
    if (!Number.isSafeInteger(id) || id <= 0) return;
    let cancelled = false;
    setLoading(true);
    api.account(id).then((account) => {
      if (!cancelled) { setStatus(account.AdminTextStatus ?? ""); setError(""); }
    }).catch((err) => { if (!cancelled) setError(errorMessage(err)); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [userID]);
  return <PageFrame title="Статуси" eyebrow="Адміністрування аккаунтів">
    {error && <Alert>{error}</Alert>}
    <section className="section-block">
      <div className="card-body">
        <label className="duration-field"><span>ID аккаунта</span><input value={userID} onChange={e => setUserID(e.target.value)} inputMode="numeric" /></label>
        {loading && <p>Завантаження статусу…</p>}
        <label className="duration-field"><span>Статус</span><input value={status} onChange={e => setStatus(e.target.value)} maxLength={64} placeholder="support" /></label>
        <ActionButton label="Зберегти статус" icon={<Tag size={15} />} tone="neutral" path="/api/actions/set-account-text-status" payload={() => ({ user_id: Number(userID), status: status.trim() })} onDone={() => undefined} disabled={!Number.isSafeInteger(Number(userID)) || Number(userID) <= 0} />
      </div>
    </section>
  </PageFrame>;
}
