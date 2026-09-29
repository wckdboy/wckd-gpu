import type { ReactNode } from "react";

import { money } from "./format";

export function StartDock({
  estimate,
  rate,
  hours,
  detail,
  disabled,
  reason,
  busy,
  onStart,
  secondary,
}: {
  estimate: number | null;
  rate: number | null;
  hours: number;
  detail?: string;
  disabled: boolean;
  reason: string;
  busy: boolean;
  onStart: () => void;
  secondary?: ReactNode;
}) {
  return (
    <aside className="start-dock" aria-label="Start">
      <div className="dock-copy">
        <span className="tag">Before start</span>
        <div className="dock-metrics">
          <div>
            <span className="tag">Est. total</span>
            <strong className="mono">{estimate === null ? "—" : money(estimate)}</strong>
          </div>
          <div>
            <span className="tag">Rate</span>
            <strong className="mono">{rate === null ? "—" : `${money(rate)}/hr`}</strong>
          </div>
          <div>
            <span className="tag">Duration</span>
            <strong className="mono">{hours > 0 ? `${hours}h` : "—"}</strong>
          </div>
        </div>
        <p className="risk-line">Hard stop at deadline; unpaid orphans are a failure.</p>
        {detail ? <p className="tiny">{detail}</p> : null}
        {reason ? <p className="dock-reason">{reason}</p> : null}
      </div>
      <div className="dock-actions">
        {secondary}
        <button type="button" className="btn primary xl" disabled={disabled || busy} onClick={onStart}>
          {busy ? "Provisioning" : "Start"}
        </button>
      </div>
    </aside>
  );
}
