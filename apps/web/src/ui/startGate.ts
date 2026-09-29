import type { Offer } from "../control/types";

export type OfferSource = "none" | "live" | "sample";

export function startGate(input: {
  mode: "cli" | "http";
  busy: boolean;
  offerSource: OfferSource;
  offer: Offer | null;
  projectId: string;
  presetId: string;
  hours: number;
}): { ok: boolean; reason: string } {
  if (input.mode !== "cli") {
    return { ok: false, reason: "Desktop required for Start." };
  }
  if (input.busy) {
    return { ok: false, reason: "Provisioning." };
  }
  if (input.offerSource === "sample") {
    return { ok: false, reason: "Sample rank. Rank live offers before Start." };
  }
  if (!input.offer) {
    return { ok: false, reason: "Lock an offer before Start." };
  }
  if (!input.projectId || !input.presetId) {
    return { ok: false, reason: "Choose a project and a preset." };
  }
  if (!(input.hours > 0 && input.hours <= 24)) {
    return { ok: false, reason: "Hours must be within (0, 24]." };
  }
  return { ok: true, reason: "" };
}
