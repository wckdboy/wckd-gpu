import type { ReactNode } from "react";

import { phaseActive } from "../control/types";
import { useApp, type View } from "../state/appState";
import { Home } from "../screens/Home";
import { Offers } from "../screens/Offers";
import { Presets } from "../screens/Presets";
import { Projects } from "../screens/Projects";
import { SessionScreen } from "../screens/Session";
import { Settings } from "../screens/Settings";
import { phaseLabel } from "./format";
import { useCountdown } from "./useCountdown";

const NAV: { id: View; label: string }[] = [
  { id: "home", label: "Home" },
  { id: "projects", label: "Projects" },
  { id: "presets", label: "Presets" },
  { id: "offers", label: "Offers" },
  { id: "session", label: "Session" },
  { id: "settings", label: "Settings" },
];

export function Shell() {
  const app = useApp();
  const countdown = useCountdown(
    app.session && phaseActive(app.session.phase) ? app.session.deadline_at : undefined,
  );

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand-lock">
          <span className={app.session?.phase === "ready" ? "live-pip on" : "live-pip"} />
          <div className="brand">wckd</div>
        </div>
        <div className="top-meta">
          {app.session ? (
            <span className={`phase phase-${app.session.phase}`}>{phaseLabel(app.session.phase)}</span>
          ) : (
            <span className="tiny">No session</span>
          )}
          {countdown ? (
            <span className={countdown.elapsed ? "countdown late" : countdown.warn ? "countdown is-warn" : "countdown"}>
              {countdown.elapsed ? "Deadline elapsed" : countdown.label}
            </span>
          ) : null}
          <span className="mode">{app.client.mode === "cli" ? "Desktop" : "PWA"}</span>
        </div>
      </header>
      <nav className="nav">
        {NAV.map((item) => (
          <button
            key={item.id}
            type="button"
            className={app.view === item.id ? "nav-item active" : "nav-item"}
            onClick={() => app.setView(item.id)}
          >
            {item.label}
          </button>
        ))}
      </nav>
      <main className="main">
        {app.error ? (
          <div className="banner error" role="alert">
            {app.error}
          </div>
        ) : null}
        {app.notice ? <div className="banner ok">{app.notice}</div> : null}
        {renderView(app.view)}
      </main>
    </div>
  );
}

function renderView(view: View): ReactNode {
  switch (view) {
    case "home":
      return <Home />;
    case "projects":
      return <Projects />;
    case "presets":
      return <Presets />;
    case "offers":
      return <Offers />;
    case "session":
      return <SessionScreen />;
    case "settings":
      return <Settings />;
    default: {
      const neverView: never = view;
      return neverView;
    }
  }
}
