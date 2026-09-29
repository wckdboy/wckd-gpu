import { useState } from "react";

import { createClient } from "../control/client";
import type { ClientSettings } from "../control/settings";
import type { ConfigCheck } from "../control/types";
import { useApp } from "../state/appState";

export function Settings() {
  const app = useApp();
  const [form, setForm] = useState<ClientSettings>(app.settings);
  const [report, setReport] = useState<ConfigCheck | null>(null);

  function setField<K extends keyof ClientSettings>(key: K, value: ClientSettings[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function check() {
    app.updateSettings(form);
    const next = await app.run(() => createClient(form).configCheck());
    if (next) {
      setReport(next);
      app.setNotice(next.ok ? "RunPod and S3 responded." : "Config check failed. No pod was created.");
    }
  }

  return (
    <section className="screen">
      <div className="screen-head">
        <h1>Settings</h1>
        <p className="lede">
          The desktop shell runs the <code>wckd</code> binary. Credentials stay in that process:
          a <code>.env</code> in the working directory, or variables already exported. This UI
          stores the binary path, config path, and working directory only.
        </p>
      </div>

      <article className="card">
        <h2>Secrets</h2>
        <p>
          Do not paste API keys here. They are not written to localStorage, and the PWA cannot
          read your <code>.env</code>. On the desktop app the CLI loads them from the working
          directory. A future control plane would keep provider keys in its vault;{" "}
          <code>HttpBridge</code> is the stub for that API.
        </p>
        <p className="muted">
          PWA installs can browse presets and local project ids. Start, status, stop, and config
          check need the Tauri app until the control plane exists.
        </p>
      </article>

      <form
        className="stack"
        onSubmit={(event) => {
          event.preventDefault();
          app.updateSettings(form);
          app.setNotice("Saved on this device. No secrets were stored.");
        }}
      >
        <label>
          Path to wckd
          <input
            value={form.wckdBin}
            onChange={(event) => setField("wckdBin", event.target.value)}
            placeholder="wckd"
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <label>
          Working directory
          <input
            value={form.workDir}
            onChange={(event) => setField("workDir", event.target.value)}
            placeholder="/path/to/checkout (where .env lives)"
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <label>
          Config file
          <input
            value={form.configPath}
            onChange={(event) => setField("configPath", event.target.value)}
            placeholder="optional wckd.yaml"
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <label>
          Future control API URL
          <input
            value={form.controlApiUrl}
            onChange={(event) => setField("controlApiUrl", event.target.value)}
            placeholder="https://… (not used yet)"
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <div className="actions">
          <button type="submit" className="btn">
            Save
          </button>
          <button type="button" className="btn primary" disabled={app.busy} onClick={() => void check()}>
            Config check
          </button>
        </div>
      </form>

      {report ? (
        <article className="card">
          <h2>{report.ok ? "Ready" : "Not ready"}</h2>
          <dl className="facts">
            <div>
              <dt>RunPod</dt>
              <dd>{probeLabel(report.runpod)}</dd>
            </div>
            <div>
              <dt>S3</dt>
              <dd>
                {probeLabel(report.s3)}
                {report.s3.bucket ? ` · ${report.s3.bucket}` : ""}
              </dd>
            </div>
            <div>
              <dt>State</dt>
              <dd className="mono">{report.state_dir || "—"}</dd>
            </div>
            <div>
              <dt>Presets</dt>
              <dd className="mono">{report.presets_dir || "—"}</dd>
            </div>
          </dl>
          {report.missing.length > 0 ? (
            <ul>
              {report.missing.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          ) : null}
        </article>
      ) : null}

      <article className="card">
        <h2>Dry run from a terminal</h2>
        <pre className="mono">{`wckd config check
wckd offers --preset comfyui-minimax-h3 --hours 1 --json
wckd start --dry-run --hours 1 --project hailuo-tests --preset comfyui-minimax-h3
wckd status --json
wckd stop`}</pre>
        <p className="muted">
          <code>--dry-run</code> prints the pick and the estimate. It does not create a pod. A real
          start spends RunPod money. <code>--force</code> on stop deletes the pod without a
          confirmed drain.
        </p>
      </article>
    </section>
  );
}

function probeLabel(probe: { ok: boolean; skipped?: boolean; error?: string }): string {
  if (probe.ok) {
    return "ok";
  }
  if (probe.skipped) {
    return "not checked";
  }
  return probe.error || "fail";
}
