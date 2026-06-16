package dag

import (
	"fmt"
	"sort"
	"time"

	"astro-telescope-cli/internal/astronomy"
	"astro-telescope-cli/internal/scheduler"
	"astro-telescope-cli/pkg/model"
)

type VLBIConfig struct {
	MinStations    int
	SyncTolerance  time.Duration
	PriorityBoost  float64
}

func DefaultVLBIConfig() VLBIConfig {
	return VLBIConfig{
		MinStations:   2,
		SyncTolerance: 5 * time.Minute,
		PriorityBoost: 1.5,
	}
}

type VLBITarget struct {
	Target     *model.ObservationTarget
	Stations   []*model.TelescopeSite
	StartTime  time.Time
	EndTime    time.Time
	GroupID    string
}

func FindVLBIOpportunities(
	sites []*model.TelescopeSite,
	targets []*model.ObservationTarget,
	startTime time.Time,
	horizonHours float64,
	config VLBIConfig,
) []VLBITarget {
	horizon := startTime.Add(time.Duration(horizonHours * float64(time.Hour)))
	step := 30 * time.Minute
	var opportunities []VLBITarget

	for _, target := range targets {
		bestCount := 0
		var bestOp *VLBITarget

		for t := startTime; t.Add(target.Duration).Before(horizon); t = t.Add(step) {
			var availableSites []*model.TelescopeSite
			for _, site := range sites {
				if site.Health < 10 {
					continue
				}

				valid := true
				for ct := t; ct.Before(t.Add(target.Duration)); ct = ct.Add(10 * time.Minute) {
					altAz := astronomy.EquatorialToHorizontal(ct, target.RA, target.Dec, site.Latitude, site.Longitude)
					if altAz.Altitude < 10.0 {
						valid = false
						break
					}
				}

				if valid {
					availableSites = append(availableSites, site)
				}
			}

			if len(availableSites) >= config.MinStations && len(availableSites) > bestCount {
				groupID := fmt.Sprintf("VLBI-%s-%s", target.ID, t.Format("1504"))
				op := VLBITarget{
					Target:    target,
					Stations:  availableSites,
					StartTime: t,
					EndTime:   t.Add(target.Duration),
					GroupID:   groupID,
				}
				bestCount = len(availableSites)
				bestOp = &op
			}
		}

		if bestOp != nil {
			opportunities = append(opportunities, *bestOp)
		}
	}

	sort.Slice(opportunities, func(i, j int) bool {
		iStations := len(opportunities[i].Stations)
		jStations := len(opportunities[j].Stations)
		if iStations != jStations {
			return iStations > jStations
		}
		return opportunities[i].Target.Priority > opportunities[j].Target.Priority
	})

	return opportunities
}

func BuildVLBIDAG(
	sites []*model.TelescopeSite,
	targets []*model.ObservationTarget,
	startTime time.Time,
	horizonHours float64,
	config VLBIConfig,
) *ObservationDAG {
	dag := NewObservationDAG()

	sched := scheduler.NewScheduler(sites, targets)
	regularObs := sched.Schedule(startTime, horizonHours)

	for _, obs := range regularObs {
		nodeID := fmt.Sprintf("OBS-%s-%s", obs.Target.ID, obs.Site.ID)
		node := &TaskNode{
			ID:        nodeID,
			Target:    obs.Target,
			Site:      obs.Site,
			Start:     obs.Start,
			End:       obs.End,
			Status:    StatusPending,
			Score:     obs.Score,
		}
		dag.AddNode(node)
	}

	vlbiOps := FindVLBIOpportunities(sites, targets, startTime, horizonHours, config)

	bookedSites := make(map[string][]model.TimeWindow)
	for _, obs := range regularObs {
		bookedSites[obs.Site.ID] = append(bookedSites[obs.Site.ID], model.TimeWindow{
			Start: obs.Start,
			End:   obs.End,
		})
	}

	vlbiBooked := make(map[string]bool)

	for _, vlbi := range vlbiOps {
		if vlbiBooked[vlbi.Target.ID] {
			continue
		}

		var nodeIDs []string
		conflict := false

		for _, site := range vlbi.Stations {
			nodeID := fmt.Sprintf("VLBI-%s-%s", vlbi.GroupID, site.ID)
			for _, w := range bookedSites[site.ID] {
				if vlbi.StartTime.Before(w.End) && vlbi.EndTime.After(w.Start) {
					conflict = true
					break
				}
			}
			if conflict {
				break
			}

			node := &TaskNode{
				ID:        nodeID,
				Target:    vlbi.Target,
				Site:      site,
				Start:     vlbi.StartTime,
				End:       vlbi.EndTime,
				Status:    StatusPending,
				VLBIGroup: vlbi.GroupID,
				Score:     float64(vlbi.Target.Priority) * config.PriorityBoost,
			}
			dag.AddNode(node)
			nodeIDs = append(nodeIDs, nodeID)

			bookedSites[site.ID] = append(bookedSites[site.ID], model.TimeWindow{
				Start: vlbi.StartTime,
				End:   vlbi.EndTime,
			})
		}

		if conflict || len(nodeIDs) < config.MinStations {
			for _, nid := range nodeIDs {
				delete(dag.Nodes, nid)
			}
			continue
		}

		for i := 0; i < len(nodeIDs); i++ {
			for j := i + 1; j < len(nodeIDs); j++ {
				dag.AddEdge(nodeIDs[i], nodeIDs[j], EdgeSynchronized, true)
				dag.AddEdge(nodeIDs[j], nodeIDs[i], EdgeSynchronized, true)
			}
		}

		vlbiBooked[vlbi.Target.ID] = true
	}

	addSequentialEdges(dag)

	return dag
}

func addSequentialEdges(dag *ObservationDAG) {
	siteTasks := make(map[string][]*TaskNode)

	for _, node := range dag.Nodes {
		siteTasks[node.Site.ID] = append(siteTasks[node.Site.ID], node)
	}

	for _, tasks := range siteTasks {
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].Start.Before(tasks[j].Start)
		})

		for i := 1; i < len(tasks); i++ {
			prevID := tasks[i-1].ID
			currID := tasks[i].ID

			alreadyExists := false
			for _, child := range dag.Children[prevID] {
				if child == currID {
					alreadyExists = true
					break
				}
			}

			if !alreadyExists {
				dag.AddEdge(prevID, currID, EdgeSequential, false)
				tasks[i].Dependencies = append(tasks[i].Dependencies, prevID)
			}
		}
	}
}
