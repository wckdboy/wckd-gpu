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
  "This installable app cannot start or stop a GPU. Open the Tauri desktop shell, which runs the wckd CLI on this machine. The HTTP control plane is not available yet.";

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
      ? `Control API (${target}) is not implemented yet. ${UNAVAILABLE}`
      : UNAVAILABLE;
    return Promise.reject(new ControlError(message));
  }
}
