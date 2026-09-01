package control

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveryConsoleKeepsBearerTokenOutOfBrowserState(t *testing.T) {
	const token = "sensitive-recovery-token"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("recovery-ok"))
	}))
	defer upstream.Close()

	console, err := StartRecoveryConsole(upstream.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.Get(console.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "recovery-ok" {
		t.Fatalf("status = %d, body = %q", response.StatusCode, body)
	}
	if strings.Contains(console.URL, token) {
		t.Fatal("recovery token leaked into console URL")
	}
	for _, cookie := range jar.Cookies(response.Request.URL) {
		if strings.Contains(cookie.Value, token) {
			t.Fatal("recovery token leaked into browser cookie")
		}
	}
}

func TestRecoveryConsoleRejectsRequestsWithoutCapabilityCookie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer upstream.Close()
	console, err := StartRecoveryConsole(upstream.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()

	base := console.URL[:strings.Index(console.URL, "/session/")]
	response, err := http.Get(base + "/api/v1/recovery/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
}

func TestRecoveryConsoleRejectsNonLoopbackUpstream(t *testing.T) {
	if _, err := StartRecoveryConsole("https://recovery.example.test", "token"); err == nil || !strings.Contains(err.Error(), "loopback-only") {
		t.Fatalf("error = %v", err)
	}
}
