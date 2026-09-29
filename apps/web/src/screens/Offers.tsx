import { useState } from "react";

import { estimateUsd } from "../control/parse";
import type { Offer } from "../control/types";
import { prefixFor, useApp } from "../state/appState";
import { money } from "../ui/format";
import { sampleOffers } from "../ui/sample";
import { StartDock } from "../ui/StartDock";
import { useSessionStart } from "../ui/useSessionStart";

const HOUR_CHOICES = [1, 2, 4, 8];

export function Offers() {
  const app = useApp();
  const [offers, setOffers] = useState<Offer[]>([]);
  const [ranking, setRanking] = useState(false);
  const [dryRunText, setDryRunText] = useState("");
  const { gate, confirmAnother, setConfirmAnother, start } = useSessionStart();
  const selected = app.lockedOffer;
  const estimate = selected ? estimateUsd(selected.usd_per_hr, app.draft.hours) : null;
  const next = nextOffer(offers, selected?.id);

  async function rank() {
    if (!app.draft.presetId) {
      app.setError("Choose a preset first.");
      return;
    }
    setDryRunText("");
    setRanking(true);
    const rows = await app.run(() =>
      app.client.listOffers({ presetId: app.draft.presetId, hours: app.draft.hours, limit: 20 }),
    );
    setRanking(false);
    if (!rows) {
      return;
    }
    setOffers(rows);
    app.lockOffer(rows[0] ?? null, rows[0] ? "live" : "none");
  }

  function previewSample() {
    const rows = sampleOffers(app.draft.hours);
    setOffers(rows);
    app.lockOffer(rows[0] ?? null, "sample");
    app.setNotice("Sample rank. Not a live quote. Desktop required to call wckd offers.");
    app.setError("");
  }

  function lock(offer: Offer) {
    app.lockOffer(offer, app.offerSource === "sample" ? "sample" : "live");
  }

  async function dryRun() {
    if (!app.draft.projectId || !app.draft.presetId || !selected) {
      app.setError("Choose a project, a preset, and an offer.");
      return;
    }
    if (app.offerSource === "sample") {
      app.setError("Sample rank. Dry run needs the desktop app and a live offer list.");
      return;
    }
    const result = await app.run(() =>
      app.client.start({
        projectId: app.draft.projectId,
        presetId: app.draft.presetId,
        hours: app.draft.hours,
        offerId: selected.id,
        dryRun: true,
      }),
    );
    if (!result || result.kind !== "dry_run") {
      return;
    }
    setDryRunText(
      `${result.dryRun.offer.sku} · ${result.dryRun.offer.cloud} · ${money(result.dryRun.estimate_usd)} · ${result.dryRun.image}`,
    );
    app.setNotice("Dry run only. No pod was created.");
  }

  return (
    <section className="screen">
      <div className="screen-head">
        <span className="tag">Offers</span>
        <h1>Ranked quotes</h1>
        <p className="lede">
          Score is (perf index / $/hr) × reliability × region. perf index values are labeled
          estimates. Est. total = hours × catalog $/hr, before storage.
        </p>
      </div>

      <div className="filters">
        <label>
          Project
          <select
            value={app.draft.projectId}
            onChange={(event) => app.setDraft({ ...app.draft, projectId: event.target.value })}
          >
            <option value="">Select</option>
            {app.projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.id}
              </option>
            ))}
          </select>
        </label>
        <label>
          Preset
          <select
            value={app.draft.presetId}
            onChange={(event) => {
              app.setDraft({ ...app.draft, presetId: event.target.value, offerId: "" });
              app.lockOffer(null, "none");
              setOffers([]);
            }}
          >
            <option value="">Select</option>
            {app.presets.map((preset) => (
              <option key={preset.id} value={preset.id}>
                {preset.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Hours
          <input
            type="number"
            min={0.25}
            max={24}
            step={0.25}
            value={app.draft.hours}
            onChange={(event) => app.setDraft({ ...app.draft, hours: Number(event.target.value) })}
          />
        </label>
        <div className="chips">
          {HOUR_CHOICES.map((hours) => (
            <button
              key={hours}
              type="button"
              className={app.draft.hours === hours ? "chip on" : "chip"}
              onClick={() => app.setDraft({ ...app.draft, hours })}
            >
              {hours}h
            </button>
          ))}
        </div>
        <button type="button" className="btn" disabled={app.busy || ranking} onClick={() => void rank()}>
          Rank offers
        </button>
      </div>
      <p className="tiny">
        Prefix {prefixFor(app.draft.projectId)}. Dry run is{" "}
        <code>wckd start --dry-run --hours {app.draft.hours || "…"}</code> and does not create a pod.
      </p>

      {app.offerSource === "sample" ? (
        <div className="banner warn" role="status">
          <span className="sample-flag">SAMPLE</span> — not a live quote. Desktop required to call{" "}
          <code>wckd offers</code>.
        </div>
      ) : null}

      {app.error && next ? (
        <div className="banner warn">
          <div className="banner-row">
            <span>Next offer failover. Lock the following row and retry.</span>
            <button type="button" className="btn" onClick={() => lock(next)}>
              Lock {next.sku}
            </button>
          </div>
        </div>
      ) : null}

      {ranking ? (
        <div className="table-wrap" aria-busy="true" aria-label="Ranking offers">
          <table>
            <OfferHead />
            <tbody>
              {Array.from({ length: 5 }, (_, index) => (
                <tr key={index} className="skeleton-row">
                  <td colSpan={7}>
                    <span className="skeleton" />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : offers.length > 0 ? (
        <div className="table-wrap">
          <table>
            <OfferHead />
            <tbody>
              {offers.map((offer, index) => {
                const locked = selected?.id === offer.id;
                const recommended = index === 0;
                return (
                  <tr
                    key={offer.id}
                    className={rowClass(locked, recommended)}
                    onClick={() => lock(offer)}
                  >
                    <td className="mono">
                      {offer.score.toFixed(1)}
                      {recommended ? <span className="rec">REC</span> : null}
                    </td>
                    <td>{offer.vendor}</td>
                    <td>
                      {offer.sku}
                      <div className="tiny">{offer.region || offer.name}</div>
                    </td>
                    <td className="mono">{offer.vram_gb} GB</td>
                    <td className={offer.cloud === "secure" ? "tier-secure" : undefined}>{offer.cloud}</td>
                    <td className="mono">{money(offer.usd_per_hr)}</td>
                    <td className="mono">{money(estimateUsd(offer.usd_per_hr, app.draft.hours))}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="empty">
          <p className="mono empty-title">no offers</p>
          <p className="muted">
            {app.client.mode === "cli"
              ? "Rank a preset to price a session before Start."
              : "Desktop required to call wckd offers. Preview a sample rank to read the sheet."}
          </p>
          {app.client.mode === "cli" ? (
            <button type="button" className="btn" disabled={app.busy} onClick={() => void rank()}>
              Rank offers
            </button>
          ) : (
            <button type="button" className="btn" onClick={previewSample}>
              Preview sample rank
            </button>
          )}
        </div>
      )}

      {dryRunText ? <p className="mono">{dryRunText}</p> : null}

      {confirmAnother ? (
        <div className="card warn-card">
          <h2>Rent another GPU?</h2>
          <p>
            {app.session?.id} is still {app.session?.phase}. Starting now provisions a second pod and
            a second bill.
          </p>
          <div className="actions">
            <button type="button" className="btn" onClick={() => setConfirmAnother(false)}>
              Cancel
            </button>
            <p className="tiny">Press Start again to rent the second pod.</p>
          </div>
        </div>
      ) : null}

      <StartDock
        estimate={estimate}
        rate={selected?.usd_per_hr ?? null}
        hours={app.draft.hours}
        detail={selected ? `${selected.vendor} · ${selected.sku} · ${selected.vram_gb} GB` : undefined}
        disabled={!gate.ok}
        reason={gate.reason}
        busy={app.busy}
        onStart={() => void start(confirmAnother)}
        secondary={
          <button type="button" className="btn" disabled={app.busy} onClick={() => void dryRun()}>
            Dry run
          </button>
        }
      />
    </section>
  );
}

function OfferHead() {
  return (
    <thead>
      <tr>
        <th>Score</th>
        <th>Vendor</th>
        <th>SKU</th>
        <th>VRAM</th>
        <th>Tier</th>
        <th>$/hr</th>
        <th>Est. total</th>
      </tr>
    </thead>
  );
}

function rowClass(locked: boolean, recommended: boolean): string | undefined {
  if (locked && recommended) {
    return "locked recommended";
  }
  if (locked) {
    return "locked";
  }
  if (recommended) {
    return "recommended";
  }
  return undefined;
}

function nextOffer(offers: Offer[], selectedId: string | undefined): Offer | undefined {
  if (offers.length < 2) {
    return undefined;
  }
  const index = offers.findIndex((offer) => offer.id === selectedId);
  if (index < 0) {
    return offers[1];
  }
  return offers[index + 1] ?? offers[0];
}
