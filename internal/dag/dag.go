package dag

import (
	"fmt"
	"sync"
	"time"

	"astro-telescope-cli/pkg/model"
)

type TaskStatus int

const (
	StatusPending TaskStatus = iota
	StatusReady
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusCancelled
	StatusRescheduling
)

func (s TaskStatus) String() string {
	switch s {
	case StatusPending:
		return "PENDING"
	case StatusReady:
		return "READY"
	case StatusRunning:
		return "RUNNING"
	case StatusCompleted:
		return "COMPLETED"
	case StatusFailed:
		return "FAILED"
	case StatusCancelled:
		return "CANCELLED"
	case StatusRescheduling:
		return "RESCHED"
	default:
		return "UNKNOWN"
	}
}

func (s TaskStatus) Symbol() string {
	switch s {
	case StatusPending:
		return "○"
	case StatusReady:
		return "◎"
	case StatusRunning:
		return "●"
	case StatusCompleted:
		return "✓"
	case StatusFailed:
		return "✗"
	case StatusCancelled:
		return "⊘"
	case StatusRescheduling:
		return "↻"
	default:
		return "?"
	}
}

type EdgeKind int

const (
	EdgeSequential EdgeKind = iota
	EdgeSynchronized
	EdgeDataDependency
)

func (e EdgeKind) String() string {
	switch e {
	case EdgeSequential:
		return "SEQ"
	case EdgeSynchronized:
		return "SYNC"
	case EdgeDataDependency:
		return "DATA"
	default:
		return "???"
	}
}

type TaskNode struct {
	ID           string
	Target       *model.ObservationTarget
	Site         *model.TelescopeSite
	Start        time.Time
	End          time.Time
	Status       TaskStatus
	Dependencies []string
	VLBIGroup    string
	Score        float64
	FailReason   string
}

type TaskEdge struct {
	From     string
	To       string
	Kind     EdgeKind
	Critical bool
}

type ObservationDAG struct {
	mu       sync.RWMutex
	Nodes    map[string]*TaskNode
	Edges    []TaskEdge
	Children map[string][]string
	Parents  map[string][]string
}

func NewObservationDAG() *ObservationDAG {
	return &ObservationDAG{
		Nodes:    make(map[string]*TaskNode),
		Children: make(map[string][]string),
		Parents:  make(map[string][]string),
	}
}

func (d *ObservationDAG) AddNode(node *TaskNode) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Nodes[node.ID] = node
}

func (d *ObservationDAG) AddEdge(from, to string, kind EdgeKind, critical bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Edges = append(d.Edges, TaskEdge{From: from, To: to, Kind: kind, Critical: critical})
	d.Children[from] = append(d.Children[from], to)
	d.Parents[to] = append(d.Parents[to], from)
}

func (d *ObservationDAG) GetNode(id string) (*TaskNode, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	n, ok := d.Nodes[id]
	return n, ok
}

func (d *ObservationDAG) TopologicalOrder() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	inDegree := make(map[string]int)
	for id := range d.Nodes {
		inDegree[id] = 0
	}
	for _, e := range d.Edges {
		inDegree[e.To]++
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, child := range d.Children[curr] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	return order
}

type FaultEvent struct {
	Timestamp time.Time
	SiteID    string
	Reason    string
	Severity  string
}

type PropagationResult struct {
	CancelledNodes []string
	ReschedNodes   []string
	HealthyNodes   []string
	Events         []PropagationEvent
}

type PropagationEvent struct {
	Timestamp time.Time
	NodeID    string
	OldStatus TaskStatus
	NewStatus TaskStatus
	Reason    string
	Depth     int
}

func (d *ObservationDAG) PropagateFault(siteID string, reason string) PropagationResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	result := PropagationResult{}

	var failedNodes []string
	for id, node := range d.Nodes {
		if node.Site.ID == siteID && (node.Status == StatusPending || node.Status == StatusReady || node.Status == StatusRunning) {
			node.Status = StatusFailed
			node.FailReason = reason
			failedNodes = append(failedNodes, id)
			result.CancelledNodes = append(result.CancelledNodes, id)
			result.Events = append(result.Events, PropagationEvent{
				Timestamp: now,
				NodeID:    id,
				OldStatus: StatusRunning,
				NewStatus: StatusFailed,
				Reason:    fmt.Sprintf("Site %s fault: %s", siteID, reason),
				Depth:     0,
			})
		}
	}

	queue := make([]struct {
		nodeID string
		depth  int
	}, 0)
	for _, nid := range failedNodes {
		queue = append(queue, struct {
			nodeID string
			depth  int
		}{nid, 0})
	}

	visited := make(map[string]bool)
	for _, nid := range failedNodes {
		visited[nid] = true
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, childID := range d.Children[curr.nodeID] {
			if visited[childID] {
				continue
			}
			visited[childID] = true

			childNode := d.Nodes[childID]

			hasSyncDep := false
			for _, parentID := range d.Parents[childID] {
				for _, edge := range d.Edges {
					if edge.From == parentID && edge.To == childID && edge.Kind == EdgeSynchronized && edge.Critical {
						hasSyncDep = true
						break
					}
				}
				if hasSyncDep {
					break
				}
			}

			allParentsFailed := true
			for _, parentID := range d.Parents[childID] {
				parentNode := d.Nodes[parentID]
				if parentNode.Status != StatusFailed && parentNode.Status != StatusCancelled {
					allParentsFailed = false
					break
				}
			}

			oldStatus := childNode.Status

			if hasSyncDep || allParentsFailed {
				childNode.Status = StatusCancelled
				childNode.FailReason = fmt.Sprintf("Cascading from %s", curr.nodeID)
				result.CancelledNodes = append(result.CancelledNodes, childID)
				queue = append(queue, struct {
					nodeID string
					depth  int
				}{childID, curr.depth + 1})
			} else {
				childNode.Status = StatusRescheduling
				childNode.FailReason = fmt.Sprintf("Rescheduling due to %s fault", siteID)
				result.ReschedNodes = append(result.ReschedNodes, childID)
			}

			result.Events = append(result.Events, PropagationEvent{
				Timestamp: now,
				NodeID:    childID,
				OldStatus: oldStatus,
				NewStatus: childNode.Status,
				Reason:    childNode.FailReason,
				Depth:     curr.depth + 1,
			})
		}
	}

	for id, node := range d.Nodes {
		if node.Status != StatusFailed && node.Status != StatusCancelled && node.Status != StatusRescheduling {
			result.HealthyNodes = append(result.HealthyNodes, id)
		}
	}

	return result
}

func (d *ObservationDAG) MarkReady() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, node := range d.Nodes {
		if node.Status != StatusPending {
			continue
		}
		allDepsComplete := true
		for _, depID := range node.Dependencies {
			depNode, ok := d.Nodes[depID]
			if !ok || depNode.Status != StatusCompleted {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			node.Status = StatusReady
		}
	}
}

func (d *ObservationDAG) SimulateExecution() []PropagationEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	inDegree := make(map[string]int)
	for id := range d.Nodes {
		inDegree[id] = 0
	}
	for _, e := range d.Edges {
		inDegree[e.To]++
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)
		for _, child := range d.Children[curr] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	var events []PropagationEvent
	now := time.Now()

	for _, id := range order {
		node := d.Nodes[id]
		if node.Status == StatusPending || node.Status == StatusReady {
			allDepsDone := true
			for _, depID := range node.Dependencies {
				depNode, ok := d.Nodes[depID]
				if !ok || (depNode.Status != StatusCompleted) {
					allDepsDone = false
					break
				}
			}
			if allDepsDone {
				oldStatus := node.Status
				node.Status = StatusRunning
				events = append(events, PropagationEvent{
					Timestamp: now,
					NodeID:    id,
					OldStatus: oldStatus,
					NewStatus: StatusRunning,
					Reason:    "Dependencies satisfied",
					Depth:     0,
				})
				node.Status = StatusCompleted
				events = append(events, PropagationEvent{
					Timestamp: now.Add(node.End.Sub(node.Start)),
					NodeID:    id,
					OldStatus: StatusRunning,
					NewStatus: StatusCompleted,
					Reason:    "Observation complete",
					Depth:     0,
				})
			}
		}
	}

	return events
}

func (d *ObservationDAG) GetVLBIGroups() map[string][]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	groups := make(map[string][]string)
	for id, node := range d.Nodes {
		if node.VLBIGroup != "" {
			groups[node.VLBIGroup] = append(groups[node.VLBIGroup], id)
		}
	}
	return groups
}

func (d *ObservationDAG) SiteHealthMap() map[string]bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	health := make(map[string]bool)
	for _, node := range d.Nodes {
		healthy := node.Status != StatusFailed && node.Status != StatusCancelled
		if _, ok := health[node.Site.ID]; ok {
			if !healthy {
				health[node.Site.ID] = false
			}
		} else {
			health[node.Site.ID] = healthy
		}
	}
	return health
}
