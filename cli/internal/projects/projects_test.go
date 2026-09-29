package projects

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAddIsIdempotentAndOmitsSecrets(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	rec, created, err := Add(dir, "hailuo-tests", now)
	if err != nil {
		t.Fatal(err)
	}
	if !created || rec.ID != "hailuo-tests" {
		t.Fatalf("created %#v %v", rec, created)
	}
	again, created, err := Add(dir, "hailuo-tests", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created || !again.CreatedAt.Equal(now) {
		t.Fatalf("second add %#v %v", again, created)
	}
	body, err := os.ReadFile(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 || string(body[len(body)-1]) != "\n" {
		t.Fatalf("file %q", body)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != "hailuo-tests" {
		t.Fatalf("loaded %#v", loaded)
	}
}
