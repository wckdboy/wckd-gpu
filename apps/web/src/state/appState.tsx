import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { bundledPresets } from "../control/bundledPresets";
import { createClient } from "../control/client";
import { loadSettings, saveSettings, type ClientSettings } from "../control/settings";
import {
  ControlError,
  phaseActive,
  type ControlClient,
  type Offer,
  type Preset,
  type Project,
  type Session,
} from "../control/types";
import type { OfferSource } from "../ui/startGate";

export type View = "home" | "projects" | "presets" | "offers" | "session" | "settings";

export interface Draft {
  projectId: string;
  presetId: string;
  hours: number;
  offerId: string;
}

interface AppState {
  view: View;
  setView: (view: View) => void;
  settings: ClientSettings;
  updateSettings: (settings: ClientSettings) => void;
  client: ControlClient;
  presets: Preset[];
  projects: Project[];
  session: Session | null;
  setSession: (session: Session | null) => void;
  draft: Draft;
  setDraft: (draft: Draft) => void;
  lockedOffer: Offer | null;
  offerSource: OfferSource;
  lockOffer: (offer: Offer | null, source: OfferSource) => void;
  error: string;
  setError: (error: string) => void;
  notice: string;
  setNotice: (notice: string) => void;
  busy: boolean;
  drainFailed: boolean;
  setDrainFailed: (value: boolean) => void;
  refreshCatalog: () => Promise<void>;
  refreshSession: () => Promise<void>;
  run: <T>(action: () => Promise<T>) => Promise<T | undefined>;
}

const AppContext = createContext<AppState | null>(null);

const HOURS = [1, 2, 4, 8];

export function AppProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<ClientSettings>(() => loadSettings());
  const [view, setView] = useState<View>("home");
  const [presets, setPresets] = useState<Preset[]>(() => bundledPresets());
  const [projects, setProjects] = useState<Project[]>([]);
  const [session, setSession] = useState<Session | null>(null);
  const [draft, setDraft] = useState<Draft>({
    projectId: "",
    presetId: bundledPresets()[0]?.id ?? "",
    hours: 4,
    offerId: "",
  });
  const [lockedOffer, setLockedOffer] = useState<Offer | null>(null);
  const [offerSource, setOfferSource] = useState<OfferSource>("none");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [drainFailed, setDrainFailed] = useState(false);

  const client = useMemo(() => createClient(settings, presets), [settings, presets]);

  const updateSettings = useCallback((next: ClientSettings) => {
    saveSettings(next);
    setSettings(next);
  }, []);

  const lockOffer = useCallback((offer: Offer | null, source: OfferSource) => {
    setLockedOffer(offer);
    setOfferSource(offer ? source : "none");
    setDraft((current) => ({ ...current, offerId: offer?.id ?? "" }));
  }, []);

  const refreshCatalog = useCallback(async () => {
    const bridge = createClient(settings, bundledPresets());
    try {
      const nextPresets = await bridge.listPresets();
      if (nextPresets.length > 0) {
        setPresets(nextPresets);
      }
    } catch (err) {
      if (bridge.mode === "cli") {
        setError(messageOf(err));
      }
    }
    try {
      setProjects(await bridge.listProjects());
    } catch (err) {
      if (bridge.mode === "cli") {
        setError(messageOf(err));
      }
    }
  }, [settings]);

  const refreshSession = useCallback(async () => {
    const bridge = createClient(settings);
    if (bridge.mode !== "cli") {
      return;
    }
    try {
      const next = await bridge.status();
      setSession(next);
      if (next?.error && /drain did not finish/i.test(next.error)) {
        setDrainFailed(true);
      }
    } catch (err) {
      setError(messageOf(err));
    }
  }, [settings]);

  useEffect(() => {
    void refreshCatalog();
    void refreshSession();
  }, [refreshCatalog, refreshSession]);

  useEffect(() => {
    if (client.mode !== "cli" || !session || !phaseActive(session.phase)) {
      return;
    }
    const timer = window.setInterval(() => {
      if (document.hidden) {
        return;
      }
      void refreshSession();
    }, 10000);
    return () => window.clearInterval(timer);
  }, [client.mode, refreshSession, session]);

  useEffect(() => {
    setDraft((current) => {
      const projectId =
        current.projectId || projects[0]?.id || "";
      const presetId =
        current.presetId || presets[0]?.id || "";
      const hours = HOURS.includes(current.hours) || current.hours > 0 ? current.hours : 4;
      if (projectId === current.projectId && presetId === current.presetId && hours === current.hours) {
        return current;
      }
      return { ...current, projectId, presetId, hours };
    });
  }, [presets, projects]);

  const run = useCallback(async <T,>(action: () => Promise<T>) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      return await action();
    } catch (err) {
      if (err instanceof ControlError && err.session) {
        setSession(err.session);
      }
      if (err instanceof ControlError && err.drainFailed) {
        setDrainFailed(true);
      }
      setError(messageOf(err));
      return undefined;
    } finally {
      setBusy(false);
    }
  }, []);

  const value = useMemo<AppState>(
    () => ({
      view,
      setView,
      settings,
      updateSettings,
      client,
      presets,
      projects,
      session,
      setSession,
      draft,
      setDraft,
      lockedOffer,
      offerSource,
      lockOffer,
      error,
      setError,
      notice,
      setNotice,
      busy,
      drainFailed,
      setDrainFailed,
      refreshCatalog,
      refreshSession,
      run,
    }),
    [
      view,
      settings,
      updateSettings,
      client,
      presets,
      projects,
      session,
      draft,
      lockedOffer,
      offerSource,
      lockOffer,
      error,
      notice,
      busy,
      drainFailed,
      refreshCatalog,
      refreshSession,
      run,
    ],
  );

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

export function useApp(): AppState {
  const value = useContext(AppContext);
  if (!value) {
    throw new Error("useApp requires AppProvider");
  }
  return value;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return "Something went wrong";
}

export function prefixFor(projectId: string): string {
  return projectId ? `projects/${projectId}/` : "projects/<id>/";
}
