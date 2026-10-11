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

func TestScopedTokenWithV1ContainerAndRegistry(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/auth/api/token" {
			w.Write([]byte(`{"access_token":"scoped-api-token"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer scoped-api-token" {
			t.Error("missing scoped token")
		}
		switch r.URL.Path {
		case "/service/container-instance/containers/app":
			if r.Method != "GET" {
				t.Error("wrong container method")
			}
			w.Write([]byte(`{"data":{"container":{"name":"app","status":"running"}}}`))
		case "/service/container-instance/containers/app/image":
			if r.Method != "PATCH" {
				t.Error("legacy update method")
			}
			var body V1PatchImageRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Image != "registry.example/app:sha" || body.Credentials != nil {
				t.Error("wrong v1 image payload")
			}
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"data":{"operation":{"id":"operation-1"}}}`))
		case "/service/container-registry/credentials/registry":
			if r.Header.Get("X-Organization-ID") != "org-a" {
				t.Error("registry lost organization header")
			}
			w.Write([]byte(`{"success":true,"data":{"registryUrl":"registry.example","username":"u","password":"p"}}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, server.URL)
	client.httpClient = server.Client()
	token, err := client.Login("u", "p", "org-a")
	if err != nil {
		t.Fatal(err)
	}
	container, err := client.GetContainer(token, "org-a", "app")
	if err != nil || container.Name != "app" {
		t.Fatalf("v1 detail: %v", err)
	}
	id, err := client.PatchContainerImage(token, "org-a", "app", RegistryConfig{Type: "tower", FullImage: "registry.example/app:sha"})
	if err != nil || id != "operation-1" {
		t.Fatalf("v1 operation: %v", err)
	}
	registry, _, _, err := client.GetRegistryCredentials(token, "org-a", "registry")
	if err != nil || registry != "registry.example" {
		t.Fatalf("registry credentials: %v", err)
	}
}
