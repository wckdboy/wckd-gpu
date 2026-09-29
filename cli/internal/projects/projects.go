// Package projects stores local project ids. An id is the S3 prefix segment
// projects/<id>/ that `wckd start --project` already accepts. This list does
// not create the prefix; start still writes project.json on the bucket.
package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Record is one saved project id.
type Record struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type fileDoc struct {
	Projects []Record `json:"projects"`
}

// Load reads stateDir/projects.json. A missing file is an empty list.
func Load(stateDir string) ([]Record, error) {
	body, err := os.ReadFile(path(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc fileDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse projects: %w", err)
	}
	return doc.Projects, nil
}

// Add inserts id if it is not already saved. created is false when the id
// was already on disk. The caller validates the id.
func Add(stateDir, id string, now time.Time) (Record, bool, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return Record{}, false, err
	}
	current, err := Load(stateDir)
	if err != nil {
		return Record{}, false, err
	}
	for _, rec := range current {
		if rec.ID == id {
			return rec, false, nil
		}
	}
	rec := Record{ID: id, CreatedAt: now.UTC()}
	current = append(current, rec)
	body, err := json.MarshalIndent(fileDoc{Projects: current}, "", "  ")
	if err != nil {
		return Record{}, false, err
	}
	body = append(body, '\n')
	tmp := path(stateDir) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return Record{}, false, err
	}
	if err := os.Rename(tmp, path(stateDir)); err != nil {
		return Record{}, false, err
	}
	return rec, true, nil
}

func path(stateDir string) string {
	return filepath.Join(stateDir, "projects.json")
}
