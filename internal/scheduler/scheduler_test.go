package scheduler

import (
	"testing"
	"time"

	"astro-telescope-cli/pkg/model"
)

func testSetup() ([]*model.TelescopeSite, []*model.ObservationTarget) {
	sites := []*model.TelescopeSite{
		{ID: "S1", Name: "Site1", Latitude: 34.0, Longitude: -107.6, Elevation: 2000, Health: 90.0},
		{ID: "S2", Name: "Site2", Latitude: -30.7, Longitude: 21.4, Elevation: 1000, Health: 85.0},
	}
	targets := []*model.ObservationTarget{
		{ID: "T1", Name: "Target1", RA: 83.6, Dec: 22.0, Duration: 30 * time.Minute, Priority: 7},
		{ID: "T2", Name: "Target2", RA: 10.7, Dec: 41.3, Duration: 45 * time.Minute, Priority: 5},
		{ID: "T3", Name: "Target3", RA: 201.4, Dec: -43.0, Duration: 60 * time.Minute, Priority: 9},
	}
	return sites, targets
}

func TestComputeScore(t *testing.T) {
	s := &Scheduler{}
	score := s.computeScore(5, 45.0, 80.0)
	if score <= 0 || score > 1.0 {
		t.Errorf("score out of expected range: %.4f", score)
	}

	scoreHigh := s.computeScore(10, 80.0, 100.0)
	scoreLow := s.computeScore(1, 10.0, 10.0)
	if scoreHigh <= scoreLow {
		t.Errorf("high score should be > low score: high=%.4f low=%.4f", scoreHigh, scoreLow)
	}
}

func TestGenerateCandidates(t *testing.T) {
	sites, targets := testSetup()
	s := NewScheduler(sites, targets)
	start := time.Now().UTC().Truncate(time.Hour)
	cands := s.generateCandidates(start, 24)
	if len(cands) == 0 {
		t.Log("No candidates generated (may depend on sky position)")
	}
	for _, c := range cands {
		if c.score <= 0 {
			t.Error("candidate score should be positive")
		}
		if c.endTime.Before(c.startTime) {
			t.Error("candidate end before start")
		}
	}
}

func TestSchedule(t *testing.T) {
	sites, targets := testSetup()
	s := NewScheduler(sites, targets)
	start := time.Now().UTC().Truncate(time.Hour)
	result := s.Schedule(start, 24)

	scheduledIDs := make(map[string]bool)
	for _, obs := range result {
		if scheduledIDs[obs.Target.ID] {
			t.Errorf("target %s scheduled twice", obs.Target.ID)
		}
		scheduledIDs[obs.Target.ID] = true
		if obs.End.Before(obs.Start) {
			t.Error("observation end before start")
		}
	}
}

func TestSiteScheduleNoOverlap(t *testing.T) {
	sites, targets := testSetup()
	s := NewScheduler(sites, targets)
	start := time.Now().UTC().Truncate(time.Hour)
	result := s.Schedule(start, 24)

	siteObs := make(map[string][]*model.ScheduledObservation)
	for _, obs := range result {
		siteObs[obs.Site.ID] = append(siteObs[obs.Site.ID], obs)
	}

	for sid, obsList := range siteObs {
		for i := 0; i < len(obsList); i++ {
			for j := i + 1; j < len(obsList); j++ {
				a, b := obsList[i], obsList[j]
				if a.Start.Before(b.End) && b.Start.Before(a.End) {
					t.Errorf("site %s has overlapping observations: %v and %v",
						sid, a.Start.Format("15:04"), b.Start.Format("15:04"))
				}
			}
		}
	}
}
