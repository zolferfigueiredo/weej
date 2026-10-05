//go:build windows

package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckParsesFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"version":"1.2.3","sha256":{"x64":"abc123","x86":"def456"}}`))
	}))
	defer server.Close()

	rel, err := Check(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"x64": "abc123", "x86": "def456"}[Arch()]
	if rel.Version != "1.2.3" || rel.SHA256 != want {
		t.Errorf("Check() = %+v", rel)
	}
}

func TestCheckRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := Check(context.Background(), server.URL+"/"); err == nil {
		t.Error("want an error for a 404 feed")
	}
}

func TestCheckRejectsNonObjectFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["not", "an", "object"]`))
	}))
	defer server.Close()

	if _, err := Check(context.Background(), server.URL+"/"); err == nil {
		t.Error("want an error for a non-object feed")
	}
}
