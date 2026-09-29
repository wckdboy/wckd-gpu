import { presetFromYaml } from "./parse";
import type { Preset } from "./types";

const rawPresets = import.meta.glob("../../../../presets/*.yaml", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

export function bundledPresets(): Preset[] {
  return Object.keys(rawPresets)
    .sort()
    .map((path) => presetFromYaml(rawPresets[path] ?? ""));
}
