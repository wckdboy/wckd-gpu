export interface ClientSettings {
  wckdBin: string;
  configPath: string;
  workDir: string;
  controlApiUrl: string;
}

export const defaultSettings: ClientSettings = {
  wckdBin: "wckd",
  configPath: "",
  workDir: "",
  controlApiUrl: "",
};

const KEY = "wckd.settings.v1";

export function loadSettings(): ClientSettings {
  if (typeof localStorage === "undefined") {
    return { ...defaultSettings };
  }
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) {
      return { ...defaultSettings };
    }
    const parsed: unknown = JSON.parse(raw);
    if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ...defaultSettings };
    }
    const row = parsed as Record<string, unknown>;
    return {
      wckdBin: stringField(row.wckdBin, defaultSettings.wckdBin),
      configPath: stringField(row.configPath, ""),
      workDir: stringField(row.workDir, ""),
      controlApiUrl: stringField(row.controlApiUrl, ""),
    };
  } catch {
    return { ...defaultSettings };
  }
}

export function saveSettings(settings: ClientSettings): void {
  const stored: ClientSettings = {
    wckdBin: settings.wckdBin.trim(),
    configPath: settings.configPath.trim(),
    workDir: settings.workDir.trim(),
    controlApiUrl: settings.controlApiUrl.trim(),
  };
  localStorage.setItem(KEY, JSON.stringify(stored));
}

function stringField(value: unknown, fallback: string): string {
  return typeof value === "string" ? value : fallback;
}
