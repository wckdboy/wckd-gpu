import { isSlug } from "./parse";
import type { Project } from "./types";

const KEY = "wckd.projects.v1";

export function loadLocalProjects(): Project[] {
  if (typeof localStorage === "undefined") {
    return [];
  }
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter((id): id is string => typeof id === "string" && isSlug(id)).map(
      (id) => ({ id, saved: true, sessions: 0 }),
    );
  } catch {
    return [];
  }
}

export function addLocalProject(id: string): Project {
  if (!isSlug(id)) {
    throw new Error("project id must match [A-Za-z0-9][A-Za-z0-9_-]*");
  }
  const current = loadLocalProjects();
  if (!current.some((project) => project.id === id)) {
    const ids = [...current.map((project) => project.id), id].sort();
    localStorage.setItem(KEY, JSON.stringify(ids));
  }
  return { id, saved: true, sessions: 0 };
}
