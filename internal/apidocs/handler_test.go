package apidocs

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/trestle-dev/trestle/internal/storetest"
)

// TestSchemaReportsFieldFlagsOnBothProviders ensures the API schema reports
// required/unique from the stored field metadata on SQLite and PostgreSQL.
// Required and unique are stored as engine-native booleans, so reading them
// must go through the dialect rather than assuming an integer column type.
func TestSchemaReportsFieldFlagsOnBothProviders(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			db := storetest.Open(t, provider)
			exec := db.DB()
			if _, err := exec.ExecContext(context.Background(), "INSERT INTO _trestle_collections(id,name,kind,created_at,updated_at) VALUES('col_1','issues','base','t','t')"); err != nil {
				t.Fatal(err)
			}
			one := db.Dialect().Boolean(true)
			if _, err := exec.ExecContext(context.Background(), "INSERT INTO _trestle_fields(id,collection_id,position,name,type,required,is_unique,default_json,created_at) VALUES('fld_1','col_1',0,'status','text',?,?,NULL,'t')", one, one); err != nil {
				t.Fatal(err)
			}
			h := New(db.DB(), nil)
			w := httptest.NewRecorder()
			h.schema(w, httptest.NewRequest("GET", "/admin/v1/api/schema", nil))
			if w.Code != 200 {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			var out struct {
				Collections map[string][]map[string]any `json:"collections"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			fields := out.Collections["issues"]
			if len(fields) != 1 {
				t.Fatalf("fields=%v", fields)
			}
			if fields[0]["required"] != true || fields[0]["unique"] != true {
				t.Fatalf("provider %s required=%v unique=%v", provider, fields[0]["required"], fields[0]["unique"])
			}
		})
	}
}
