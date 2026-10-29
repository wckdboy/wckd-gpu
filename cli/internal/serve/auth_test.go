package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyFileOwnerOnlyAndStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "serve.key")
	key, created, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created || len(key) < 16 {
		t.Fatalf("created %v len %d", created, len(key))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	again, created, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if created || again != key {
		t.Fatalf("created %v key changed", created)
	}

	bad := filepath.Join(dir, "wide.key")
	if err := os.WriteFile(bad, []byte(key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateKey(bad); err == nil {
		t.Fatal("world-readable key was accepted")
	} else if strings.Contains(err.Error(), key) {
		t.Fatalf("mode error contains the key: %v", err)
	}

	link := filepath.Join(dir, "link.key")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateKey(link); err == nil {
		t.Fatal("symlink was accepted")
	}
}

func TestTokensEqualIgnoresLengthShortcut(t *testing.T) {
	if !tokensEqual("same-token-value", "same-token-value") {
		t.Fatal("equal tokens rejected")
	}
	if tokensEqual("same-token-value", "same-token-other") {
		t.Fatal("unequal tokens accepted")
	}
	if tokensEqual("", "same-token-value") || tokensEqual("short", "same-token-value") {
		t.Fatal("different lengths matched")
	}
}

func TestListenRefusesNonLoopback(t *testing.T) {
	if _, err := Listen("0.0.0.0:0", false); err == nil {
		t.Fatal("accepted 0.0.0.0")
	}
	ln, err := Listen("127.0.0.1:0", false)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if !strings.HasPrefix(ln.Addr().String(), "127.0.0.1:") {
		t.Fatalf("addr %s", ln.Addr())
	}
}
