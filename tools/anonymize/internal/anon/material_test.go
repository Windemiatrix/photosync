package anon

import (
	"errors"
	"testing"
)

// partitionMaterial is a small consistent material with every kind of row
// the partition check looks at.
func partitionMaterial() testMaterial {
	trash := photo(5, "IMG_0005.HEIC", "2023-01-01T00:00:00+00:00", 5)
	trash.InTrash = true
	return testMaterial{
		assets: []Asset{
			photo(1, "IMG_0001.HEIC", "2023-01-01T10:00:00+00:00", 100),
			photo(2, "IMG_0002.HEIC", "2023-01-01T11:00:00+00:00", 200),
			photo(3, "IMG_0003.HEIC", "2023-01-01T12:00:00+00:00", 300),
			photo(4, "IMG_0004.HEIC", "2023-01-01T13:00:00+00:00", 400),
			trash,
			photo(6, "IMG_0006.HEIC", "2023-01-01T14:00:00+00:00", 600),
		},
		files: []File{
			cloudFile("2023/01-01 - A/IMG_0001.HEIC", "2023-01-01T10:00:00", 100),
			cloudFile("2023/01-01 - A/IMG_0002.JPG", "2023-01-01T11:00:00", 150),
			cloudFile("2023/01-01 - A/IMG_9003.JPG", "2023-01-01T12:00:00", 290),
			cloudFile("2023/01-01 - A/IMG_0004b.JPG", "", 1),
			cloudFile("2023/01-01 - A/IMG_0001.xmp", "", 1),
			cloudFile("2023/01-01 - A/notes.txt", "", 1),
			cloudFile("2023/01-01 - A/IMG_0006.JPG", "2023-01-01T14:00:00", 900),
		},
		links: Links{
			uuidN(1): {{"2023/01-01 - A/IMG_0001.HEIC", LevelExact}},
			uuidN(2): {{"2023/01-01 - A/IMG_0002.JPG", LevelSameShot}},
			uuidN(3): {{"2023/01-01 - A/IMG_9003.JPG", LevelProbable}},
			uuidN(6): {{"2023/01-01 - A/IMG_0006.JPG", LevelSameShot}},
		},
		replace: [][2]string{{uuidN(2), "2023/01-01 - A/IMG_0002.JPG"}},
		keep:    []string{"2023/01-01 - A/IMG_0006.JPG"},
	}
}

func TestCheckPartitionConsistent(t *testing.T) {
	t.Parallel()
	m := partitionMaterial().build()
	if c := CheckPartition(m); !c.Zero() {
		t.Errorf("CheckPartition = %s", c)
	}
}

func TestCheckPartitionCounters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(m *Material)
		want   Counters
	}{
		{"unknown asset", func(m *Material) { m.Links[uuidN(5)] = []Link{} }, Counters{UnknownAsset: 1}},
		{"unknown file", func(m *Material) {
			m.Links[uuidN(1)] = append(m.Links[uuidN(1)], Link{"2023/nope.jpg", LevelSameShot})
		}, Counters{UnknownFile: 1}},
		{"link to a sidecar", func(m *Material) {
			m.Links[uuidN(1)] = append(m.Links[uuidN(1)], Link{"2023/01-01 - A/IMG_0001.xmp", LevelSameShot})
		}, Counters{UnknownFile: 1}},
		{"double link", func(m *Material) {
			m.Links[uuidN(1)] = append(m.Links[uuidN(1)], m.Links[uuidN(1)][0])
		}, Counters{DoubleLink: 1}},
		{"download", func(m *Material) { m.Plan.Download = m.Plan.Download[1:] }, Counters{Download: 1}},
		{"upload", func(m *Material) {
			m.Plan.Upload = append(m.Plan.Upload, PlanUpload{Path: "2023/01-01 - A/IMG_0001.HEIC"})
		}, Counters{Upload: 1}},
		{"replace", func(m *Material) { m.Plan.Replace[0].Match = LevelSameShotWeak }, Counters{Replace: 1}},
		{"probable", func(m *Material) { m.Links[uuidN(3)][0].Kind = LevelSameShot }, Counters{Probable: 1}},
		{"keep cloud", func(m *Material) {
			m.Plan.Conflicts = append(m.Plan.Conflicts, []string{ConflictYandexBetter, "x", "2023/01-01 - A/IMG_0004b.JPG", ""})
		}, Counters{KeepCloud: 1}},
		{"short conflict", func(m *Material) {
			m.Plan.Conflicts = append(m.Plan.Conflicts, []string{ConflictYandexBetter})
		}, Counters{KeepCloud: 1}},
		{"derivation stem", func(m *Material) { m.Derived[0].Stem = "other" }, Counters{Derivation: 1}},
		{"derivation month", func(m *Material) { m.Derived[1].EventMonth = "02" }, Counters{Derivation: 1}},
		{"derivation kind", func(m *Material) { m.Files[3].Kind = KindConvert }, Counters{Derivation: 1}},
		{"duplicate asset", func(m *Material) { m.Assets = append(m.Assets, m.Assets[3]) }, Counters{Duplicate: 1}},
		{"duplicate file", func(m *Material) {
			m.Files = append(m.Files, m.Files[5])
			m.Derived = append(m.Derived, m.Derived[5])
		}, Counters{Duplicate: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := partitionMaterial().build()
			tt.mutate(m)
			if got := CheckPartition(m); got != tt.want {
				t.Errorf("CheckPartition = %s; want %s", got, tt.want)
			}
		})
	}
}

// TestDerivationNonASCIIDigits: the inventory script derives the year and the
// event month with Python's \d, which matches every decimal digit.
func TestDerivationNonASCIIDigits(t *testing.T) {
	t.Parallel()
	f := File{Path: "２０２３/０７-１４ - A/x.jpg", Kind: KindPhotosOK}
	d := Derived{Stem: "x", Folder: "２０２３/０７-１４ - A", Year: "２０２３", EventMonth: "０７"}
	if derivationDiffers(f, d) {
		t.Error("derivation of full-width digits differs from the inventory")
	}
	if !derivationDiffers(f, Derived{Stem: "x", Folder: "２０２３/０７-１４ - A"}) {
		t.Error("a missing year is not reported")
	}
}

func TestCountersString(t *testing.T) {
	t.Parallel()
	got := Counters{Download: 2, Derivation: 1}.String()
	want := "unknown_asset=0 unknown_file=0 double_link=0 download=2 upload=0 replace=0 probable=0 keep_cloud=0 derivation=1 duplicate=0"
	if got != want {
		t.Errorf("String = %q", got)
	}
}

func TestSamePlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		want bool
	}{
		{`{"download": [{"uuid": "A", "size": 1}], "upload": []}`, `{"upload":[],"download":[{"size":1,"uuid":"A"}]}`, true},
		{`{"x": 1}`, `{"x": 1.0}`, false},
		{`{"x": [1, 2]}`, `{"x": [2, 1]}`, false},
		{`{"x": "a"}`, `{"x": "b"}`, false},
	}
	for _, tt := range tests {
		got, err := SamePlan([]byte(tt.a), []byte(tt.b))
		if err != nil || got != tt.want {
			t.Errorf("SamePlan(%s, %s) = %v, %v", tt.a, tt.b, got, err)
		}
	}
	if _, err := SamePlan([]byte(`{`), []byte(`{}`)); err == nil {
		t.Error("SamePlan accepts broken JSON")
	}
	if _, err := SamePlan([]byte(`{} {}`), []byte(`{}`)); err == nil {
		t.Error("SamePlan accepts trailing data")
	}
}

func TestPickInventory(t *testing.T) {
	t.Parallel()
	good := partitionMaterial().build()
	goodInv := &Inventory{Files: good.Files, Derived: good.Derived}
	badInv := &Inventory{Files: good.Files[:3], Derived: good.Derived[:3]}
	tests := []struct {
		name     string
		invs     []*Inventory
		picked   int
		attempts int
		fail     bool
	}{
		{"current", []*Inventory{goodInv, badInv}, 0, 1, false},
		{"prev", []*Inventory{badInv, goodInv}, 1, 2, false},
		{"none", []*Inventory{badInv, badInv}, 0, 2, true},
		{"no prev", []*Inventory{badInv, nil}, 0, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := partitionMaterial().build()
			m.Files, m.Derived = nil, nil
			picked, attempts, err := PickInventory(m, func(i int) (*Inventory, error) {
				if i >= len(tt.invs) {
					return nil, nil
				}
				return tt.invs[i], nil
			})
			if (err != nil) != tt.fail || len(attempts) != tt.attempts {
				t.Fatalf("PickInventory = %d, %v, %v", picked, attempts, err)
			}
			if tt.fail {
				if !errors.Is(err, ErrPartition) {
					t.Errorf("error %v is not ErrPartition", err)
				}
				return
			}
			if picked != tt.picked || len(m.Files) != len(good.Files) {
				t.Errorf("picked %d, %d files", picked, len(m.Files))
			}
		})
	}
	boom := errors.New("boom")
	m := partitionMaterial().build()
	if _, _, err := PickInventory(m, func(int) (*Inventory, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("load error = %v", err)
	}
}

func TestPlanDecision(t *testing.T) {
	t.Parallel()
	m := partitionMaterial().build()
	pi := newPlanIndex(m.Plan)
	tests := []struct{ uuid, path, want string }{
		{uuidN(2), "2023/01-01 - A/IMG_0002.JPG", PlanReplaceDecision},
		{uuidN(1), "2023/01-01 - A/IMG_0002.JPG", ""},
		{uuidN(6), "2023/01-01 - A/IMG_0006.JPG", PlanKeepCloudDecision},
		{uuidN(1), "2023/01-01 - A/IMG_0001.HEIC", ""},
	}
	for _, tt := range tests {
		if got := pi.decision(tt.uuid, tt.path); got != tt.want {
			t.Errorf("decision(%s, %s) = %q; want %q", tt.uuid, tt.path, got, tt.want)
		}
	}
}
