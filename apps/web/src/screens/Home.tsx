import { phaseActive } from "../control/types";
import { prefixFor, useApp } from "../state/appState";
import { money, phaseLabel, uiEndpoint } from "../ui/format";
import { useCountdown } from "../ui/useCountdown";

export function Home() {
  const app = useApp();
  const session = app.session;
  const active = session ? phaseActive(session.phase) : false;
  const countdown = useCountdown(active ? session?.deadline_at : undefined);
  const openUrl = session ? uiEndpoint(session) : undefined;

  return (
    <section className="screen">
      <div className="screen-head">
        <h1>Home</h1>
        <p className="lede">
          Pick a project, a preset, and hours. The estimate is shown before anything is rented.
        </p>
      </div>

      {app.client.mode === "http" ? (
        <div className="banner warn">
          GPU start, status, and stop run in the desktop app. This PWA is the same UI and keeps
          projects and presets on this device. Secrets stay in the CLI <code>.env</code>, not in
          the browser.
        </div>
      ) : null}

      {session ? (
        <article className="card session-card">
          <div className="card-row">
            <span className={`phase phase-${session.phase}`}>{phaseLabel(session.phase)}</span>
            <strong className="mono">{session.id}</strong>
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
              <dd>{money(session.cost_estimate_usd)}</dd>
            </div>
            <div>
              <dt>Hard stop</dt>
              <dd className={countdown?.elapsed ? "late" : undefined}>
                {countdown ? (countdown.elapsed ? "Deadline elapsed" : countdown.label) : "—"}
              </dd>
            </div>
          </dl>
          {session.offer.name ? (
            <p className="muted">
              {session.offer.name} · {session.offer.vram_gb} GB · {session.offer.cloud} ·{" "}
              {money(session.offer.usd_per_hr)}/hr
            </p>
          ) : null}
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
        <article className="card">
          <h2>No active session</h2>
          <p className="muted">
            {app.projects.length} project{app.projects.length === 1 ? "" : "s"} · {app.presets.length}{" "}
            preset{app.presets.length === 1 ? "" : "s"}
            {app.draft.projectId ? ` · prefix ${prefixFor(app.draft.projectId)}` : ""}
          </p>
        </article>
      )}

      <div className="start-block">
        <button type="button" className="btn primary xl" onClick={() => app.setView("offers")}>
          Start
        </button>
        {active ? (
          <p className="muted">A session is already running. Start rents another GPU.</p>
        ) : (
          <p className="muted">Opens ranked offers. Nothing is provisioned until you confirm.</p>
        )}
      </div>
    </section>
  );
}
