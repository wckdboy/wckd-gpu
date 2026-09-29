import { isPhase, type Phase } from "../control/types";

export function money(value: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

export function duration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  const pad = (value: number) => String(value).padStart(2, "0");
  if (hours > 0) {
    return `${hours}h ${pad(minutes)}m ${pad(seconds)}s`;
  }
  return `${minutes}m ${pad(seconds)}s`;
}

export function phaseLabel(phase: string): string {
  if (!isPhase(phase)) {
    return phase || "Unknown";
  }
  return label(phase);
}

function label(phase: Phase): string {
  switch (phase) {
    case "pending":
      return "Pending";
    case "hydrating":
      return "Hydrating";
    case "ready":
      return "Ready";
    case "draining":
      return "Draining";
    case "terminated":
      return "Terminated";
    case "failed":
      return "Failed";
    default: {
      const neverPhase: never = phase;
      return neverPhase;
    }
  }
}

export function uiEndpoint(session: { endpoints: { name: string; url: string }[] }): string | undefined {
  const named = session.endpoints.find((endpoint) => endpoint.name === "ui" && endpoint.url);
  return named?.url ?? session.endpoints.find((endpoint) => endpoint.url.startsWith("http"))?.url;
}
