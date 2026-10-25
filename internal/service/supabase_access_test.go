package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oeggy03/call_analyzer/internal/capture"
	syncer "github.com/oeggy03/call_analyzer/internal/sync"
)

func TestCheckSupabaseAccessReportsMissingKey(t *testing.T) {
	svc := New(nil, capture.NewMockSource(), NewMemorySecretStore(), nil)
	if err := svc.CheckSupabaseAccess(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, detail := svc.SupabaseAccess()
	if status != "missing" || !strings.Contains(detail, "Add the cloud vocabulary key") {
		t.Fatalf("status=%s detail=%s", status, detail)
	}
}

func TestSetSupabaseSecretProbesReadAndWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	secrets := NewMemorySecretStore()
	svc := New(nil, capture.NewMockSource(), secrets, nil)
	svc.supabaseBaseURL = server.URL
	svc.supabaseHTTP = server.Client()
	if err := svc.SetSupabaseSecret(context.Background(), "fixture-secret"); err != nil {
		t.Fatal(err)
	}
	status, detail := svc.SupabaseAccess()
	if status != "ready" {
		t.Fatalf("status=%s detail=%s", status, detail)
	}
	stored, err := secrets.Get(context.Background(), syncer.SupabaseSecretName)
	if err != nil || stored != "fixture-secret" {
		t.Fatalf("stored %q err %v", stored, err)
	}
}

func TestSetSupabaseSecretRejectsUnauthorizedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, `{"message":"bad"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	secrets := NewMemorySecretStore()
	svc := New(nil, capture.NewMockSource(), secrets, nil)
	svc.supabaseBaseURL = server.URL
	svc.supabaseHTTP = server.Client()
	if err := svc.SetSupabaseSecret(context.Background(), "fixture-secret"); err != nil {
		t.Fatal(err)
	}
	status, detail := svc.SupabaseAccess()
	if status != "invalid" || !strings.Contains(detail, "cannot read and write") {
		t.Fatalf("status=%s detail=%s", status, detail)
	}
}
