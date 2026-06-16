package dag

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var UseColor = false

func c(code, text string) string {
	if UseColor {
		return code + text + colorReset
	}
	return text
}

func cb(text string) string {
	if UseColor {
		return colorBold + text + colorReset
	}
	return text
}

const (
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorBold    = "\033[1m"
	colorReset   = "\033[0m"
)

func statusColor(s TaskStatus) string {
	if !UseColor {
		return ""
	}
	switch s {
	case StatusPending:
		return colorBlue
	case StatusReady:
		return colorCyan
	case StatusRunning:
		return colorGreen
	case StatusCompleted:
		return colorGreen
	case StatusFailed:
		return colorRed
	case StatusCancelled:
		return colorMagenta
	case StatusRescheduling:
		return colorYellow
	default:
		return ""
	}
}

func edgeSymbol(kind EdgeKind, critical bool) string {
	if critical {
		switch kind {
		case EdgeSynchronized:
			return "══"
		case EdgeDataDependency:
			return "──"
		case EdgeSequential:
			return "──"
		}
	}
	switch kind {
	case EdgeSynchronized:
		return "──"
	case EdgeDataDependency:
		return "╌╌"
	case EdgeSequential:
		return "╌╌"
	}
	return "──"
}

func RenderDAGTree(dag *ObservationDAG) string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n%sTASK DAG — Topology Tree%s\n", cb(""), ""))
	if UseColor {
		sb.WriteString(strings.Repeat("═", 70) + "\n\n")
	} else {
		sb.WriteString(strings.Repeat("=", 70) + "\n\n")
	}

	inDegree := make(map[string]int)
	for id := range dag.Nodes {
		inDegree[id] = 0
	}
	for _, e := range dag.Edges {
		inDegree[e.To]++
	}

	var roots []string
	for id, deg := range inDegree {
		if deg == 0 {
			roots = append(roots, id)
		}
	}
	sort.Strings(roots)

	visited := make(map[string]bool)

	var renderNode func(id string, prefix string, isLast bool, depth int)
	renderNode = func(id string, prefix string, isLast bool, depth int) {
		if visited[id] {
			return
		}
		visited[id] = true

		node, ok := dag.Nodes[id]
		if !ok {
			return
		}

		connector := "+-- "
		if isLast {
			connector = "\\-- "
		}
		if depth == 0 {
			connector = ""
		}

		vlbiTag := ""
		if node.VLBIGroup != "" {
			vlbiTag = fmt.Sprintf(" [VLBI:%s]", node.VLBIGroup)
		}

		timeStr := fmt.Sprintf("%s->%s", node.Start.Format("15:04"), node.End.Format("15:04"))

		sc := statusColor(node.Status)
		statusStr := node.Status.String()

		sb.WriteString(fmt.Sprintf("%s%s%s%s %-12s @ %-14s %s  %s%s\n",
			prefix, connector,
			c(sc, node.Status.Symbol()),
			statusStr,
			node.Target.Name,
			node.Site.Name,
			timeStr,
			c(sc, statusStr),
			vlbiTag,
		))

		children := dag.Children[id]
		sort.Strings(children)

		childPrefix := prefix
		if depth > 0 {
			if isLast {
				childPrefix += "    "
			} else {
				childPrefix += "|   "
			}
		}

		for i, childID := range children {
			isChildLast := i == len(children)-1
			renderNode(childID, childPrefix, isChildLast, depth+1)
		}
	}

	for i, rootID := range roots {
		renderNode(rootID, "", i == len(roots)-1, 0)
	}

	for id := range dag.Nodes {
		if !visited[id] {
			node := dag.Nodes[id]
			sc := statusColor(node.Status)
			sb.WriteString(fmt.Sprintf("  %s %s %-12s @ %-14s  %s\n",
				c(sc, node.Status.Symbol()),
				node.Target.Name,
				node.Site.Name,
				fmt.Sprintf("%s->%s", node.Start.Format("15:04"), node.End.Format("15:04")),
				c(sc, node.Status.String()),
			))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

func RenderDAGProgressBar(dag *ObservationDAG) string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder

	total := len(dag.Nodes)
	if total == 0 {
		return "No tasks in DAG.\n"
	}

	counts := make(map[TaskStatus]int)
	for _, node := range dag.Nodes {
		counts[node.Status]++
	}

	width := 40

	completed := counts[StatusCompleted]
	running := counts[StatusRunning]
	failed := counts[StatusFailed]
	cancelled := counts[StatusCancelled]
	resched := counts[StatusRescheduling]
	ready := counts[StatusReady]

	cW := int(float64(completed) / float64(total) * float64(width))
	rW := int(float64(running) / float64(total) * float64(width))
	fW := int(float64(failed+cancelled) / float64(total) * float64(width))
	sW := int(float64(resched) / float64(total) * float64(width))
	rdW := width - cW - rW - fW - sW
	if rdW < 0 {
		rdW = 0
	}

	bar := ""
	bar += c(colorGreen, strings.Repeat("#", cW))
	bar += c(colorYellow, strings.Repeat("=", rW))
	bar += c(colorRed, strings.Repeat("X", fW))
	bar += c(colorMagenta, strings.Repeat("~", sW))
	bar += c(colorBlue, strings.Repeat(".", rdW))

	sb.WriteString(fmt.Sprintf("\nDAG Progress  [%s]  %d/%d tasks\n",
		bar,
		completed+running, total))

	sb.WriteString(fmt.Sprintf("  [OK] Completed:%d  [==] Running:%d  [XX] Failed:%d  [~~] Cancelled:%d  [~~] Resched:%d  [..] Ready:%d\n",
		completed, running, failed, cancelled, resched, ready))

	sb.WriteString("\n")
	return sb.String()
}

func RenderVLBIGroups(dag *ObservationDAG) string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder

	groups := make(map[string][]*TaskNode)
	for _, node := range dag.Nodes {
		if node.VLBIGroup != "" {
			groups[node.VLBIGroup] = append(groups[node.VLBIGroup], node)
		}
	}

	if len(groups) == 0 {
		return "No VLBI groups in DAG.\n"
	}

	sb.WriteString("VLBI Synchronized Groups\n")
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	for groupID, nodes := range groups {
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Site.ID < nodes[j].Site.ID
		})

		allComplete := true
		anyFailed := false
		for _, n := range nodes {
			if n.Status != StatusCompleted {
				allComplete = false
			}
			if n.Status == StatusFailed || n.Status == StatusCancelled {
				anyFailed = true
			}
		}

		statusTag := c(colorGreen, "[SYNCED]")
		if anyFailed {
			statusTag = c(colorRed, "[BROKEN]")
		} else if !allComplete {
			statusTag = c(colorYellow, "[WAITING]")
		}

		sb.WriteString(fmt.Sprintf("  %s  %s  [%d stations]\n", groupID, statusTag, len(nodes)))
		for _, n := range nodes {
			sc := statusColor(n.Status)
			sb.WriteString(fmt.Sprintf("    %s %-12s @ %s->%s  %s\n",
				c(sc, n.Status.Symbol()),
				n.Site.Name,
				n.Start.Format("15:04"),
				n.End.Format("15:04"),
				c(sc, n.Status.String()),
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func RenderFaultPropagation(result PropagationResult) string {
	var sb strings.Builder

	sb.WriteString("\n!! FAULT PROPAGATION REPORT !!\n")
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	sb.WriteString(fmt.Sprintf("  [XX] Cancelled Tasks: %d\n", len(result.CancelledNodes)))
	sb.WriteString(fmt.Sprintf("  [~~] Rescheduling:    %d\n", len(result.ReschedNodes)))
	sb.WriteString(fmt.Sprintf("  [OK] Healthy:         %d\n\n", len(result.HealthyNodes)))

	if len(result.Events) > 0 {
		sb.WriteString("  Propagation Trace:\n")
		sb.WriteString(strings.Repeat("-", 60) + "\n")

		sort.Slice(result.Events, func(i, j int) bool {
			return result.Events[i].Depth < result.Events[j].Depth
		})

		for _, evt := range result.Events {
			indent := strings.Repeat("  ", evt.Depth)
			arrow := "->"
			if evt.Depth > 0 {
				arrow = "=>"
			}

			sb.WriteString(fmt.Sprintf("  %s %s %-16s  %s -> %s  %s\n",
				indent, arrow,
				evt.NodeID,
				evt.OldStatus, evt.NewStatus,
				evt.Reason,
			))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

func RenderRescheduleProgress(dag *ObservationDAG, reschedNodes []string, step int, totalSteps int) string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder

	pct := 0
	if totalSteps > 0 {
		pct = step * 100 / totalSteps
	}

	barWidth := 30
	filled := pct * barWidth / 100
	bar := strings.Repeat("#", filled) + strings.Repeat(".", barWidth-filled)

	sb.WriteString(fmt.Sprintf("\r  [~~] Rescheduling: [%s] %3d%%  ", bar, pct))

	if step < totalSteps {
		for _, nid := range reschedNodes {
			node, ok := dag.Nodes[nid]
			if ok && node.Status == StatusRescheduling {
				sb.WriteString(fmt.Sprintf("%s~ ", node.Target.Name))
			}
		}
	} else {
		sb.WriteString("[OK] Complete")
	}

	return sb.String()
}

func RenderSiteHealthBar(dag *ObservationDAG, sites []string) string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString("  Site Health Monitor\n")
	sb.WriteString("  ")

	siteStatus := make(map[string]TaskStatus)
	for _, node := range dag.Nodes {
		current, exists := siteStatus[node.Site.ID]
		if !exists || node.Status == StatusFailed || node.Status == StatusCancelled {
			if node.Status == StatusFailed || node.Status == StatusCancelled {
				siteStatus[node.Site.ID] = node.Status
			} else if current != StatusFailed && current != StatusCancelled {
				siteStatus[node.Site.ID] = node.Status
			}
		}
	}

	for _, siteID := range sites {
		status, ok := siteStatus[siteID]
		if !ok {
			status = StatusCompleted
		}

		label := siteID
		if len(label) > 6 {
			label = label[:6]
		}

		sc := statusColor(status)
		sb.WriteString(fmt.Sprintf("%s[%-6s%s]%s ", sc, label, c(sc, status.Symbol()), ""))
	}

	sb.WriteString("\n")
	return sb.String()
}

func RenderTimeline(dag *ObservationDAG, startTime time.Time, horizonHours float64) string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	var sb strings.Builder

	totalHours := int(horizonHours)
	hourWidth := 3
	totalWidth := totalHours * hourWidth

	siteMap := make(map[string][]*TaskNode)
	siteNames := make(map[string]string)
	var siteIDs []string

	for _, node := range dag.Nodes {
		siteMap[node.Site.ID] = append(siteMap[node.Site.ID], node)
		if siteNames[node.Site.ID] == "" {
			siteNames[node.Site.ID] = node.Site.Name
			siteIDs = append(siteIDs, node.Site.ID)
		}
	}

	sb.WriteString("\nDAG Timeline\n")
	sb.WriteString("  ")

	header := fmt.Sprintf("%-16s", "Site")
	for h := 0; h < totalHours; h++ {
		hour := startTime.Add(time.Duration(h) * time.Hour)
		label := hour.Format("15")
		if len(label) == 1 {
			label = " " + label
		}
		header += " " + label
	}
	sb.WriteString(header + "\n")

	for _, siteID := range siteIDs {
		nodes := siteMap[siteID]
		name := siteNames[siteID]
		if len(name) > 14 {
			name = name[:14]
		}

		timeline := make([]string, totalWidth)
		for i := range timeline {
			timeline[i] = "."
		}

		for _, node := range nodes {
			startMin := node.Start.Sub(startTime).Minutes()
			endMin := node.End.Sub(startTime).Minutes()

			startIdx := int((startMin / 60.0) * float64(hourWidth))
			endIdx := int((endMin / 60.0) * float64(hourWidth))

			if startIdx < 0 {
				startIdx = 0
			}
			if endIdx > totalWidth {
				endIdx = totalWidth
			}

			ch := "#"
			switch node.Status {
			case StatusFailed:
				ch = "X"
			case StatusCancelled:
				ch = "/"
			case StatusRescheduling:
				ch = "~"
			case StatusRunning:
				ch = "="
			}

			for i := startIdx; i < endIdx && i < totalWidth; i++ {
				if i >= 0 {
					timeline[i] = ch
				}
			}
		}

		sb.WriteString(fmt.Sprintf("  %-16s%s\n", name, strings.Join(timeline, "")))
	}

	sb.WriteString("\n  Legend: #=OK  ==Running  X=Failed  /=Cancelled  ~=Resched  .=Free\n")
	return sb.String()
}
