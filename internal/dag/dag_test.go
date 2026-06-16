package dag

import (
	"testing"
	"time"

	"astro-telescope-cli/pkg/model"
)

func makeTestSite(id, name string, lat, lon, health float64) *model.TelescopeSite {
	return &model.TelescopeSite{ID: id, Name: name, Latitude: lat, Longitude: lon, Health: health}
}

func makeTestTarget(id, name string, ra, dec float64, dur time.Duration, prio int) *model.ObservationTarget {
	return &model.ObservationTarget{ID: id, Name: name, RA: ra, Dec: dec, Duration: dur, Priority: prio}
}

func TestNewObservationDAG(t *testing.T) {
	dag := NewObservationDAG()
	if len(dag.Nodes) != 0 {
		t.Error("new DAG should have no nodes")
	}
	if len(dag.Edges) != 0 {
		t.Error("new DAG should have no edges")
	}
}

func TestAddNodeAndEdge(t *testing.T) {
	dag := NewObservationDAG()

	site := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	n1 := &TaskNode{ID: "N1", Target: target, Site: site, Status: StatusPending}
	n2 := &TaskNode{ID: "N2", Target: target, Site: site, Status: StatusPending}

	dag.AddNode(n1)
	dag.AddNode(n2)
	dag.AddEdge("N1", "N2", EdgeSequential, false)

	if len(dag.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(dag.Nodes))
	}
	if len(dag.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(dag.Edges))
	}
	if len(dag.Children["N1"]) != 1 || dag.Children["N1"][0] != "N2" {
		t.Error("N1 should have N2 as child")
	}
	if len(dag.Parents["N2"]) != 1 || dag.Parents["N2"][0] != "N1" {
		t.Error("N2 should have N1 as parent")
	}
}

func TestTopologicalOrder(t *testing.T) {
	dag := NewObservationDAG()
	site := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	dag.AddNode(&TaskNode{ID: "A", Target: target, Site: site, Status: StatusPending})
	dag.AddNode(&TaskNode{ID: "B", Target: target, Site: site, Status: StatusPending})
	dag.AddNode(&TaskNode{ID: "C", Target: target, Site: site, Status: StatusPending})

	dag.AddEdge("A", "B", EdgeSequential, false)
	dag.AddEdge("A", "C", EdgeSequential, false)

	order := dag.TopologicalOrder()
	if len(order) != 3 {
		t.Fatalf("expected 3 nodes in order, got %d", len(order))
	}

	posA := -1
	for i, id := range order {
		if id == "A" {
			posA = i
		}
	}
	if posA < 0 {
		t.Error("A not found in topological order")
	}
	for i, id := range order {
		if (id == "B" || id == "C") && i < posA {
			t.Errorf("%s appears before A in topological order", id)
		}
	}
}

func TestPropagateFault(t *testing.T) {
	dag := NewObservationDAG()

	site1 := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	site2 := makeTestSite("S2", "Site2", -30.0, 21.0, 85)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	dag.AddNode(&TaskNode{ID: "N1", Target: target, Site: site1, Status: StatusPending})
	dag.AddNode(&TaskNode{ID: "N2", Target: target, Site: site1, Status: StatusPending})
	dag.AddNode(&TaskNode{ID: "N3", Target: target, Site: site2, Status: StatusPending})

	dag.AddEdge("N1", "N2", EdgeSequential, false)

	result := dag.PropagateFault("S1", "test failure")

	if len(result.CancelledNodes) != 2 {
		t.Errorf("expected 2 cancelled nodes (S1 tasks), got %d", len(result.CancelledNodes))
	}
	if len(result.HealthyNodes) != 1 {
		t.Errorf("expected 1 healthy node (S2 task), got %d", len(result.HealthyNodes))
	}

	n1, _ := dag.GetNode("N1")
	if n1.Status != StatusFailed {
		t.Errorf("N1 should be FAILED, got %s", n1.Status)
	}
	n3, _ := dag.GetNode("N3")
	if n3.Status != StatusPending {
		t.Errorf("N3 should remain PENDING, got %s", n3.Status)
	}
}

func TestPropagateFaultWithSyncEdge(t *testing.T) {
	dag := NewObservationDAG()

	site1 := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	site2 := makeTestSite("S2", "Site2", -30.0, 21.0, 85)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	dag.AddNode(&TaskNode{ID: "N1", Target: target, Site: site1, Status: StatusPending})
	dag.AddNode(&TaskNode{ID: "N2", Target: target, Site: site2, Status: StatusPending, VLBIGroup: "VLBI-1"})

	dag.AddEdge("N1", "N2", EdgeSynchronized, true)
	dag.AddEdge("N2", "N1", EdgeSynchronized, true)

	result := dag.PropagateFault("S1", "antenna failure")

	if len(result.CancelledNodes) < 2 {
		t.Errorf("with SYNC edge, both nodes should be cancelled, got %d cancelled", len(result.CancelledNodes))
	}

	n2, _ := dag.GetNode("N2")
	if n2.Status != StatusCancelled {
		t.Errorf("N2 with SYNC dependency should be CANCELLED, got %s", n2.Status)
	}
}

func TestSimulateExecution(t *testing.T) {
	dag := NewObservationDAG()
	site := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	n1 := &TaskNode{ID: "A", Target: target, Site: site, Status: StatusPending, Start: time.Now(), End: time.Now().Add(30 * time.Minute)}
	n2 := &TaskNode{ID: "B", Target: target, Site: site, Status: StatusPending, Start: time.Now(), End: time.Now().Add(30 * time.Minute), Dependencies: []string{"A"}}

	dag.AddNode(n1)
	dag.AddNode(n2)
	dag.AddEdge("A", "B", EdgeSequential, false)

	events := dag.SimulateExecution()

	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}

	if n1.Status != StatusCompleted {
		t.Errorf("A should be COMPLETED, got %s", n1.Status)
	}
	if n2.Status != StatusCompleted {
		t.Errorf("B should be COMPLETED, got %s", n2.Status)
	}
}

func TestTaskStatusString(t *testing.T) {
	statuses := []TaskStatus{StatusPending, StatusReady, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled, StatusRescheduling}
	for _, s := range statuses {
		str := s.String()
		if str == "" || str == "UNKNOWN" {
			t.Errorf("status %d should have a valid string representation", s)
		}
	}
}

func TestEdgeKindString(t *testing.T) {
	kinds := []EdgeKind{EdgeSequential, EdgeSynchronized, EdgeDataDependency}
	for _, k := range kinds {
		str := k.String()
		if str == "" || str == "???" {
			t.Errorf("edge kind %d should have a valid string representation", k)
		}
	}
}

func TestGetVLBIGroups(t *testing.T) {
	dag := NewObservationDAG()
	site := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	dag.AddNode(&TaskNode{ID: "N1", Target: target, Site: site, Status: StatusPending, VLBIGroup: "G1"})
	dag.AddNode(&TaskNode{ID: "N2", Target: target, Site: site, Status: StatusPending, VLBIGroup: "G1"})
	dag.AddNode(&TaskNode{ID: "N3", Target: target, Site: site, Status: StatusPending})

	groups := dag.GetVLBIGroups()
	if len(groups) != 1 {
		t.Errorf("expected 1 VLBI group, got %d", len(groups))
	}
	if len(groups["G1"]) != 2 {
		t.Errorf("G1 should have 2 members, got %d", len(groups["G1"]))
	}
}

func TestSiteHealthMap(t *testing.T) {
	dag := NewObservationDAG()

	site1 := makeTestSite("S1", "Site1", 34.0, -107.0, 90)
	site2 := makeTestSite("S2", "Site2", -30.0, 21.0, 85)
	target := makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 5)

	dag.AddNode(&TaskNode{ID: "N1", Target: target, Site: site1, Status: StatusFailed})
	dag.AddNode(&TaskNode{ID: "N2", Target: target, Site: site2, Status: StatusCompleted})

	health := dag.SiteHealthMap()
	if health["S1"] != false {
		t.Error("S1 should be unhealthy")
	}
	if health["S2"] != true {
		t.Error("S2 should be healthy")
	}
}

func TestBuildVLBIDAG(t *testing.T) {
	sites := []*model.TelescopeSite{
		makeTestSite("S1", "Site1", 34.0, -107.0, 90),
		makeTestSite("S2", "Site2", -30.0, 21.0, 85),
	}
	targets := []*model.ObservationTarget{
		makeTestTarget("T1", "Target1", 83.6, 22.0, 30*time.Minute, 7),
	}

	startTime := time.Now().UTC().Truncate(time.Hour)
	config := DefaultVLBIConfig()

	dag := BuildVLBIDAG(sites, targets, startTime, 8, config)

	if len(dag.Nodes) == 0 {
		t.Error("DAG should have at least some nodes")
	}

	for _, node := range dag.Nodes {
		if node.Target == nil || node.Site == nil {
			t.Error("node should have target and site")
		}
	}
}
