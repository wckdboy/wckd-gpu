import { useRef, useState } from "react";

import { phaseActive, type Session } from "../control/types";
import { useApp } from "../state/appState";
import { money, phaseLabel, uiEndpoint } from "../ui/format";
import { sampleSession } from "../ui/sample";
import { useCostTicker } from "../ui/useCostTicker";
import { useCountdown } from "../ui/useCountdown";

export function SessionScreen() {
  const app = useApp();
  const [preview, setPreview] = useState<Session | null>(null);
  const session = app.session ?? preview;
  const sample = !app.session && preview !== null;
  const stopRef = useRef<HTMLDialogElement>(null);
  const forceRef = useRef<HTMLDialogElement>(null);
  const extendRef = useRef<HTMLDialogElement>(null);
  const [phrase, setPhrase] = useState("");
  const active = session ? phaseActive(session.phase) : false;
  const countdown = useCountdown(active ? session?.deadline_at : undefined);
  const accrued = useCostTicker(session?.offer.usd_per_hr ?? 0, session?.created_at, active);
  const openUrl = session ? uiEndpoint(session) : undefined;
  const drain = app.drainFailed || /drain did not finish/i.test(session?.error ?? "");

  async function stop(force: boolean) {
    if (sample) {
      setPhrase("");
      app.setNotice("Sample layout. Desktop required to stop a pod.");
      return;
    }
    const next = await app.run(() => app.client.stop({ sessionId: session?.id, force }));
    if (!next) {
      return;
    }
    app.setSession(next);
    app.setDrainFailed(false);
    setPhrase("");
    app.setNotice(force ? "Pod terminated with --force. drain_ok is false." : "Session stopped.");
  }

  return (
    <section className="screen">
      <div className="screen-head">
        <span className="tag">Session</span>
        <h1>Live</h1>
        <p className="lede">Phase, hard-stop countdown, and the catalog cost ticker.</p>
      </div>

      {!session ? (
        <div className="empty">
          <p className="mono empty-title">no session</p>
          <p className="muted">
            {app.client.mode === "cli"
              ? "Status uses the current session remembered by the CLI."
              : "Desktop required for live status. The PWA cannot shell out to wckd."}
          </p>
          {app.client.mode === "cli" ? (
            <button
              type="button"
              className="btn"
              disabled={app.busy}
              onClick={() =>
                void app.run(async () => {
                  app.setSession(await app.client.status());
                })
              }
            >
              Refresh
            </button>
          ) : (
            <button
              type="button"
              className="btn"
              onClick={() => {
                app.setError("");
                setPreview(sampleSession(app.draft.hours));
              }}
            >
              Preview live layout
            </button>
          )}
        </div>
      ) : (
        <>
          {sample ? (
            <div className="banner warn" role="status">
              <span className="sample-flag">SAMPLE</span> — deadline is inside T−15 so the amber flash
              is visible. Not a live pod.
            </div>
          ) : null}
          <article className="card">
            <div className="live-metrics">
              <span className={`phase phase-${session.phase}`}>{phaseLabel(session.phase)}</span>
              <span className={countdownClass(countdown)}>
                {countdown ? (countdown.elapsed ? "Deadline elapsed" : countdown.label) : "No deadline"}
              </span>
              <div className="ticker-block">
                <span className="tag">{active ? "Catalog ticker" : "Estimate"}</span>
                <strong className="ticker">{money(active ? accrued : session.cost_estimate_usd)}</strong>
              </div>
            </div>
            <dl className="facts">
              <div>
                <dt>Session</dt>
                <dd className="mono">{session.id}</dd>
              </div>
              <div>
                <dt>Project</dt>
                <dd>{session.project_id}</dd>
              </div>
              <div>
                <dt>Preset</dt>
                <dd>{session.preset_id}</dd>
              </div>
              <div>
                <dt>Estimate</dt>
                <dd className="mono">{money(session.cost_estimate_usd)}</dd>
              </div>
              <div>
                <dt>Actual</dt>
                <dd className="mono">
                  {session.cost_actual_usd === undefined ? "—" : money(session.cost_actual_usd)}
                </dd>
              </div>
              <div>
                <dt>Pod</dt>
                <dd className="mono">{session.instance_id || "—"}</dd>
              </div>
              <div>
                <dt>Pod status</dt>
                <dd>{session.pod_status || "—"}</dd>
              </div>
              <div>
                <dt>Drain</dt>
                <dd>{session.phase === "terminated" ? (session.drain_ok ? "ok" : "not confirmed") : "—"}</dd>
              </div>
            </dl>
            {session.offer.name ? (
              <p className="muted">
                {session.offer.vendor} · {session.offer.sku} · {session.offer.vram_gb} GB ·{" "}
                <span className={session.offer.cloud === "secure" ? "tier-secure" : undefined}>
                  {session.offer.cloud}
                </span>{" "}
                · <span className="mono">{money(session.offer.usd_per_hr)}/hr</span>
              </p>
            ) : null}
            {session.error ? <p className="late">{session.error}</p> : null}
            {session.warning ? <p className="warn-text">{session.warning}</p> : null}
            {session.forced ? <p className="late">Terminated with --force before drain completed.</p> : null}
            <div className="actions">
              <button
                type="button"
                className="btn"
                disabled={app.busy || sample}
                onClick={() => void app.refreshSession()}
              >
                Refresh
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
              {active ? (
                <button type="button" className="btn" onClick={() => extendRef.current?.showModal()}>
                  Extend
                </button>
              ) : null}
              {active ? (
                <button type="button" className="btn danger" onClick={() => stopRef.current?.showModal()}>
                  Stop
                </button>
              ) : null}
            </div>
          </article>

          {drain && !sample ? (
            <article className="card warn-card">
              <h2>Drain failed</h2>
              <p>
                The pod is still running. Retry Stop to wait for the S3 drain again. Force skip drain
                only if you accept data loss risk. The CLI form is{" "}
                <code>wckd stop {session.id} --force</code>.
              </p>
              <div className="actions">
                <button type="button" className="btn" onClick={() => stopRef.current?.showModal()}>
                  Retry stop
                </button>
                <button type="button" className="btn danger" onClick={() => forceRef.current?.showModal()}>
                  Force…
                </button>
              </div>
            </article>
          ) : null}
        </>
      )}

      <dialog ref={stopRef} className="modal">
        <form method="dialog" className="sheet">
          <span className="tag">Stop</span>
          <h2>Drain, then terminate</h2>
          <p>Stop runs drain → S3 sync → terminate. Force skip drain only if you accept data loss risk.</p>
          <div className="actions">
            <button type="submit" className="btn" value="cancel">
              Cancel
            </button>
            <button type="submit" className="btn danger fill" value="stop" onClick={() => void stop(false)}>
              Stop
            </button>
          </div>
        </form>
      </dialog>

      <dialog ref={forceRef} className="modal">
        <form
          className="sheet"
          onSubmit={(event) => {
            event.preventDefault();
            if (phrase !== "DESTROY") {
              return;
            }
            forceRef.current?.close();
            void stop(true);
          }}
        >
          <span className="tag">Force</span>
          <h2>Skip drain</h2>
          <p>
            Stop runs drain → S3 sync → terminate. Force skip drain only if you accept data loss
            risk. Type <code>DESTROY</code> to delete the pod anyway.
          </p>
          <input
            value={phrase}
            onChange={(event) => setPhrase(event.target.value)}
            autoComplete="off"
            spellCheck={false}
            aria-label="Type DESTROY to confirm"
          />
          <div className="actions">
            <button
              type="button"
              className="btn"
              onClick={() => {
                setPhrase("");
                forceRef.current?.close();
              }}
            >
              Cancel
            </button>
            <button type="submit" className="btn danger fill" disabled={phrase !== "DESTROY" || app.busy}>
              Force stop
            </button>
          </div>
        </form>
      </dialog>

      <dialog ref={extendRef} className="modal">
        <form method="dialog" className="sheet">
          <span className="tag">Extend</span>
          <h2>Not in P1</h2>
          <p>
            The CLI has no extend command. Stop this session, then start again.{" "}
            <code>wckd session extend</code> stays deferred.
          </p>
          <div className="actions">
            <button type="submit" className="btn">
              Close
            </button>
          </div>
        </form>
      </dialog>
    </section>
  );
}

function countdownClass(countdown: { elapsed: boolean; warn: boolean } | null): string {
  if (!countdown) {
    return "countdown";
  }
  if (countdown.elapsed) {
    return "countdown late";
  }
  if (countdown.warn) {
    return "countdown is-warn";
  }
  return "countdown";
}
