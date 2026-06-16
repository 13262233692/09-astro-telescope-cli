package scheduler

import (
	"sort"
	"time"

	"astro-telescope-cli/internal/astronomy"
	"astro-telescope-cli/pkg/model"
)

const (
	weightPriority = 0.35
	weightAltitude = 0.40
	weightHealth   = 0.25
	altitudeMaxRef = 90.0
)

type candidateSlot struct {
	target     *model.ObservationTarget
	site       *model.TelescopeSite
	startTime  time.Time
	endTime    time.Time
	avgAlt     float64
	score      float64
}

type Scheduler struct {
	sites   []*model.TelescopeSite
	targets []*model.ObservationTarget
}

func NewScheduler(sites []*model.TelescopeSite, targets []*model.ObservationTarget) *Scheduler {
	return &Scheduler{
		sites:   sites,
		targets: targets,
	}
}

func (s *Scheduler) computeScore(priority int, avgAlt float64, health float64) float64 {
	priorityNorm := float64(priority) / 10.0
	if priorityNorm > 1.0 {
		priorityNorm = 1.0
	}
	altNorm := avgAlt / altitudeMaxRef
	if altNorm < 0 {
		altNorm = 0
	}
	if altNorm > 1.0 {
		altNorm = 1.0
	}
	healthNorm := health / 100.0
	if healthNorm < 0 {
		healthNorm = 0
	}
	if healthNorm > 1.0 {
		healthNorm = 1.0
	}

	return weightPriority*priorityNorm + weightAltitude*altNorm + weightHealth*healthNorm
}

func (s *Scheduler) generateCandidates(startTime time.Time, horizonHours float64) []candidateSlot {
	var candidates []candidateSlot
	step := 60 * time.Minute
	horizon := startTime.Add(time.Duration(horizonHours * float64(time.Hour)))

	for _, target := range s.targets {
		for _, site := range s.sites {
			if site.Health < 10 {
				continue
			}

			for t := startTime; t.Add(target.Duration).Before(horizon); t = t.Add(step) {
				avgAlt := 0.0
				samples := 0
				valid := true
				checkStep := 15 * time.Minute

				for ct := t; ct.Before(t.Add(target.Duration)); ct = ct.Add(checkStep) {
					altAz := astronomy.EquatorialToHorizontal(ct, target.RA, target.Dec, site.Latitude, site.Longitude)
					if altAz.Altitude < 10.0 {
						valid = false
						break
					}
					avgAlt += altAz.Altitude
					samples++
				}

				if valid && samples > 0 {
					avgAlt /= float64(samples)
					score := s.computeScore(target.Priority, avgAlt, site.Health)

					candidates = append(candidates, candidateSlot{
						target:    target,
						site:      site,
						startTime: t,
						endTime:   t.Add(target.Duration),
						avgAlt:    avgAlt,
						score:     score,
					})
				}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return candidates
}

type siteSchedule struct {
	site     *model.TelescopeSite
	busy     []model.TimeWindow
}

func (ss *siteSchedule) isAvailable(start, end time.Time) bool {
	for _, b := range ss.busy {
		if start.Before(b.End) && end.After(b.Start) {
			return false
		}
	}
	return true
}

func (ss *siteSchedule) book(start, end time.Time) {
	ss.busy = append(ss.busy, model.TimeWindow{Start: start, End: end})
}

func (s *Scheduler) Schedule(startTime time.Time, horizonHours float64) []*model.ScheduledObservation {
	candidates := s.generateCandidates(startTime, horizonHours)

	siteSchedules := make(map[string]*siteSchedule)
	for _, site := range s.sites {
		siteSchedules[site.ID] = &siteSchedule{site: site}
	}

	bookedTargets := make(map[string]bool)
	var result []*model.ScheduledObservation

	for _, cand := range candidates {
		if bookedTargets[cand.target.ID] {
			continue
		}

		ss := siteSchedules[cand.site.ID]
		if ss.isAvailable(cand.startTime, cand.endTime) {
			ss.book(cand.startTime, cand.endTime)
			bookedTargets[cand.target.ID] = true

			result = append(result, &model.ScheduledObservation{
				Target:     cand.target,
				Site:       cand.site,
				Start:      cand.startTime,
				End:        cand.endTime,
				Score:      cand.score,
				AltAtStart: cand.avgAlt,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Start.Before(result[j].Start)
	})

	return result
}
