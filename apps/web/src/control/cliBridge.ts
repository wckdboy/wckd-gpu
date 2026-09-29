import { invoke } from "@tauri-apps/api/core";

import type { ClientSettings } from "./settings";
import {
  parseConfigCheck,
  parseOffers,
  parsePresets,
  parseProject,
  parseProjects,
  parseSession,
  parseStartResult,
} from "./parse";
import {
  ControlError,
  isDrainFailure,
  type ConfigCheck,
  type ControlClient,
  type OffersQuery,
  type Preset,
  type Project,
  type Session,
  type StartInput,
  type StartResult,
  type StopInput,
} from "./types";

interface CmdOutput {
  code: number;
  stdout: string;
  stderr: string;
}

interface BridgeOpts {
  bin: string;
  configPath: string;
  workDir: string;
}

export class CliBridge implements ControlClient {
  readonly mode = "cli" as const;

  constructor(private readonly settings: ClientSettings) {}

  async configCheck(): Promise<ConfigCheck> {
    const out = await this.run("config_check", {});
    try {
      return parseConfigCheck(out.stdout);
    } catch (err) {
      throw failure(out, err);
    }
  }

  async listOffers(query: OffersQuery): Promise<ReturnType<typeof parseOffers>> {
    const out = await this.run("list_offers", {
      preset: query.presetId,
      hours: query.hours,
      limit: query.limit ?? 20,
    });
    if (out.code !== 0) {
      throw failure(out);
    }
    return parseOffers(out.stdout, query.hours);
  }

  async start(input: StartInput): Promise<StartResult> {
    const out = await this.run("start_session", {
      project: input.projectId,
      preset: input.presetId,
      hours: input.hours,
      offerId: input.offerId ?? "",
      dryRun: input.dryRun === true,
    });
    if (out.code !== 0) {
      throw failure(out);
    }
    return parseStartResult(out.stdout, input.hours);
  }

  async status(sessionId?: string): Promise<Session | null> {
    const out = await this.run("session_status", { sessionId: sessionId ?? "" });
    const blob = `${out.stderr}\n${out.stdout}`;
    if (out.code !== 0 && /no current session/i.test(blob)) {
      return null;
    }
    if (out.code !== 0) {
      throw failure(out);
    }
    return parseSession(out.stdout);
  }

  async stop(input: StopInput): Promise<Session> {
    const out = await this.run("stop_session", {
      sessionId: input.sessionId ?? "",
      force: input.force === true,
    });
    let session: Session | undefined;
    try {
      session = parseSession(out.stdout);
    } catch {
      session = undefined;
    }
    if (out.code !== 0) {
      throw new ControlError(cliMessage(out), {
        drainFailed: isDrainFailure(`${out.stderr}\n${out.stdout}`),
        session,
      });
    }
    if (!session) {
      throw new ControlError("stop returned no session");
    }
    return session;
  }

  async listPresets(): Promise<Preset[]> {
    const out = await this.run("list_presets", {});
    if (out.code !== 0) {
      throw failure(out);
    }
    return parsePresets(out.stdout);
  }

  async listProjects(): Promise<Project[]> {
    const out = await this.run("list_projects", {});
    if (out.code !== 0) {
      throw failure(out);
    }
    return parseProjects(out.stdout);
  }

  async addProject(id: string): Promise<Project> {
    const out = await this.run("add_project", { id });
    if (out.code !== 0) {
      throw failure(out);
    }
    return parseProject(out.stdout);
  }

  async openUrl(url: string): Promise<void> {
    await invoke("open_external", { url });
  }

  private run(command: string, args: Record<string, unknown>): Promise<CmdOutput> {
    const opts: BridgeOpts = {
      bin: this.settings.wckdBin,
      configPath: this.settings.configPath,
      workDir: this.settings.workDir,
    };
    return invoke<CmdOutput>(command, { opts, ...args });
  }
}

function failure(out: CmdOutput, cause?: unknown): ControlError {
  if (cause instanceof Error && out.stdout.trim() === "") {
    return new ControlError(cause.message);
  }
  return new ControlError(cliMessage(out));
}

function cliMessage(out: CmdOutput): string {
  const stderr = out.stderr.trim().replace(/^error:\s*/i, "");
  const stdout = out.stdout.trim();
  if (stderr) {
    return stderr;
  }
  if (stdout) {
    return stdout;
  }
  return `wckd exited ${out.code}`;
}
