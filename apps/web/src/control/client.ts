import { bundledPresets } from "./bundledPresets";
import { CliBridge } from "./cliBridge";
import { inTauri } from "./env";
import { HttpBridge } from "./httpBridge";
import type { ClientSettings } from "./settings";
import type { ControlClient, Preset } from "./types";

export function createClient(settings: ClientSettings, presets: Preset[] = bundledPresets()): ControlClient {
  if (inTauri()) {
    return new CliBridge(settings);
  }
  return new HttpBridge(presets, settings.controlApiUrl);
}
