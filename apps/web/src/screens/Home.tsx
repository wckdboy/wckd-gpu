import { phaseActive } from "../control/types";
import { estimateUsd } from "../control/parse";
import { prefixFor, useApp } from "../state/appState";
import { money, phaseLabel, uiEndpoint } from "../ui/format";
import { StartDock } from "../ui/StartDock";
import { useCountdown } from "../ui/useCountdown";
import { useSessionStart } from "../ui/useSessionStart";

export function Home() {
  const app = useApp();
  const session = app.session;
  const active = session ? phaseActive(session.phase) : false;
  const countdown = useCountdown(active ? session?.deadline_at : undefined);
  const openUrl = session ? uiEndpoint(session) : undefined;
  const { gate, confirmAnother, setConfirmAnother, start } = useSessionStart();
  const locked = app.lockedOffer;
  const estimate = locked ? estimateUsd(locked.usd_per_hr, app.draft.hours) : null;

  return (
    <section className="screen">
      <div className="screen-head">
        <span className="tag">Session</span>
        <h1>Home</h1>
        <p className="lede">Project, preset, and hours. The catalog estimate is on screen before Start.</p>
      </div>

      {app.client.mode === "http" ? (
        <div className="banner warn" role="status">
          Desktop required for Start. This PWA keeps projects and presets on device. Secrets stay in
          the CLI <code>.env</code>.
        </div>
      ) : null}

      {session ? (
        <article className="card">
          <div className="card-row">
            <span className={`phase phase-${session.phase}`}>{phaseLabel(session.phase)}</span>
            <strong className="mono">{session.id}</strong>
            {countdown ? (
              <span className={countdownClass(countdown.elapsed, countdown.warn)}>
                {countdown.elapsed ? "Deadline elapsed" : countdown.label}
              </span>
            ) : null}
          </div>
          <dl className="facts">
            <div>
              <dt>Project</dt>
              <dd>{session.project_id || "—"}</dd>
            </div>
            <div>
              <dt>Preset</dt>
              <dd>{session.preset_id || "—"}</dd>
            </div>
            <div>
              <dt>Estimate</dt>
              <dd className="mono">{money(session.cost_estimate_usd)}</dd>
            </div>
            <div>
              <dt>Offer</dt>
              <dd className="mono">{session.offer.sku || session.offer.name || "—"}</dd>
            </div>
          </dl>
          <div className="actions">
            <button type="button" className="btn" onClick={() => app.setView("session")}>
              Live session
            </button>
            {openUrl ? (
              <button
                type="button"
                className="btn"
                onClick={() => void app.run(() => app.client.openUrl(openUrl))}
              >
                Open UI
              </button>
            ) : null}
          </div>
        </article>
      ) : (
        <div className="empty">
          <p className="mono empty-title">no session</p>
          <p className="muted">
            {app.projects.length} project{app.projects.length === 1 ? "" : "s"} · {app.presets.length}{" "}
            preset{app.presets.length === 1 ? "" : "s"}
            {app.draft.projectId ? ` · ${prefixFor(app.draft.projectId)}` : ""}
          </p>
        </div>
      )}

      {confirmAnother ? (
        <SecondSession
          sessionId={app.session?.id ?? ""}
          phase={app.session?.phase ?? ""}
          onCancel={() => setConfirmAnother(false)}
        />
      ) : null}

      <StartDock
        estimate={estimate}
        rate={locked?.usd_per_hr ?? null}
        hours={app.draft.hours}
        detail={locked ? `${locked.vendor} · ${locked.sku}` : app.draft.presetId || undefined}
        disabled={!gate.ok}
        reason={gate.reason}
        busy={app.busy}
        onStart={() => void start(confirmAnother)}
        secondary={
          <button type="button" className="btn" onClick={() => app.setView("offers")}>
            Offers
          </button>
        }
      />
    </section>
  );
}

function countdownClass(elapsed: boolean, warn: boolean): string {
  if (elapsed) {
    return "countdown late";
  }
  if (warn) {
    return "countdown is-warn";
  }
  return "countdown";
}

function SecondSession({
  sessionId,
  phase,
  onCancel,
}: {
  sessionId: string;
  phase: string;
  onCancel: () => void;
}) {
  return (
    <div className="card warn-card">
      <h2>Rent another GPU?</h2>
      <p>
        {sessionId} is still {phase}. Starting now provisions a second pod and a second bill.
      </p>
      <div className="actions">
        <button type="button" className="btn" onClick={onCancel}>
          Cancel
        </button>
        <p className="tiny">Press Start again to rent the second pod.</p>
      </div>
    </div>
  );
}
