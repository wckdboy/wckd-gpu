import { useState } from "react";

import { phaseActive } from "../control/types";
import { useApp } from "../state/appState";
import { startGate } from "./startGate";

export function useSessionStart() {
  const app = useApp();
  const [confirmAnother, setConfirmAnother] = useState(false);
  const gate = startGate({
    mode: app.client.mode,
    busy: app.busy,
    offerSource: app.offerSource,
    offer: app.lockedOffer,
    projectId: app.draft.projectId,
    presetId: app.draft.presetId,
    hours: app.draft.hours,
  });
  const active = app.session ? phaseActive(app.session.phase) : false;

  async function start(confirmed: boolean) {
    if (!gate.ok || !app.lockedOffer) {
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
        offerId: app.lockedOffer?.id,
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

  return { gate, confirmAnother, setConfirmAnother, start, active };
}
