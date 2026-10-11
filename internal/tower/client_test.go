package tower

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScopedLogin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/api/token" || r.Method != "POST" {
			t.Errorf("unexpected login route %s", r.URL.Path)
		}
		var body LoginRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.OrganizationID != "org-a" || body.Username != "user" || body.Password != "password" {
			t.Error("wrong scoped login payload")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"scoped-api-token","expires_in":120,"token_type":"Bearer"}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, server.URL)
	client.httpClient = server.Client()
	token, err := client.Login("user", "password", "org-a")
	if err != nil || token != "scoped-api-token" {
		t.Fatalf("scoped login failed: %v", err)
	}
}
func TestLoginDoesNotLeakResponse(t *testing.T) {
	for _, status := range []int{200, 401, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"secret":"do-not-log-me"}`))
			}))
			defer server.Close()
			client := NewClient(server.URL, server.URL)
			client.httpClient = server.Client()
			_, err := client.Login("u", "p", "o")
			if err == nil || strings.Contains(err.Error(), "do-not-log-me") {
				t.Fatalf("unsafe response error: %v", err)
			}
		})
	}
}
func TestLoginRequiresHTTPS(t *testing.T) {
	_, err := NewClient("https://api.example", "http://auth.example").Login("u", "p", "o")
	if err == nil {
		t.Fatal("plaintext login accepted")
	}
}
func TestLoginDoesNotFollowRedirects(t *testing.T) {
	var called bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	client := NewClient(server.URL, server.URL)
	client.httpClient.Transport = server.Client().Transport
	_, err := client.Login("u", "p", "o")
	if err == nil || called {
		t.Fatal("login redirect followed")
	}
}
