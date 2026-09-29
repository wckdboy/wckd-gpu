import { useState } from "react";

import { isSlug } from "../control/parse";
import { prefixFor, useApp } from "../state/appState";

export function Projects() {
  const app = useApp();
  const [id, setId] = useState("");
  const invalid = id.trim() !== "" && !isSlug(id.trim());

  async function createProject() {
    const next = id.trim();
    if (!isSlug(next)) {
      app.setError("Project id must match [A-Za-z0-9][A-Za-z0-9_-]* and be one S3 prefix segment.");
      return;
    }
    await app.run(async () => {
      await app.client.addProject(next);
      await app.refreshCatalog();
      app.setDraft({ ...app.draft, projectId: next });
      app.setNotice(`Saved ${next}. Start uses the prefix ${prefixFor(next)}.`);
      setId("");
    });
  }

  return (
    <section className="screen">
      <div className="screen-head">
        <span className="tag">Projects</span>
        <h1>Projects</h1>
        <p className="lede">
          A project id is the S3 prefix <code>projects/&lt;id&gt;/</code>. Creating one here only
          remembers it locally. <code>wckd start</code> writes the prefix when a session begins.
        </p>
      </div>
      <form
        className="inline-form"
        onSubmit={(event) => {
          event.preventDefault();
          void createProject();
        }}
      >
        <label>
          New project id
          <input
            value={id}
            onChange={(event) => setId(event.target.value)}
            placeholder="hailuo-tests"
            autoComplete="off"
            spellCheck={false}
          />
        </label>
        <button type="submit" className="btn" disabled={app.busy || invalid || id.trim() === ""}>
          Create
        </button>
      </form>
      {invalid ? <p className="late">Use letters, numbers, hyphens, or underscores. No slashes.</p> : null}
      {app.projects.length === 0 ? (
        <div className="empty">
          <p className="mono empty-title">no projects</p>
          <p className="muted">Add an id above. It becomes the S3 prefix when a session starts.</p>
        </div>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Id</th>
              <th>Prefix</th>
              <th>Saved</th>
              <th>Sessions</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {app.projects.map((project) => (
              <tr key={project.id} className={app.draft.projectId === project.id ? "selected" : undefined}>
                <td className="mono">{project.id}</td>
                <td className="mono">{prefixFor(project.id)}</td>
                <td>{project.saved ? "yes" : "from sessions"}</td>
                <td>{project.sessions}</td>
                <td>
                  <button
                    type="button"
                    className="btn"
                    onClick={() => {
                      app.setDraft({ ...app.draft, projectId: project.id });
                      app.setView("offers");
                    }}
                  >
                    Use
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
