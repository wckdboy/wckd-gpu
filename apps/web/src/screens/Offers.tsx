import { useState } from "react";

import { estimateUsd } from "../control/parse";
import { phaseActive, type Offer } from "../control/types";
import { prefixFor, useApp } from "../state/appState";
import { money } from "../ui/format";

const HOUR_CHOICES = [1, 2, 4, 8];

export function Offers() {
  const app = useApp();
  const [offers, setOffers] = useState<Offer[]>([]);
  const [dryRunText, setDryRunText] = useState("");
  const [confirmAnother, setConfirmAnother] = useState(false);
  const selected = offers.find((offer) => offer.id === app.draft.offerId) ?? offers[0];
  const estimate = selected ? estimateUsd(selected.usd_per_hr, app.draft.hours) : 0;
  const active = app.session ? phaseActive(app.session.phase) : false;

  async function rank() {
    if (!app.draft.presetId) {
      app.setError("Choose a preset first.");
      return;
    }
    setDryRunText("");
    const rows = await app.run(() =>
      app.client.listOffers({ presetId: app.draft.presetId, hours: app.draft.hours, limit: 20 }),
    );
    if (!rows) {
      return;
    }
    setOffers(rows);
    app.setDraft({ ...app.draft, offerId: rows[0]?.id ?? "" });
  }

  async function dryRun() {
    if (!ready()) {
      return;
    }
    const result = await app.run(() =>
      app.client.start({
        projectId: app.draft.projectId,
        presetId: app.draft.presetId,
        hours: app.draft.hours,
        offerId: selected?.id,
        dryRun: true,
      }),
    );
    if (!result || result.kind !== "dry_run") {
      return;
    }
    setDryRunText(
      `${result.dryRun.offer.name} · ${result.dryRun.offer.cloud} · ${money(result.dryRun.estimate_usd)} · ${result.dryRun.image}`,
    );
    app.setNotice("Dry run only. No pod was created.");
  }

  async function start(confirmed: boolean) {
    if (!ready()) {
      return;
    }
    if (active && !confirmed) {
      setConfirmAnother(true);
      return;
    }
    setConfirmAnother(false);
    const result = await app.run(() =>
      app.client.start({
        projectId: app.draft.projectId,
        presetId: app.draft.presetId,
        hours: app.draft.hours,
        offerId: selected?.id,
        dryRun: false,
      }),
    );
    if (!result || result.kind !== "session") {
      return;
    }
    app.setSession(result.session);
    app.setDrainFailed(false);
    app.setView("session");
    app.setNotice(`Session ${result.session.id} started. The deadline sweeper was armed by the CLI.`);
  }

  function ready(): boolean {
    if (!app.draft.projectId || !app.draft.presetId) {
      app.setError("Choose a project and a preset.");
      return false;
    }
    if (!(app.draft.hours > 0 && app.draft.hours <= 24)) {
      app.setError("Hours must be within (0, 24].");
      return false;
    }
    return true;
  }

  return (
    <section className="screen">
      <div className="screen-head">
        <h1>Offers</h1>
        <p className="lede">
          Score is (perf index / $/hr) × reliability × region. perf index values are labeled
          estimates, not live benchmarks. Estimate = hours × catalog $/hr, before storage.
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
            onChange={(event) =>
              app.setDraft({ ...app.draft, presetId: event.target.value, offerId: "" })
            }
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
            onChange={(event) =>
              app.setDraft({ ...app.draft, hours: Number(event.target.value) })
            }
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
        <button type="button" className="btn" disabled={app.busy} onClick={() => void rank()}>
          Rank offers
        </button>
      </div>
      <p className="muted tiny">
        Prefix {prefixFor(app.draft.projectId)}. Dry run is{" "}
        <code>wckd start --dry-run --hours {app.draft.hours || "…"} --project … --preset …</code>{" "}
        and does not create a pod.
      </p>

      {offers.length > 0 ? (
        <table>
          <thead>
            <tr>
              <th></th>
              <th>Rank</th>
              <th>GPU</th>
              <th>VRAM</th>
              <th>Cloud</th>
              <th>Region</th>
              <th>$/hr</th>
              <th>Estimate</th>
              <th>Score</th>
            </tr>
          </thead>
          <tbody>
            {offers.map((offer, index) => (
              <tr
                key={offer.id}
                className={selected?.id === offer.id ? "selected" : undefined}
                onClick={() => app.setDraft({ ...app.draft, offerId: offer.id })}
              >
                <td>
                  <input
                    type="radio"
                    name="offer"
                    checked={selected?.id === offer.id}
                    onChange={() => app.setDraft({ ...app.draft, offerId: offer.id })}
                  />
                </td>
                <td>{index + 1}</td>
                <td>
                  {offer.name}
                  <div className="tiny muted">{offer.availability || offer.id}</div>
                </td>
                <td>{offer.vram_gb}</td>
                <td>{offer.cloud}</td>
                <td>{offer.region || "—"}</td>
                <td>{money(offer.usd_per_hr)}</td>
                <td>{money(estimateUsd(offer.usd_per_hr, app.draft.hours))}</td>
                <td>{offer.score.toFixed(1)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="muted">Rank offers to see a price before start.</p>
      )}

      <div className="estimate-bar">
        <div>
          <div className="tiny muted">Estimate before start</div>
          <strong>{selected ? `${money(estimate)} for ${app.draft.hours}h` : "—"}</strong>
          {selected ? (
            <div className="tiny muted">
              {money(selected.usd_per_hr)}/hr × {app.draft.hours}h · {selected.name}
            </div>
          ) : null}
        </div>
        <div className="actions">
          <button type="button" className="btn" disabled={app.busy} onClick={() => void dryRun()}>
            Dry run
          </button>
          <button
            type="button"
            className="btn primary"
            disabled={app.busy || !selected}
            onClick={() => void start(false)}
          >
            Start
          </button>
        </div>
      </div>
      {dryRunText ? <p className="mono">{dryRunText}</p> : null}

      {confirmAnother ? (
        <div className="card warn-card">
          <h2>Rent another GPU?</h2>
          <p>
            {app.session?.id} is still {app.session?.phase}. Starting now provisions a second pod
            and a second bill.
          </p>
          <div className="actions">
            <button type="button" className="btn" onClick={() => setConfirmAnother(false)}>
              Cancel
            </button>
            <button type="button" className="btn primary" onClick={() => void start(true)}>
              Start another
            </button>
          </div>
        </div>
      ) : null}
    </section>
  );
}
