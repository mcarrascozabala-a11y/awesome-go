package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Shift names as entered on the line.
const (
	PosteMatin     = "Matin"
	PosteApresMidi = "Après-midi"
	PosteNuit      = "Nuit"
)

// Stop is one line stoppage declared during a shift.
type Stop struct {
	Cause string  `json:"cause"`
	Min   float64 `json:"min"`
}

// Entry is one shift report ("poste") for the Muller line.
type Entry struct {
	ID            string  `json:"id"`
	Date          string  `json:"date"`
	Year          int     `json:"year"`
	Week          int     `json:"week"`
	Poste         string  `json:"poste"`
	Equipe        string  `json:"equipe"`
	Ref           string  `json:"ref"`
	Pieces        int     `json:"pieces"`
	Ouverture     float64 `json:"ouverture"`
	NbChangements int     `json:"nbChangements"`
	TpsChangement float64 `json:"tpsChangement"`
	Arrets        []Stop  `json:"arrets"`
	Commentaire   string  `json:"commentaire"`
	Auteur        string  `json:"auteur"`
	UpdatedAt     string  `json:"updatedAt"`
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidID reports whether id is an acceptable entry identifier.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// TeamFor returns the setter ("régleur") scheduled for a shift in an ISO week:
// MR works mornings on odd weeks and afternoons on even weeks, BC the opposite.
func TeamFor(week int, poste string) string {
	if poste == PosteNuit {
		return PosteNuit
	}
	if (poste == PosteMatin) == (week%2 == 1) {
		return "MR"
	}
	return "BC"
}

// Normalize checks e against the line's rules and fills derived fields
// (ISO year and week, night team, trimmed text). It returns a French message
// suitable for display when the entry is rejected.
func (e *Entry) Normalize() error {
	if !ValidID(e.ID) {
		return errors.New("identifiant de poste invalide")
	}
	d, err := time.Parse("2006-01-02", e.Date)
	if err != nil {
		return errors.New("date invalide (format AAAA-MM-JJ attendu)")
	}
	switch e.Poste {
	case PosteMatin, PosteApresMidi, PosteNuit:
	default:
		return fmt.Errorf("poste inconnu : %q", e.Poste)
	}
	switch d.Weekday() {
	case time.Sunday:
		return errors.New("pas de poste le dimanche ; la nuit de dimanche à lundi se saisit au lundi")
	case time.Saturday:
		if e.Poste != PosteMatin {
			return errors.New("le samedi, seul le poste du matin existe")
		}
	}
	if e.Poste == PosteNuit {
		e.Equipe = PosteNuit
	} else if e.Equipe != "MR" && e.Equipe != "BC" {
		return fmt.Errorf("régleur inconnu : %q", e.Equipe)
	}
	if e.Ouverture <= 0 || e.Ouverture > 24 {
		return errors.New("heures d'ouverture à indiquer (entre 0 et 24 h)")
	}
	if e.NbChangements < 0 || e.TpsChangement < 0 || e.Pieces < 0 {
		return errors.New("les valeurs ne peuvent pas être négatives")
	}
	stops := make([]Stop, 0, len(e.Arrets))
	for _, s := range e.Arrets {
		s.Cause = strings.TrimSpace(s.Cause)
		if s.Min <= 0 {
			continue
		}
		if s.Cause == "" {
			s.Cause = "Autre"
		}
		stops = append(stops, s)
	}
	e.Arrets = stops
	e.Year, e.Week = d.ISOWeek()
	e.Ref = strings.TrimSpace(e.Ref)
	e.Commentaire = strings.TrimSpace(e.Commentaire)
	e.Auteur = strings.TrimSpace(e.Auteur)
	if e.UpdatedAt == "" {
		e.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return nil
}

// StopMinutes is the total stoppage time of the shift, in minutes.
func (e *Entry) StopMinutes() float64 {
	var t float64
	for _, s := range e.Arrets {
		t += s.Min
	}
	return t
}
