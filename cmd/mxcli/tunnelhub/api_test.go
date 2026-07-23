// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestAPI(t *testing.T, secret string) (*API, *Registry) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := newTestRegistry(clk)
	api := NewAPI(APIOptions{
		Registry:       reg,
		ControlURL:     "https://hub.mxcli.org",
		RegisterSecret: secret,
	})
	return api, reg
}

func doJSON(t *testing.T, api *API, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	api.Mount(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAPI_RegisterAndBackends(t *testing.T) {
	api, _ := newTestAPI(t, "")
	rec := doJSON(t, api, http.MethodPost, "/api/register",
		`{"project":"MyApp","solution":"Sol","branch":"feature/x","appPort":8080}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body %s", rec.Code, rec.Body)
	}
	var resp RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.URL != "https://myapp-feature-x.mxcli.org" {
		t.Errorf("url = %q", resp.URL)
	}
	if resp.ReversePort == 0 || resp.Token == "" || resp.ControlURL != "https://hub.mxcli.org" {
		t.Errorf("incomplete response: %+v", resp)
	}

	// backends lists it.
	lb := doJSON(t, api, http.MethodGet, "/api/backends?sort=project", "", nil)
	var views []BackendView
	if err := json.Unmarshal(lb.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Project != "MyApp" || views[0].Availability != Available {
		t.Errorf("backends = %+v", views)
	}
}

func TestAPI_RegisterRequiresProject(t *testing.T) {
	api, _ := newTestAPI(t, "")
	rec := doJSON(t, api, http.MethodPost, "/api/register", `{"branch":"main"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPI_RegisterSecretGate(t *testing.T) {
	api, _ := newTestAPI(t, "s3cret")
	// missing secret -> 401
	if rec := doJSON(t, api, http.MethodPost, "/api/register", `{"project":"A"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no secret: status = %d, want 401", rec.Code)
	}
	// correct secret -> 200
	rec := doJSON(t, api, http.MethodPost, "/api/register", `{"project":"A"}`,
		map[string]string{"X-Hub-Secret": "s3cret"})
	if rec.Code != http.StatusOK {
		t.Errorf("with secret: status = %d, want 200", rec.Code)
	}
}

func TestAPI_HeartbeatAndDeregister(t *testing.T) {
	api, reg := newTestAPI(t, "")
	var resp RegisterResponse
	rec := doJSON(t, api, http.MethodPost, "/api/register", `{"project":"A","branch":"main"}`, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	// heartbeat with the token succeeds.
	hb := doJSON(t, api, http.MethodPost, "/api/status", "",
		map[string]string{"Authorization": "Bearer " + resp.Token})
	if hb.Code != http.StatusNoContent {
		t.Errorf("heartbeat status = %d, want 204", hb.Code)
	}
	// bad token -> 404.
	bad := doJSON(t, api, http.MethodPost, "/api/status", "",
		map[string]string{"Authorization": "Bearer nope"})
	if bad.Code != http.StatusNotFound {
		t.Errorf("bad-token heartbeat status = %d, want 404", bad.Code)
	}
	// missing token -> 401.
	if none := doJSON(t, api, http.MethodPost, "/api/status", "", nil); none.Code != http.StatusUnauthorized {
		t.Errorf("no-token heartbeat status = %d, want 401", none.Code)
	}
	// deregister removes it.
	dr := doJSON(t, api, http.MethodPost, "/api/deregister", "",
		map[string]string{"Authorization": "Bearer " + resp.Token})
	if dr.Code != http.StatusNoContent {
		t.Errorf("deregister status = %d, want 204", dr.Code)
	}
	if len(reg.List("used")) != 0 {
		t.Error("backend should be gone after deregister")
	}
}

func TestAPI_RegisterMethodNotAllowed(t *testing.T) {
	api, _ := newTestAPI(t, "")
	if rec := doJSON(t, api, http.MethodGet, "/api/register", "", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET register status = %d, want 405", rec.Code)
	}
}
