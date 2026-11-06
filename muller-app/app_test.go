package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestServer(t *testing.T, password string) (*httptest.Server, *Store) {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	static := fstest.MapFS{"index.html": {Data: []byte("<title>Saisie Ligne Muller</title>")}}
	srv := httptest.NewServer((&Server{Store: st, Static: static, Password: password}).Handler())
	t.Cleanup(srv.Close)
	return srv, st
}

func do(t *testing.T, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func sample(id string) Entry {
	return Entry{ID: id, Date: "2026-11-05", Poste: PosteApresMidi, Equipe: "BC", Ref: " DAN0021 ", Pieces: 100,
		Ouverture: 3, NbChangements: 1, TpsChangement: 30,
		Arrets: []Stop{{Cause: "Panne presse", Min: 20}, {Cause: "Autre", Min: 0}, {Cause: "", Min: 10}}}
}

func TestTeamFor(t *testing.T) {
	cases := []struct {
		week  int
		poste string
		want  string
	}{{45, PosteMatin, "MR"}, {45, PosteApresMidi, "BC"}, {46, PosteMatin, "BC"}, {46, PosteApresMidi, "MR"}, {46, PosteNuit, "Nuit"}}
	for _, c := range cases {
		if got := TeamFor(c.week, c.poste); got != c.want {
			t.Errorf("TeamFor(%d,%s)=%s want %s", c.week, c.poste, got, c.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	e := sample("abc")
	if err := e.Normalize(); err != nil {
		t.Fatal(err)
	}
	if e.Year != 2026 || e.Week != 45 || e.Ref != "DAN0021" || len(e.Arrets) != 2 || e.Arrets[1].Cause != "Autre" || e.UpdatedAt == "" {
		t.Fatalf("unexpected normalized entry: %+v", e)
	}
	if e.StopMinutes() != 30 {
		t.Fatalf("StopMinutes=%v", e.StopMinutes())
	}
	night := sample("n1")
	night.Poste, night.Equipe = PosteNuit, "MR"
	if err := night.Normalize(); err != nil || night.Equipe != PosteNuit {
		t.Fatalf("night: %v %s", err, night.Equipe)
	}

	bad := map[string]func(*Entry){
		"id":       func(e *Entry) { e.ID = "../x" },
		"date":     func(e *Entry) { e.Date = "05/11/2026" },
		"poste":    func(e *Entry) { e.Poste = "Soir" },
		"sunday":   func(e *Entry) { e.Date = "2026-11-08" },
		"saturday": func(e *Entry) { e.Date = "2026-11-07" },
		"equipe":   func(e *Entry) { e.Equipe = "XX" },
		"ouv":      func(e *Entry) { e.Ouverture = 0 },
		"neg":      func(e *Entry) { e.Pieces = -1 },
	}
	for name, mut := range bad {
		e := sample("ok")
		mut(&e)
		if err := e.Normalize(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	sat := sample("sat")
	sat.Date, sat.Poste, sat.Equipe = "2026-11-07", PosteMatin, "MR"
	if err := sat.Normalize(); err != nil {
		t.Errorf("saturday morning should be allowed: %v", err)
	}
}

func TestCRUD(t *testing.T) {
	srv, _ := newTestServer(t, "")
	res, body := do(t, "PUT", srv.URL+"/api/entries/e1", sample("ignored"))
	if res.StatusCode != 200 {
		t.Fatalf("put: %d %s", res.StatusCode, body)
	}
	var got Entry
	json.Unmarshal(body, &got)
	if got.ID != "e1" || got.Week != 45 {
		t.Fatalf("put returned %+v", got)
	}

	// Upsert is idempotent and replaces.
	upd := sample("e1")
	upd.Commentaire = "presse 2 bloquée"
	do(t, "PUT", srv.URL+"/api/entries/e1", upd)
	_, body = do(t, "GET", srv.URL+"/api/entries/e1", nil)
	json.Unmarshal(body, &got)
	if got.Commentaire != "presse 2 bloquée" {
		t.Fatalf("update not applied: %+v", got)
	}

	var list []Entry
	_, body = do(t, "GET", srv.URL+"/api/entries?year=2026&week=45", nil)
	json.Unmarshal(body, &list)
	if len(list) != 1 {
		t.Fatalf("week list: %s", body)
	}
	_, body = do(t, "GET", srv.URL+"/api/entries?year=2026&week=44", nil)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("empty week should be []: %s", body)
	}
	_, body = do(t, "GET", srv.URL+"/api/entries?all=1", nil)
	json.Unmarshal(body, &list)
	if len(list) != 1 {
		t.Fatalf("all: %s", body)
	}
	_, body = do(t, "GET", srv.URL+"/api/refs", nil)
	if strings.TrimSpace(string(body)) != `["DAN0021"]` {
		t.Fatalf("refs: %s", body)
	}

	if res, _ := do(t, "DELETE", srv.URL+"/api/entries/e1", nil); res.StatusCode != 204 {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if res, _ := do(t, "DELETE", srv.URL+"/api/entries/e1", nil); res.StatusCode != 204 {
		t.Fatalf("second delete should be accepted: %d", res.StatusCode)
	}
	if res, _ := do(t, "GET", srv.URL+"/api/entries/e1", nil); res.StatusCode != 404 {
		t.Fatalf("get deleted: %d", res.StatusCode)
	}
}

func TestBadRequests(t *testing.T) {
	srv, _ := newTestServer(t, "")
	sun := sample("x")
	sun.Date = "2026-11-08"
	if res, body := do(t, "PUT", srv.URL+"/api/entries/x", sun); res.StatusCode != 422 || !strings.Contains(string(body), "dimanche") {
		t.Fatalf("sunday: %d %s", res.StatusCode, body)
	}
	req, _ := http.NewRequest("PUT", srv.URL+"/api/entries/x", strings.NewReader("{"))
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 400 {
		t.Fatalf("bad json: %d", res.StatusCode)
	}
	for _, q := range []string{"week=99", "week=a", "year=1", "year=x"} {
		if res, _ := do(t, "GET", srv.URL+"/api/entries?"+q, nil); res.StatusCode != 400 {
			t.Errorf("%s: %d", q, res.StatusCode)
		}
		if res, _ := do(t, "GET", srv.URL+"/api/export?"+q, nil); res.StatusCode != 400 {
			t.Errorf("export %s: %d", q, res.StatusCode)
		}
	}
	if res, _ := do(t, "GET", srv.URL+"/api/entries", nil); res.StatusCode != 200 {
		t.Fatalf("default week: %d", res.StatusCode)
	}
}

func TestImportAndExport(t *testing.T) {
	srv, _ := newTestServer(t, "")
	// Format of the former claude.ai page: authorId is dropped, the rest maps 1:1.
	payload := `[
	 {"id":"19au5yspyihg8hjq8enf","arrets":[],"authorId":"u_x","commentaire":"","date":"2026-11-05","equipe":"BC","nbChangements":1,"ouverture":3,"pieces":100,"poste":"Après-midi","ref":"DAN0021","tpsChangement":15,"updatedAt":"2026-11-05T21:23:54.143Z","week":45,"year":2026},
	 {"id":"1w17yq4e7jrngtxy54k5","arrets":[{"cause":"Panne presse","min":45}],"date":"2026-11-05","equipe":"BC","nbChangements":0,"ouverture":4.5,"pieces":180,"poste":"Après-midi","ref":"DAN0014","tpsChangement":0},
	 {"id":"n1","date":"2026-11-03","poste":"Nuit","ouverture":7},
	 {"id":"bad","date":"2026-11-08","poste":"Matin","equipe":"MR","ouverture":7},
	 {"date":"2026-11-04","poste":"Matin","equipe":"MR","ouverture":7}
	]`
	res, err := http.Post(srv.URL+"/api/import", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Imported int               `json:"imported"`
		Rejected map[string]string `json:"rejected"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out.Imported != 3 || len(out.Rejected) != 2 || out.Rejected["#4"] == "" {
		t.Fatalf("import: %+v", out)
	}
	if res, _ := do(t, "POST", srv.URL+"/api/import", map[string]int{"x": 1}); res.StatusCode != 400 {
		t.Fatalf("import object: %d", res.StatusCode)
	}

	res, body := do(t, "GET", srv.URL+"/api/export?year=2026&week=45", nil)
	if !strings.Contains(res.Header.Get("Content-Disposition"), "saisie_manuelle_2026_S45.csv") {
		t.Fatalf("disposition: %s", res.Header.Get("Content-Disposition"))
	}
	want := "\xEF\xBB\xBFDate;Poste;Equipe;Semaine;Nb changements;Temps changement (h);Heures ouverture;Pannes (h)\r\n" +
		"03/11/2026;Nuit;Nuit;S45;0;0;7;0\r\n" +
		"05/11/2026;Après-midi;BC;S45;1;0,25;7,5;0,75\r\n"
	if string(body) != want {
		t.Fatalf("csv:\n%q\nwant\n%q", body, want)
	}
}

func TestCSVOrdering(t *testing.T) {
	var b bytes.Buffer
	WriteCSV(&b, []Entry{
		{Date: "2026-11-03", Poste: PosteApresMidi, Equipe: "MR", Week: 45, Ouverture: 1},
		{Date: "2026-11-03", Poste: PosteMatin, Equipe: "BC", Week: 45, Ouverture: 1},
		{Date: "2026-11-02", Poste: PosteMatin, Equipe: "BC", Week: 45, Ouverture: 1},
	})
	lines := strings.Split(strings.TrimSpace(b.String()), "\r\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "02/11") || !strings.Contains(lines[2], "Matin") || !strings.Contains(lines[3], "Après-midi") {
		t.Fatalf("order: %q", lines)
	}
	if frDate("bad") != "bad" || frNum(1.005) != "1" && frNum(1.005) != "1,01" {
		t.Fatal("helpers")
	}
}

func TestAuthAndStatic(t *testing.T) {
	srv, _ := newTestServer(t, "s3cret")
	if res, _ := do(t, "GET", srv.URL+"/api/entries", nil); res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated: %d", res.StatusCode)
	}
	if res, _ := do(t, "GET", srv.URL+"/healthz", nil); res.StatusCode != 200 {
		t.Fatalf("healthz must stay open: %d", res.StatusCode)
	}
	for pw, code := range map[string]int{"nope": 401, "s3cret": 200} {
		req, _ := http.NewRequest("GET", srv.URL+"/", nil)
		req.SetBasicAuth("ligne", pw)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != code {
			t.Fatalf("pw %s: %d", pw, res.StatusCode)
		}
		if code == 200 && (!strings.Contains(string(b), "Saisie Ligne Muller") || res.Header.Get("Cache-Control") != "no-cache") {
			t.Fatalf("static: %s %s", b, res.Header.Get("Cache-Control"))
		}
	}
}

func TestStoreErrors(t *testing.T) {
	if _, err := OpenStore(filepath.Join(t.TempDir(), "missing", "dir", "x.db")); err == nil {
		t.Fatal("expected error for unwritable path")
	}
	srv, st := newTestServer(t, "")
	st.Close() // every query now fails → 500s
	for _, p := range []string{"/api/entries", "/api/entries?all=1", "/api/entries/a", "/api/refs", "/api/export"} {
		if res, _ := do(t, "GET", srv.URL+p, nil); res.StatusCode != 500 {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
	if res, _ := do(t, "PUT", srv.URL+"/api/entries/a", sample("a")); res.StatusCode != 500 {
		t.Errorf("put: %d", res.StatusCode)
	}
	if res, _ := do(t, "DELETE", srv.URL+"/api/entries/a", nil); res.StatusCode != 500 {
		t.Errorf("delete: %d", res.StatusCode)
	}
	if res, _ := do(t, "POST", srv.URL+"/api/import", []Entry{sample("a")}); res.StatusCode != 500 {
		t.Errorf("import: %d", res.StatusCode)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("MULLER_TEST_X", "v")
	if envOr("MULLER_TEST_X", "d") != "v" || envOr("MULLER_TEST_UNSET", "d") != "d" {
		t.Fatal("envOr")
	}
}
