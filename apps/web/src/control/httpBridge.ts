import { addLocalProject, loadLocalProjects } from "./localProjects";
import type {
  ConfigCheck,
  ControlClient,
  Preset,
  Project,
  Session,
  StartResult,
} from "./types";
import { ControlError } from "./types";

const UNAVAILABLE =
  "This installable app cannot start or stop a GPU. Open the Tauri desktop shell, which runs the wckd CLI on this machine.";

// TODO(http-bridge): `wckd serve` is the loopback engine (docs/API.md).
// Invincible will supervise that process and proxy `/v1/rent/*`, attaching the
// bearer from the mode-0600 key file. Do not put that key in localStorage.
// When this bridge is pointed at the proxy, map:
//   GET  /v1/rent/offers?preset&hours
//   POST /v1/rent/sessions — 409 confirmation_required is the quote, not a failure;
//        resend with confirm:true only after the user approves that offer
//   GET  /v1/rent/sessions and /v1/rent/sessions/{id}
//   POST /v1/rent/sessions/{id}/stop
// Until then CliBridge remains the working client. There is no auto-start path.

export class HttpBridge implements ControlClient {
  readonly mode = "http" as const;

  constructor(
    private readonly presets: Preset[],
    private readonly apiUrl: string,
  ) {}

  configCheck(): Promise<ConfigCheck> {
    return this.unavailable();
  }

  listOffers(): Promise<never> {
    return this.unavailable();
  }

  start(): Promise<StartResult> {
    return this.unavailable();
  }

  status(): Promise<Session | null> {
    return this.unavailable();
  }

  stop(): Promise<Session> {
    return this.unavailable();
  }

  listPresets(): Promise<Preset[]> {
    return Promise.resolve(this.presets);
  }

  listProjects(): Promise<Project[]> {
    return Promise.resolve(loadLocalProjects());
  }

  addProject(id: string): Promise<Project> {
    return Promise.resolve(addLocalProject(id));
  }

  openUrl(url: string): Promise<void> {
    window.open(url, "_blank", "noopener,noreferrer");
    return Promise.resolve();
  }

  private unavailable(): Promise<never> {
    const target = this.apiUrl.trim();
    const message = target
      ? `Control API (${target}) is not called from this PWA yet. ${UNAVAILABLE}`
      : UNAVAILABLE;
    return Promise.reject(new ControlError(message));
  }
}
