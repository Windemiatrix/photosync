package main

import (
	"encoding/json"
	"testing"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// TestTypeErrorFieldPaths pins the field paths encoding/json reports for a
// type error (array indices included) and their translation to the known
// field names the error messages use.
func TestTypeErrorFieldPaths(t *testing.T) {
	t.Parallel()
	var inv struct {
		Files []inventoryRow `json:"files"`
	}
	var a anon.Asset
	tests := []struct {
		err  error
		want string
	}{
		{json.Unmarshal([]byte(`{"files": [{"path": "x"}, {"size": "x"}]}`), &inv), "read x: wrong type in field files.size"},
		{json.Unmarshal([]byte(`{"album_info": [{"title": "a"}, {"title": ["x"]}]}`), &a), "read x: wrong type in field album_info.title"},
		{json.Unmarshal([]byte(`{"album_info": [{"folder_names": [1]}]}`), &a), "read x: wrong type in field album_info.folder_names"},
		{json.Unmarshal([]byte(`{"original_filesize": "x"}`), &a), "read x: wrong type in field original_filesize"},
	}
	for _, tt := range tests {
		if got := translate("read x", tt.err, "default").Error(); got != tt.want {
			t.Errorf("translate = %q; want %q", got, tt.want)
		}
	}
}
