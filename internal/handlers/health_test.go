package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestLiveAdvertisesSyncProtocol(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := (&HealthHandler{}).Live(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	var body struct {
		Status   string `json:"status"`
		Protocol int    `json:"protocol"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if SyncProtocol != 3 {
		t.Fatalf("SyncProtocol = %d, want 3", SyncProtocol)
	}
	if body.Status != "ok" || body.Protocol != SyncProtocol {
		t.Fatalf("body %+v", body)
	}
}
