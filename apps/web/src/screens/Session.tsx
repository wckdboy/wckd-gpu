import { useRef, useState } from "react";

import { phaseActive } from "../control/types";
import { useApp } from "../state/appState";
import { money, phaseLabel, uiEndpoint } from "../ui/format";
import { useCountdown } from "../ui/useCountdown";

export function SessionScreen() {
  const app = useApp();
  const session = app.session;
  const stopRef = useRef<HTMLDialogElement>(null);
  const forceRef = useRef<HTMLDialogElement>(null);
  const [phrase, setPhrase] = useState("");
  const countdown = useCountdown(session && phaseActive(session.phase) ? session.deadline_at : undefined);
  const openUrl = session ? uiEndpoint(session) : undefined;
  const drain = app.drainFailed || /drain did not finish/i.test(session?.error ?? "");

  async function stop(force: boolean) {
    const next = await app.run(() =>
      app.client.stop({ sessionId: session?.id, force }),
    );
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
        <h1>Session</h1>
        <p className="lede">Phase, hard-stop countdown, and the catalog cost estimate.</p>
      </div>

      {!session ? (
        <article className="card">
          <h2>No session loaded</h2>
          <p className="muted">
            {app.client.mode === "cli"
              ? "Status uses the current session remembered by the CLI."
              : "Live status needs the desktop app. The PWA cannot shell out to wckd."}
          </p>
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
        </article>
      ) : (
        <>
          <article className="card">
            <div className="card-row">
              <span className={`phase phase-${session.phase}`}>{phaseLabel(session.phase)}</span>
              <strong className={countdown?.elapsed ? "countdown late" : "countdown"}>
                {countdown ? (countdown.elapsed ? "Deadline elapsed" : countdown.label) : "No deadline"}
              </strong>
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
                <dd>{money(session.cost_estimate_usd)}</dd>
              </div>
              <div>
                <dt>Actual</dt>
                <dd>{session.cost_actual_usd === undefined ? "—" : money(session.cost_actual_usd)}</dd>
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
              <p>
                {session.offer.name} {session.offer.vram_gb} GB {session.offer.cloud}{" "}
                {money(session.offer.usd_per_hr)}/hr
              </p>
            ) : null}
            {session.error ? <p className="late">{session.error}</p> : null}
            {session.warning ? <p className="warn-text">{session.warning}</p> : null}
            {session.forced ? (
              <p className="late">Terminated with --force before drain completed.</p>
            ) : null}
            <div className="actions">
              <button type="button" className="btn" disabled={app.busy} onClick={() => void app.refreshSession()}>
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
              {phaseActive(session.phase) ? (
                <button type="button" className="btn danger" onClick={() => stopRef.current?.showModal()}>
                  Stop
                </button>
              ) : null}
            </div>
          </article>

          {drain ? (
            <article className="card warn-card">
              <h2>Drain failed</h2>
              <p>
                The pod is still running. Retry Stop to wait for the S3 drain again. Force deletes
                the pod even if <code>drain-ok</code> never arrived. Unsynced files on the pod can
                be lost. The CLI form is <code>wckd stop {session.id} --force</code>.
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
          <h2>Stop and drain?</h2>
          <p>
            Stop asks the sidecar to sync the project to S3, waits for the drain marker, then
            deletes the pod. If the marker does not arrive, the pod stays up.
          </p>
          <div className="actions">
            <button type="submit" className="btn" value="cancel">
              Cancel
            </button>
            <button
              type="submit"
              className="btn danger"
              value="stop"
              onClick={() => void stop(false)}
            >
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
          <h2>Force terminate</h2>
          <p>
            This is destructive. The pod is deleted even when the drain did not finish. Type{" "}
            <code>DESTROY</code> to continue.
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
            <button type="submit" className="btn danger" disabled={phrase !== "DESTROY" || app.busy}>
              Force stop
            </button>
          </div>
        </form>
      </dialog>
    </section>
  );
}
