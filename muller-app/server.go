package main

import (
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Server exposes the shift-entry API and the installable web app.
type Server struct {
	Store    *Store
	Static   fs.FS
	Password string // optional shared password (HTTP Basic auth); empty disables it
}

// Handler returns the HTTP routes of the app.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/entries", s.listEntries)
	mux.HandleFunc("GET /api/entries/{id}", s.getEntry)
	mux.HandleFunc("PUT /api/entries/{id}", s.putEntry)
	mux.HandleFunc("DELETE /api/entries/{id}", s.deleteEntry)
	mux.HandleFunc("GET /api/refs", s.listRefs)
	mux.HandleFunc("GET /api/export", s.exportCSV)
	mux.HandleFunc("POST /api/import", s.importEntries)
	static := http.FileServerFS(s.Static)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// The shell and service worker must be revalidated so updates reach tablets.
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	})
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	if s.Password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		_, pw, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(pw), []byte(s.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Ligne Muller", charset="UTF-8"`)
			http.Error(w, "accès réservé", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func serverErr(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	writeErr(w, http.StatusInternalServerError, "erreur serveur")
}

// weekParams reads ?year=&week=, defaulting to the current ISO week.
func weekParams(r *http.Request) (int, int, error) {
	year, week := time.Now().ISOWeek()
	q := r.URL.Query()
	if v := q.Get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil || y < 2000 || y > 2100 {
			return 0, 0, errors.New("année invalide")
		}
		year = y
	}
	if v := q.Get("week"); v != "" {
		wk, err := strconv.Atoi(v)
		if err != nil || wk < 1 || wk > 53 {
			return 0, 0, errors.New("semaine invalide")
		}
		week = wk
	}
	return year, week, nil
}

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	var (
		es  []Entry
		err error
	)
	if r.URL.Query().Get("all") == "1" {
		es, err = s.Store.All(r.Context())
	} else {
		year, week, perr := weekParams(r)
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		es, err = s.Store.Week(r.Context(), year, week)
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, es)
}

func (s *Server) getEntry(w http.ResponseWriter, r *http.Request) {
	e, err := s.Store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

const maxBody = 1 << 20

func (s *Server) putEntry(w http.ResponseWriter, r *http.Request) {
	var e Entry
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&e); err != nil {
		writeErr(w, http.StatusBadRequest, "données illisibles")
		return
	}
	e.ID = r.PathValue("id")
	if err := e.Normalize(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.Store.Put(r.Context(), &e); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request) {
	err := s.Store.Delete(r.Context(), r.PathValue("id"))
	if err != nil && !errors.Is(err, ErrNotFound) {
		serverErr(w, err)
		return
	}
	// Deleting twice is fine: offline tablets may replay a queued delete.
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRefs(w http.ResponseWriter, r *http.Request) {
	refs, err := s.Store.Refs(r.Context(), 30)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

// importEntries loads a JSON array of entries (e.g. exported from the former
// claude.ai page). Entries are upserted by id; invalid ones are reported.
func (s *Server) importEntries(w http.ResponseWriter, r *http.Request) {
	var es []Entry
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*maxBody)).Decode(&es); err != nil {
		writeErr(w, http.StatusBadRequest, "un tableau JSON de postes est attendu")
		return
	}
	imported := 0
	rejected := map[string]string{}
	for i := range es {
		e := &es[i]
		if err := e.Normalize(); err != nil {
			key := e.ID
			if key == "" {
				key = "#" + strconv.Itoa(i)
			}
			rejected[key] = err.Error()
			continue
		}
		if err := s.Store.Put(r.Context(), e); err != nil {
			serverErr(w, err)
			return
		}
		imported++
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "rejected": rejected})
}

// exportCSV writes one row per date and shift, in the column order of the
// "Saisie manuelle" sheet (A→H), using French number formatting.
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	year, week, err := weekParams(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	es, err := s.Store.Week(r.Context(), year, week)
	if err != nil {
		serverErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="saisie_manuelle_%d_S%02d.csv"`, year, week))
	w.Write([]byte("\xEF\xBB\xBF")) // BOM so Excel reads UTF-8
	WriteCSV(w, es)
}

type csvRow struct {
	date, poste, equipe   string
	week, nb              int
	chgMin, ouv, panneMin float64
}

var posteOrder = map[string]int{PosteNuit: 0, PosteMatin: 1, PosteApresMidi: 2}

// WriteCSV aggregates entries per date and shift and writes them as CSV.
func WriteCSV(w interface{ Write([]byte) (int, error) }, es []Entry) error {
	groups := map[string]*csvRow{}
	for _, e := range es {
		k := e.Date + "|" + e.Poste
		g := groups[k]
		if g == nil {
			g = &csvRow{date: e.Date, poste: e.Poste, equipe: e.Equipe, week: e.Week}
			groups[k] = g
		}
		g.nb += e.NbChangements
		g.chgMin += e.TpsChangement
		g.ouv += e.Ouverture
		g.panneMin += e.StopMinutes()
	}
	rows := make([]*csvRow, 0, len(groups))
	for _, g := range groups {
		rows = append(rows, g)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].date != rows[j].date {
			return rows[i].date < rows[j].date
		}
		return posteOrder[rows[i].poste] < posteOrder[rows[j].poste]
	})
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cw.UseCRLF = true
	cw.Write([]string{"Date", "Poste", "Equipe", "Semaine", "Nb changements", "Temps changement (h)", "Heures ouverture", "Pannes (h)"})
	for _, g := range rows {
		cw.Write([]string{frDate(g.date), g.poste, g.equipe, "S" + strconv.Itoa(g.week), strconv.Itoa(g.nb),
			frNum(g.chgMin / 60), frNum(g.ouv), frNum(g.panneMin / 60)})
	}
	cw.Flush()
	return cw.Error()
}

func frDate(iso string) string {
	p := strings.Split(iso, "-")
	if len(p) != 3 {
		return iso
	}
	return p[2] + "/" + p[1] + "/" + p[0]
}

func frNum(x float64) string {
	x = math.Round(x*100) / 100
	return strings.Replace(strconv.FormatFloat(x, 'f', -1, 64), ".", ",", 1)
}
