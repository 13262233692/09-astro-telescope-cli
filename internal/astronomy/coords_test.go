package astronomy

import (
	"math"
	"testing"
	"time"

	"astro-telescope-cli/pkg/model"
)

func TestJulianDate(t *testing.T) {
	t1, _ := time.Parse(time.RFC3339, "2000-01-01T12:00:00Z")
	jd := JulianDate(t1)
	expected := 2451545.0
	if math.Abs(jd-expected) > 0.0001 {
		t.Errorf("JD for J2000 expected %.4f, got %.4f", expected, jd)
	}
}

func TestGreenwichSiderealTime(t *testing.T) {
	t1, _ := time.Parse(time.RFC3339, "2000-01-01T12:00:00Z")
	gmst := GreenwichSiderealTime(t1)
	if gmst < 0 || gmst >= 360 {
		t.Errorf("GMST out of range: %.4f", gmst)
	}
}

func TestLocalSiderealTime(t *testing.T) {
	t1, _ := time.Parse(time.RFC3339, "2000-01-01T12:00:00Z")
	lst0 := LocalSiderealTime(t1, 0)
	gmst := GreenwichSiderealTime(t1)
	if math.Abs(lst0-gmst) > 0.0001 {
		t.Errorf("LST at 0 lon should equal GMST")
	}

	lst := LocalSiderealTime(t1, 90)
	expected := math.Mod(gmst+90, 360)
	if math.Abs(lst-expected) > 0.0001 {
		t.Errorf("LST at 90 lon expected %.4f, got %.4f", expected, lst)
	}
}

func TestEquatorialToHorizontal(t *testing.T) {
	site := &model.TelescopeSite{
		Latitude:  34.0784,
		Longitude: -107.6184,
	}

	target := &model.ObservationTarget{
		RA:  83.6331,
		Dec: 22.0145,
	}

	testTime := time.Now().UTC()
	altAz := EquatorialToHorizontal(testTime, target.RA, target.Dec, site.Latitude, site.Longitude)

	if altAz.Altitude < -90 || altAz.Altitude > 90 {
		t.Errorf("Altitude out of range: %.2f", altAz.Altitude)
	}
	if altAz.Azimuth < 0 || altAz.Azimuth >= 360 {
		t.Errorf("Azimuth out of range: %.2f", altAz.Azimuth)
	}
}

func TestIsVisible(t *testing.T) {
	site := &model.TelescopeSite{
		Latitude:  34.0784,
		Longitude: -107.6184,
	}
	target := &model.ObservationTarget{
		RA:  83.6331,
		Dec: 22.0145,
	}

	tNow := time.Now().UTC()
	_ = IsVisible(tNow, target, site)
}

func TestComputeVisibility(t *testing.T) {
	site := &model.TelescopeSite{
		ID:        "TEST",
		Name:      "Test Site",
		Latitude:  34.0784,
		Longitude: -107.6184,
	}
	target := &model.ObservationTarget{
		ID:       "T1",
		Name:     "Test Target",
		RA:       83.6331,
		Dec:      22.0145,
		Duration: 30 * time.Minute,
		Priority: 5,
	}

	start := time.Now().UTC().Truncate(time.Hour)
	vis := ComputeVisibility(target, site, start, 24*time.Hour)

	if vis.MaxAlt < -90 || vis.MaxAlt > 90 {
		t.Errorf("invalid max altitude: %.2f", vis.MaxAlt)
	}
	if vis.BestTime.IsZero() {
		t.Error("best time should not be zero")
	}
	for _, w := range vis.Windows {
		if w.End.Before(w.Start) {
			t.Error("window end before start")
		}
	}
}

func TestFindBestObservationSlot(t *testing.T) {
	site := &model.TelescopeSite{
		Latitude:  34.0784,
		Longitude: -107.6184,
	}
	target := &model.ObservationTarget{
		RA:       83.6331,
		Dec:      22.0145,
		Duration: 30 * time.Minute,
	}

	start := time.Now().UTC().Truncate(time.Hour)
	_, _, _ = FindBestObservationSlot(target, site, start, 24)
}
