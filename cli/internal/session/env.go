package session

import (
	"strconv"
	"strings"

	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
)

// StorageCreds are copied into the pod environment so the sidecar can run rclone.
// P1 has no short-lived token broker; the user's key is injected for the session.
type StorageCreds struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
}

// PodEnv builds the environment for the sidecar. Preset env is applied first.
// WCKD_* identity and storage keys are then overwritten so a preset cannot drop them.
func PodEnv(sess Session, p preset.File, st StorageCreds) map[string]string {
	env := map[string]string{}
	for k, v := range p.Spec.Runtime.Env {
		env[k] = v
	}
	hydrate, drain, excludes := SyncMaps(p, sess.ProjectID)
	env["WCKD_SESSION_ID"] = sess.ID
	env["WCKD_PROJECT_ID"] = sess.ProjectID
	env["WCKD_PRESET_ID"] = sess.PresetID
	env["WCKD_DEADLINE_AT"] = sess.DeadlineAt.UTC().Format("2006-01-02T15:04:05Z")
	env["WCKD_S3_ENDPOINT"] = st.Endpoint
	env["WCKD_S3_BUCKET"] = st.Bucket
	env["WCKD_S3_ACCESS_KEY_ID"] = st.AccessKey
	env["WCKD_S3_SECRET_ACCESS_KEY"] = st.SecretKey
	env["WCKD_S3_REGION"] = st.Region
	env["WCKD_HYDRATE_MAP"] = hydrate
	env["WCKD_DRAIN_MAP"] = drain
	env["WCKD_DRAIN_EXCLUDES"] = excludes
	env["WCKD_WORKSPACE"] = workspace(p)
	if cmd := shellJoin(p.Spec.Runtime.Entrypoint); cmd != "" {
		env["WCKD_WORKLOAD"] = cmd
	}
	return env
}

// SyncMaps encodes hydrate and drain pairs for the sidecar.
// Hydrate pairs are "s3-prefix|container-path". Drain pairs are "container-path|s3-prefix".
// Pairs are separated by ";;".
func SyncMaps(p preset.File, projectID string) (hydrate, drain, excludes string) {
	var h []string
	for _, path := range p.Spec.Sync.Hydrate {
		from := path.From
		to := path.To
		if from == "" {
			from = "workspace/"
		}
		if to == "" {
			to = "/workspace"
		}
		h = append(h, s3Prefix(projectID, from)+"|"+to)
	}
	if len(h) == 0 {
		h = append(h, s3Prefix(projectID, "workspace/")+"|/workspace")
	}
	var d []string
	var ex []string
	for _, path := range p.Spec.Sync.Drain {
		from := path.From
		to := path.To
		if from == "" {
			from = "/workspace"
		}
		if to == "" {
			to = "workspace/"
		}
		d = append(d, from+"|"+s3Prefix(projectID, to))
		ex = append(ex, path.Exclude...)
	}
	if len(d) == 0 {
		d = append(d, "/workspace|"+s3Prefix(projectID, "workspace/"))
	}
	return strings.Join(h, ";;"), strings.Join(d, ";;"), strings.Join(ex, ",")
}

func workspace(p preset.File) string {
	if len(p.Spec.Sync.Hydrate) > 0 && p.Spec.Sync.Hydrate[0].To != "" {
		return p.Spec.Sync.Hydrate[0].To
	}
	return "/workspace"
}

func s3Prefix(projectID, rel string) string {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return "projects/" + projectID
	}
	return "projects/" + projectID + "/" + rel
}

func shellJoin(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = strconv.Quote(a)
	}
	return strings.Join(parts, " ")
}
