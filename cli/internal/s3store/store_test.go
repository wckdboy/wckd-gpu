package s3store

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wckdboy/wckd-gpu/cli/internal/config"
)

func TestPutExistsAndCheck(t *testing.T) {
	mem := map[string][]byte{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/wckd-gpu/")
		key = strings.TrimPrefix(key, "/")
		if r.URL.Path == "/wckd-gpu" || r.URL.Path == "/wckd-gpu/" {
			key = ""
		}
		switch r.Method {
		case http.MethodHead:
			if key == "" {
				w.WriteHeader(http.StatusOK)
				return
			}
			mu.Lock()
			_, ok := mem[key]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			mem[key] = body
			mu.Unlock()
			w.Header().Set("ETag", `"etag"`)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	pathStyle := true
	st, err := New(config.S3{
		Endpoint:  srv.URL,
		Bucket:    "wckd-gpu",
		AccessKey: "ak",
		SecretKey: "sk",
		Region:    "auto",
		PathStyle: &pathStyle,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := st.Check(ctx); err != nil {
		t.Fatal(err)
	}
	ok, err := st.Exists(ctx, "sessions/sess_abc/drain-ok.json")
	if err != nil || ok {
		t.Fatalf("exists before put: %v %v", ok, err)
	}
	if err := st.Put(ctx, "sessions/sess_abc/drain-ok.json", []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	ok, err = st.Exists(ctx, "sessions/sess_abc/drain-ok.json")
	if err != nil || !ok {
		t.Fatalf("exists after put: %v %v", ok, err)
	}
	mu.Lock()
	got := string(mem["sessions/sess_abc/drain-ok.json"])
	mu.Unlock()
	if got != `{"ok":true}` {
		t.Fatalf("body %q", got)
	}
}
