package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// The synthetic corpus looks like personal material: invented people and
// places, example.com addresses, local paths, coordinates of well-known
// squares, a login, descriptions, keywords and faces. Every value is made up.
// Links and the plan are driven by one table: the tool does no matching, so
// the corpus needs consistency, not a correct matching algorithm.

// corpusAsset is one catalog record of the table with its links.
type corpusAsset struct {
	a     anon.Asset
	links []corpusLink
}

type corpusLink struct {
	path, kind string
	plan       string // "", replace, keep_cloud
}

// corpusOptions selects a corpus variant.
type corpusOptions struct {
	// prev writes the full inventory as yandex_inventory.prev.json and a
	// current inventory without one row, which fails the partition check.
	prev bool
	// rerunDiffers writes a rerun plan that differs from the reviewed plan.
	rerunDiffers bool
	// noPrevMatch writes a previous inventory that fails too.
	noPrevMatch bool
}

// corpus describes a written corpus.
type corpus struct {
	canaries []string // strings that must never reach any output
	read     []string // file names the tool opens, sorted
	ignored  []string // file names the tool never opens, sorted
	uuids    []string // all UUIDs and other identifiers of the input
	sha1s    []string
	years    []string // years of the corpus dates except 1970
}

// cid returns the invented UUID of corpus asset n (two digits).
func cid(n string) string { return "5E1F" + n + "AB-0000-4000-8000-0000000000" + n }

var canaryRE = regexp.MustCompile(`Kanareyka[A-Za-z]+`)

func asset(n, name, date string, size int64) anon.Asset {
	u := cid(n)
	return anon.Asset{UUID: u, OriginalFilename: name, Filename: u + ".heic", Date: date, TZOffset: 10800,
		OriginalFilesize: size, OriginalWidth: 4032, OriginalHeight: 3024, Width: 4032, Height: 3024, Albums: []anon.AlbumRef{}}
}

func album(folders []string, title string) anon.AlbumRef {
	return anon.AlbumRef{FolderNames: folders, Title: title}
}

func cfile(path, date string, size int64) anon.File {
	return anon.File{Path: path, Kind: anon.KindOf(anon.BaseName(path)), Size: size, DateTaken: date, Width: 4032, Height: 3024}
}

// corpusTable returns the catalog and the inventory of the corpus.
func corpusTable() ([]corpusAsset, []anon.File) {
	var as []corpusAsset
	var fs []anon.File
	add := func(a anon.Asset, links ...corpusLink) { as = append(as, corpusAsset{a, links}) }
	file := func(f anon.File) { fs = append(fs, f) }
	link := func(path, kind string) corpusLink { return corpusLink{path: path, kind: kind} }

	// A Live photo: the photo matches exactly, the MOV is its component.
	a := asset("01", "IMG_4821.HEIC", "2023-07-14T18:03:27.455000+03:00", 2734118)
	a.LivePhoto = true
	a.Albums = []anon.AlbumRef{album([]string{"2023"}, "07/14 Мырдон")}
	f := cfile("2023/07-14 - Мырдон/IMG_4821.HEIC", "2023-07-14T18:03:27.455", 2734118)
	f.ContentID, f.LivePair, f.SHA1 = "7A1C0D2E-3F40-4B5C-8D6E-7F8091A2B3C4", anon.LivePairCID, "3b18e512dba79e4c8300dd08aeb37f8e728b8dad"
	file(f)
	f = cfile("2023/07-14 - Мырдон/IMG_4821.MOV", "2023-07-14T18:03:27", 3100000)
	f.ContentID, f.LivePair = "7A1C0D2E-3F40-4B5C-8D6E-7F8091A2B3C4", anon.LivePairCID
	file(f)
	add(a, link("2023/07-14 - Мырдон/IMG_4821.HEIC", anon.LevelExact), link("2023/07-14 - Мырдон/IMG_4821.MOV", anon.RefLiveComponent))

	// A derived JPEG copy on the cloud, the original in Photos: replace.
	a = asset("02", "IMG_0102.HEIC", "2022-12-31T23:30:00+03:00", 3000000)
	a.Albums = []anon.AlbumRef{album([]string{"2022"}, "12/31 Новый год у Пельменевых")}
	file(cfile("2022/12-31 - Новый год у Пельменевых/IMG_0102.JPG", "2022-12-31T23:30:00", 1200000))
	add(a, corpusLink{"2022/12-31 - Новый год у Пельменевых/IMG_0102.JPG", anon.LevelSameShot, anon.PlanReplaceDecision})

	// The cloud copy is better: keep it.
	a = asset("03", "IMG_0103.JPG", "2021-06-05T10:00:00+03:00", 500000)
	file(cfile("2021/06 - Zabludovo/IMG_0103.jpg", "2021-06-05T10:00:00.250", 900000))
	add(a, corpusLink{"2021/06 - Zabludovo/IMG_0103.jpg", anon.LevelSameShot, anon.PlanKeepCloudDecision})

	// No date on the cloud file: the year and month of the folder.
	a = asset("04", "DSC_0001.JPG", "2019-08-10T12:00:00+03:00", 4100000)
	a.Albums = []anon.AlbumRef{album([]string{"2019"}, "08 Отпуск в Кукуево")}
	file(cfile("2019/08 - Отпуск в Кукуево/DSC_0001.JPG", "", 4000000))
	add(a, link("2019/08 - Отпуск в Кукуево/DSC_0001.JPG", anon.LevelSameShotWeak))

	// Renamed on import: the same second and size (Android name).
	a = asset("05", "IMG_5000.HEIC", "2020-05-05T10:10:10+03:00", 2000000)
	file(cfile("2020/05-05/20200505_101010.jpg", "2020-05-05T10:10:10", 2000000))
	add(a, link("2020/05-05/20200505_101010.jpg", anon.LevelSameTimeSize))

	// The same second, another size: probable.
	a = asset("06", "IMG_6000.HEIC", "2020-05-06T11:11:11+03:00", 2500000)
	file(cfile("2020/05-06 - Пикник у Зинаиды/PXL_20200506_081111123.jpg", "2020-05-06T11:11:11", 1000000))
	add(a, link("2020/05-06 - Пикник у Зинаиды/PXL_20200506_081111123.jpg", anon.LevelProbable))

	// A video with a time zone shift of three hours.
	a = asset("07", "IMG_7000.MOV", "2018-03-01T10:00:00+03:00", 52000000)
	a.IsMovie = true
	file(cfile("2018/03-01 - Видео с катка/IMG_7000.MOV", "2018-03-01T07:00:00", 52000000))
	add(a, link("2018/03-01 - Видео с катка/IMG_7000.MOV", anon.LevelSameShot))

	// A Pixel photo dated 1970: the date is fixed from the name.
	a = asset("08", "PXL_20210305_101112345.jpg", "1970-01-01T03:00:00+03:00", 3300000)
	file(cfile("2021/03-05/PXL_20210305_101112345.jpg", "2021-03-05T13:11:12", 3200000))
	add(a, link("2021/03-05/PXL_20210305_101112345.jpg", anon.LevelSameShot))

	// One photo in two cloud folders with the same content.
	a = asset("09", "IMG_9000.HEIC", "2017-02-02T09:00:00+03:00", 2900000)
	f = cfile("2017/02-02 - Каток/IMG_9000.HEIC", "2017-02-02T09:00:00", 2900000)
	f.SHA1 = "aa2c4f3e5b6d7081920a1b2c3d4e5f60718293a4"
	file(f)
	f = cfile("Избранное/Лучшее у Пельменевых/IMG_9000.HEIC", "2017-02-02T09:00:00", 2900000)
	f.SHA1 = "aa2c4f3e5b6d7081920a1b2c3d4e5f60718293a4"
	file(f)
	add(a, link("2017/02-02 - Каток/IMG_9000.HEIC", anon.LevelExact), link("Избранное/Лучшее у Пельменевых/IMG_9000.HEIC", anon.LevelExact))

	// Only in Photos, in two albums with "/" in the title.
	a = asset("10", "IMG_1000.HEIC", "2016-09-01T08:30:00+03:00", 2100000)
	a.Albums = []anon.AlbumRef{album([]string{"2016"}, "09/01 Школа"), album([]string{}, "Семья / Дети / Зинаида")}
	add(a)

	// Only on the cloud, next to a file of another kind.
	file(cfile("2015/04-04 - Мырдон/IMG_1500.JPG", "2015-04-04T15:00:00", 1500000))
	file(cfile("2015/04-04 - Мырдон/readme.txt", "", 120))
	file(cfile("2015/04-04 - МЫРДОН/img_1501.jpg", "2015-04-04T15:05:00", 1500001))

	// RAW and JPEG of one shot on the cloud; a DNG in Photos.
	file(cfile("2014/05 - Горы Кавказа/IMG_1200.CR2", "2014-05-10T06:00:00", 25000000))
	file(cfile("2014/05 - Горы Кавказа/IMG_1200.JPG", "2014-05-10T06:00:00", 6000000))
	a = asset("13", "IMG_1300.DNG", "2014-06-01T07:00:00+03:00", 30000000)
	a.IsRaw = true
	file(cfile("2014/06-01/IMG_1300.DNG", "2014-06-01T07:00:00", 30000000))
	add(a, link("2014/06-01/IMG_1300.DNG", anon.LevelExact))
	a = asset("12", "IMG_1250.HEIC", "2014-06-02T07:00:00+03:00", 3000000)
	a.HasRaw = true
	add(a)

	// A burst of three frames in the osxphotos shape: burst_key on the first,
	// burst_photos with the UUIDs of the other frames. The third frame is in
	// the trash: its UUID stays in burst_photos of the others.
	var frames []anon.Asset
	for i, n := range []string{"14", "15", "41"} {
		frames = append(frames, asset(n, fmt.Sprintf("IMG_140%d.JPG", i+1), fmt.Sprintf("2013-07-07T07:07:07.%d00000+03:00", 1+2*i), int64(1400000+i)))
	}
	for i := range frames {
		frames[i].Burst, frames[i].BurstKeyPhoto, frames[i].BurstPhotos = true, i == 0, []string{}
		for j := range frames {
			if j != i {
				frames[i].BurstPhotos = append(frames[i].BurstPhotos, frames[j].UUID)
			}
		}
	}
	frames[2].InTrash = true
	for _, f := range frames {
		add(f)
	}

	// The same name in different years.
	add(asset("16", "IMG_0001.HEIC", "2012-05-05T05:05:05+04:00", 1100000))
	a = asset("17", "IMG_0001.HEIC", "2024-01-01T00:10:00+03:00", 1200000)
	file(cfile("2024/01-01 - Ёлка у Зинаиды/IMG_0001.HEIC", "2024-01-01T00:10:00", 1200000))
	add(a, link("2024/01-01 - Ёлка у Зинаиды/IMG_0001.HEIC", anon.LevelExact))

	// Duplicates in Photos: same name and size.
	add(asset("18", "IMG_1900.JPG", "2011-01-11T11:11:11+03:00", 1234567))
	add(asset("19", "img_1900.jpg", "2011-02-12T12:12:12+03:00", 1234567))

	// A " (1)" copy next to its original.
	a = asset("20", "IMG_1100.JPG", "2011-10-10T10:10:10+03:00", 1100000)
	file(cfile("2011/10-10 - Дача Пельменевых/IMG_1100 (1).JPG", "2011-10-10T10:10:10", 1100000))
	file(cfile("2011/10-10 - Дача Пельменевых/IMG_1100.JPG", "2011-10-10T10:10:10", 1100000))
	add(a, link("2011/10-10 - Дача Пельменевых/IMG_1100.JPG", anon.LevelSameShot))

	// Non-ASCII names.
	file(cfile("2010/11-11 - Зимний лес/Фото_2010.jpg", "2010-11-11T11:00:00", 900000))
	add(asset("21", "Снимок экрана Зинаиды.png", "2010-11-12T12:00:00+03:00", 300000))

	// Edited in Photos; an edited copy and a sidecar on the cloud.
	a = asset("22", "IMG_2200.HEIC", "2009-09-09T09:09:09+04:00", 2200000)
	a.HasAdjustments = true
	file(cfile("2009/09-09/IMG_2200.HEIC", "2009-09-09T09:09:09", 2200000))
	file(cfile("2009/09-09/IMG_2200_edited.JPG", "2009-09-09T09:09:09", 1700000))
	file(cfile("2009/09-09/IMG_2200.xmp", "", 4000))
	file(cfile("2008/IMG_0800.AAE", "", 2000))
	add(a, link("2009/09-09/IMG_2200.HEIC", anon.LevelExact))

	// Videos Photos cannot read.
	file(cfile("2007/12-25 - Рождество/MVI_0001.AVI", "2007-12-25T12:00:00", 80000000))
	f = cfile("2007/12-25 - Рождество/VID_20071225_120000.mts", "2007-12-25T12:00:00", 81000000)
	f.LivePair = anon.LivePairName
	file(f)

	// Not supported: an e-mail in a name, a year out of range, a decomposed
	// letter, a coordinate in a title, an event folder with a year, a year
	// folder out of range. The e-mail asset takes its supported linked file
	// and the neighbor of that file out of every case.
	file(cfile("2006/06-06 - KanareykaUnit/IMG_2300.JPG", "2006-06-06T06:06:06", 600000))
	file(cfile("2006/06-06 - KanareykaUnit/IMG_2300 (1).JPG", "", 600001))
	add(asset("23", "zinaida.pelmeneva@example.com.jpg", "2006-06-06T06:06:06+03:00", 600000),
		link("2006/06-06 - KanareykaUnit/IMG_2300.JPG", anon.LevelSameShot))
	add(asset("24", "IMG_2400.JPG", "2031-01-01T00:00:00+03:00", 240000))
	a = asset("25", "IMG_2500.JPG", "2005-05-05T05:05:05+03:00", 250000)
	a.Albums = []anon.AlbumRef{album([]string{}, "Е\u0308лка")}
	add(a)
	a = asset("26", "IMG_2600.JPG", "2004-04-04T04:04:04+03:00", 260000)
	a.Albums = []anon.AlbumRef{album([]string{}, "Площадь 55.7539")}
	add(a)
	file(cfile("2019/2019 Отпуск/IMG_0019.jpg", "2019-07-01T10:00:00", 190000))
	file(cfile("2031/IMG_2031.jpg", "", 203100))

	// A title with a four-digit number and the hierarchy.
	a = asset("27", "IMG_2700.JPG", "2003-03-03T03:03:03+03:00", 270000)
	a.Albums = []anon.AlbumRef{album([]string{"Путешествия"}, "Поездка 2019 / Казань")}
	add(a)

	// In the trash: never in a case.
	a = asset("28", "IMG_2800.JPG", "2002-02-02T02:02:02+03:00", 280000)
	a.InTrash = true
	add(a)

	// One record for each remaining rule of "not supported".
	titled := func(n, title string) {
		a := asset(n, "IMG_"+n+"00.JPG", "2001-01-01T01:01:01+03:00", 100000)
		a.Albums = []anon.AlbumRef{album([]string{}, title)}
		add(a)
	}
	titled("29", "Высота 2000° у Мырдоновых")
	titled("30", "Пикник\tу Зинаиды")
	titled("33", "/Users/zinaida/Фото")
	titled("34", `D:/Фото Зинаиды`)
	titled("38", "İzmir с Зинаидой")
	add(asset("31", "\u0301Зинаида.jpg", "2001-01-01T01:01:01+03:00", 310000))
	add(asset("32", "IMG_\u0661\u0662\u0663.jpg", "2001-01-01T01:01:01+03:00", 320000))
	add(asset("35", "IMG_3500.JPG", "2001:01:01 01:01:01", 350000))
	add(asset("36", "PXL_20211305_101112.jpg", "1970-01-01T03:00:00+03:00", 360000))
	add(asset("37", "IMG_3700.JPG", "2001-01-01T01:01:01+03:00", -1))
	add(asset("40", "IMG_4000.JPG", "1970-12-31T20:00:00+03:00", 400000))
	// An epoch date far from 1971 on the cloud: supported, kept as it is.
	file(cfile("1970/01-02/IMG_4100.JPG", "1970-01-02T00:00:00", 410000))
	// Hangul jamo compose under NFC: supported only without normalization.
	add(asset("39", "\u1100\u1161\u1102\u1161.jpg", "2001-02-02T02:02:02+03:00", 390000))
	// A decomposed file name: supported only with normalization.
	file(cfile("2012/03-03 - Зимний лес/Е\u0308лка у дома.jpg", "2012-03-03T10:00:00", 120000))
	return as, fs
}

// personal fields of the catalog and the inventory that are never decoded.
func catalogExtras(n int) map[string]any {
	return map[string]any{
		"latitude":    55.7539 + float64(n)/1e4,
		"longitude":   37.6208,
		"place":       map[string]any{"name": "Красная площадь, Москва", "address": "Москва, Россия", "country_code": "RU"},
		"persons":     []string{"Зинаида Пельменева", "Arkadiy Myrdonov"},
		"face_info":   []map[string]any{{"name": "Зинаида Пельменева", "center_x": 0.4171, "uuid": "F00DFACE-1111-4222-8333-444455556666"}},
		"exif_info":   map[string]any{"camera_make": "Apple", "camera_model": "iPhone 13 Pro Zinaidy", "lens_model": "back triple camera"},
		"description": "Встреча у Мырдоновых на даче, Пельменево",
		"title":       "Прогулка Зинаиды",
		"keywords":    []string{"дача Пельменевых", "Мырдоновка"},
		"labels":      []string{"Sky", "Plant"},
		"comments":    []map[string]any{{"user": "Arkadiy Myrdonov", "text": "Отличное фото!"}},
		"owner":       "zinaida_p",
		"library":     "/Users/zinaida.pelmeneva/Pictures/Photos Library.photoslibrary",
		"path":        "/Users/zinaida.pelmeneva/Pictures/Photos Library.photoslibrary/originals/A/x.heic",
		"cloud_guid":  "AYQ3bXJkb25vdmFfc2VjcmV0",
		"fingerprint": "AWJjZGVmZ2hpamtsbW5vcA",
		"score":       map[string]any{"overall": 0.8813},
		"search_info": map[string]any{"venue_names": []string{"Кафе Пельмешка"}},
		"albums":      []string{"Мырдоновка 2019"},
	}
}

// writeCorpus writes the corpus files into dir.
func writeCorpus(t *testing.T, dir string) corpus { return writeCorpusWith(t, dir, corpusOptions{}) }

func writeCorpusWith(t *testing.T, dir string, opt corpusOptions) corpus {
	t.Helper()
	as, fs := corpusTable()
	c := corpus{canaries: []string{
		"Зинаида Пельменева", "Arkadiy Myrdonov", "Красная площадь", "55.7539", "37.6208", "zinaida_p",
		"zinaida.pelmeneva", "/Users/zinaida", `C:\Users\Zinaida`, "iPhone 13 Pro Zinaidy", "Мырдоновых", "Пельменево",
		"Кафе Пельмешка", "AYQ3bXJkb25vdmFfc2VjcmV0", "Canon EOS 5D Zinaidy", "Мырдоновка", "Отличное фото",
		"KanareykaUnit",
	}}
	years := map[string]bool{}
	var catalog []map[string]any
	for i, ca := range as {
		m := catalogExtras(i)
		raw, err := json.Marshal(ca.a)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for k, v := range fields {
			m[k] = v
		}
		m["intrash"] = ca.a.InTrash
		catalog = append(catalog, m)
		c.uuids = append(c.uuids, ca.a.UUID)
		// burst_photos holds UUIDs of the catalog only: they are in c.uuids.
		if y := ca.a.Date[:4]; y != "1970" {
			years[y] = true
		}
	}
	for _, f := range fs {
		if f.ContentID != "" {
			c.uuids = append(c.uuids, f.ContentID)
		}
		if f.SHA1 != "" {
			c.sha1s = append(c.sha1s, f.SHA1)
		}
		if len(f.DateTaken) >= 4 && f.DateTaken[:4] != "1970" {
			years[f.DateTaken[:4]] = true
		}
	}
	for y := range years {
		if y <= "2027" { // later years are not supported and never reach a case
			c.years = append(c.years, y)
		}
	}
	sort.Strings(c.years)

	rows := inventoryRows(fs)
	inv := map[string]any{"root": `C:\Users\Zinaida\YandexDisk\Фото`, "created": "2026-09-01T10:00:00", "files": rows}
	links := map[string][][]string{}
	plan := planFor(as, fs)
	for _, ca := range as {
		for _, l := range ca.links {
			links[ca.a.UUID] = append(links[ca.a.UUID], []string{l.path, l.kind})
		}
	}
	writeJSON(t, dir, "icloud_catalog.json", catalog)
	switch {
	case opt.prev || opt.noPrevMatch:
		writeJSON(t, dir, "yandex_inventory.json", map[string]any{"root": inv["root"], "files": rows[1:]})
		prev := rows
		if opt.noPrevMatch {
			prev = rows[2:]
		}
		writeJSON(t, dir, "yandex_inventory.prev.json", map[string]any{"root": inv["root"], "files": prev})
	default:
		writeJSON(t, dir, "yandex_inventory.json", inv)
	}
	writeJSON(t, dir, "plan/plan.json", plan)
	if opt.rerunDiffers {
		plan["download"] = plan["download"].([]map[string]any)[1:]
	}
	writeJSON(t, dir, "plan-rerun/plan.json", plan)
	writeJSON(t, dir, "plan-rerun/links.json", links)
	c.read = []string{"icloud_catalog.json", "plan-rerun/links.json", "plan-rerun/plan.json", "plan/plan.json", "yandex_inventory.json"}
	if opt.prev || opt.noPrevMatch {
		c.read = append(c.read, "yandex_inventory.prev.json")
		sort.Strings(c.read)
	}

	// Files the tool must never open, each with its own canary.
	ignored := [][2]string{
		{"yandex_inventory.csv", "path;name;camera\n2023/x.jpg;x.jpg;KanareykaInventoryCsv\n"},
		{"yandex_duplicates.json", `{"3b18": ["KanareykaDuplicatesJson/a.jpg"]}`},
		{"yandex_summary.json", `{"files_total": 31337, "note": "KanareykaSummaryJson"}`},
		{"plan/0_summary.txt", "KanareykaPlanSummary 31337\n"},
		{"plan/1_download_to_yandex.csv", "name;size\nKanareykaPlanDownload;1\n"},
		{"plan/2_replace_on_yandex.csv", "yandex_path\nKanareykaPlanReplace\n"},
		{"plan/3_download_edited.csv", "name\nKanareykaPlanEdited\n"},
		{"plan/4_upload_to_icloud.csv", "path\nKanareykaPlanUpload\n"},
		{"plan/5_icloud_album_ops.csv", "name\nKanareykaPlanAlbumOps\n"},
		{"plan/5a_icloud_album_renames.csv", "old_album\nKanareykaPlanRenames\n"},
		{"plan/6_conflicts.csv", "type\nKanareykaPlanConflicts\n"},
		{"plan-rerun/0_summary.txt", "KanareykaRerunSummary\n"},
		{"plan-rerun/1_download_to_yandex.csv", "name\nKanareykaRerunDownload\n"},
		{"export/batch_001.txt", "KanareykaBatchList\n" + as[0].a.UUID + "\n"},
		{"export/reports/batch_001.csv", "uuid,filename\nKanareykaExportReport,x\n"},
		{"export/done/batch_001", "KanareykaExportDone"},
		{"inventory_log.txt", "Корень: C:\\Users\\Zinaida\\YandexDisk KanareykaInventoryLog\n"},
		{"_probe1.txt", "KanareykaProbeLog zinaida_p\n"},
	}
	for _, ig := range ignored {
		writeFile(t, dir, ig[0], []byte(ig[1]))
		c.ignored = append(c.ignored, ig[0])
		c.canaries = append(c.canaries, canaryRE.FindAllString(ig[1], -1)...)
	}
	c.uuids = append(c.uuids, "F00DFACE-1111-4222-8333-444455556666")
	sort.Strings(c.ignored)
	return c
}

// inventoryRows writes the rows as the inventory script does, with the
// derived columns and the personal ones.
func inventoryRows(fs []anon.File) []map[string]any {
	var rows []map[string]any
	for _, f := range fs {
		name := anon.BaseName(f.Path)
		stem, ext := anon.Splitext(name)
		fi := anon.FolderInfoOf(f.Path)
		var w, h any = int(f.Width), int(f.Height)
		if f.DateTaken == "" {
			w, h = "", ""
		}
		row := map[string]any{"path": f.Path, "name": name, "stem": stem, "ext": strings.ToLower(strings.TrimPrefix(ext, "."))}
		row["kind"], row["size"], row["mtime"], row["date_taken"] = f.Kind, f.Size, "2025-01-02T03:04:05", f.DateTaken
		row["date_source"] = map[bool]string{true: "exif", false: ""}[f.DateTaken != ""]
		row["width"], row["height"], row["content_id"], row["camera"], row["duration"] = w, h, f.ContentID, "Canon EOS 5D Zinaidy", ""
		row["sha1"], row["folder"], row["year"], row["event"], row["event_month"] = f.SHA1, fi.Folder, fi.Year, fi.Event, fi.EventMonth
		row["event_day"], row["unsorted"], row["folder_date_mismatch"], row["live_pair"] = fi.EventDay, fi.Year == "", "", f.LivePair
		rows = append(rows, row)
	}
	return rows
}

// planFor writes a reviewed plan consistent with the table.
func planFor(as []corpusAsset, fs []anon.File) map[string]any {
	download := []map[string]any{}
	replace := []map[string]any{}
	upload := []map[string]any{}
	conflicts := [][]string{}
	linked := map[string]bool{}
	for _, ca := range as {
		for _, l := range ca.links {
			linked[l.path] = true
		}
	}
	for _, ca := range as {
		a := ca.a
		if a.InTrash {
			continue
		}
		if len(ca.links) == 0 {
			download = append(download, map[string]any{"uuid": a.UUID, "name": a.Name(), "size": a.OriginalFilesize,
				"date": a.Date, "target": "Мырдоновка/" + a.Name(), "from_album": "Семья Пельменевых"})
		}
		for _, l := range ca.links {
			switch {
			case l.kind == anon.LevelProbable:
				conflicts = append(conflicts, []string{anon.ConflictProbableMatch, a.Name(), l.path, "та же секунда съёмки " + a.Date})
			case l.plan == anon.PlanReplaceDecision:
				replace = append(replace, map[string]any{"uuid": a.UUID, "icloud_name": a.Name(), "icloud_size": a.OriginalFilesize,
					"yandex_path": l.path, "yandex_size": 1, "match": l.kind, "target": l.path})
			case l.plan == anon.PlanKeepCloudDecision:
				conflicts = append(conflicts, []string{anon.ConflictYandexBetter, a.Name(), l.path, "на Диске больше — оставляем Диск"})
			}
		}
	}
	for _, f := range fs {
		if anon.IsMedia(f.Kind) && !linked[f.Path] {
			upload = append(upload, map[string]any{"path": f.Path, "size": f.Size, "convert": f.Kind == anon.KindConvert,
				"live_pair": f.LivePair, "album": "Зинаида", "_album": []any{[]string{}, "Зинаида"}})
		}
		if !anon.IsMedia(f.Kind) {
			conflicts = append(conflicts, []string{"disk_only_not_media", f.Path, "", "остаётся только на Диске"})
		}
	}
	conflicts = append(conflicts, []string{"fix_date_1970", "PXL_20210305_101112345.jpg", cid("08"), "дата 1970 исправлена"})
	conflicts = append(conflicts, []string{"yandex_duplicate", "2017/02-02 - Каток/IMG_9000.HEIC", "Избранное/Лучшее у Пельменевых/IMG_9000.HEIC", "одинаковый SHA-1"})
	plan := map[string]any{"download": download, "replace": replace, "upload": upload, "conflicts": conflicts}
	plan["download_edited"] = []map[string]any{{"uuid": cid("22"), "name": "IMG_2200.HEIC", "target": "x"}}
	plan["album_ops"] = []map[string]any{{"uuid": cid("10"), "name": "IMG_1000.HEIC", "add": "Мырдоновка", "remove": "",
		"_add": [][]any{{[]string{}, "Мырдоновка"}}, "_remove": []any{}}}
	return plan
}

func writeJSON(t *testing.T, dir, name string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, name, data)
}

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCorpusIsConsistent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := writeCorpus(t, dir)
	if len(c.canaries) < 30 || len(c.read) != 5 || len(c.ignored) < 15 {
		t.Errorf("corpus = %d canaries, %d read, %d ignored", len(c.canaries), len(c.read), len(c.ignored))
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeQuietly(root)
	m, which, err := readMaterial(root.FS())
	if err != nil {
		t.Fatalf("readMaterial: %v", err)
	}
	if which != "current" || len(m.Assets) == 0 || len(m.Files) == 0 {
		t.Errorf("material: %s, %d assets, %d files", which, len(m.Assets), len(m.Files))
	}
	if cnt := anon.CheckPartition(m); !cnt.Zero() {
		t.Errorf("partition: %s", cnt)
	}
}
