import { useApp } from "../state/appState";

export function Presets() {
  const app = useApp();

  return (
    <section className="screen">
      <div className="screen-head">
        <h1>Presets</h1>
        <p className="lede">
          Loaded from <code>presets/*.yaml</code>
          {app.client.mode === "cli"
            ? " via wckd presets, with the repo copies bundled as a fallback."
            : " bundled into this app. The desktop shell can also read the CLI presets directory."}
        </p>
      </div>
      <div className="preset-grid">
        {app.presets.map((preset) => (
          <article
            key={preset.id}
            className={app.draft.presetId === preset.id ? "card selected-card" : "card"}
          >
            <div className="card-row">
              <h2>{preset.name}</h2>
              <span className="phase">{preset.workload_class}</span>
            </div>
            <p className="mono muted">{preset.id}</p>
            <dl className="facts">
              <div>
                <dt>VRAM</dt>
                <dd>{preset.min_vram_gb} GB min</dd>
              </div>
              <div>
                <dt>RAM</dt>
                <dd>{preset.min_ram_gb} GB</dd>
              </div>
              <div>
                <dt>Disk</dt>
                <dd>{preset.min_disk_gb} GB</dd>
              </div>
              <div>
                <dt>Reliability</dt>
                <dd>{preset.reliability}</dd>
              </div>
            </dl>
            <p className="muted">{preset.gpu_families.join(" · ") || "Any GPU family"}</p>
            <p className="notes">{preset.notes || "No notes."}</p>
            <p className="mono tiny">{preset.image}</p>
            <button
              type="button"
              className="btn"
              onClick={() => {
                app.setDraft({ ...app.draft, presetId: preset.id, offerId: "" });
                app.setView("offers");
              }}
            >
              Use preset
            </button>
          </article>
        ))}
      </div>
      {app.presets.length === 0 ? <p className="muted">No presets found.</p> : null}
    </section>
  );
}
