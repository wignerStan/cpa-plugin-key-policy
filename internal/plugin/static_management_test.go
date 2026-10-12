package plugin

import (
	"net/http"
	"path/filepath"
	"testing"
)

func TestStaticPolicyManagementRejectsWrites(t *testing.T) {
	app := configureSettingsApp(t, filepath.Join(t.TempDir(), "state.json"))
	for _, route := range []string{"keys", "keys/rotate", "settings", "aliases", "classify-rules", "classify-rules/reorder"} {
		for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
			r := callManagementForTest(t, app, method, "/v0/management/plugins/cpa-key-policy/"+route, []byte(`{}`))
			if r.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s: %d", method, route, r.StatusCode)
			}
		}
	}
	r := callManagementForTest(t, app, http.MethodGet, "/v0/management/plugins/cpa-key-policy/keys", nil)
	if r.StatusCode != http.StatusOK {
		t.Fatal("read-only management unavailable")
	}
}
