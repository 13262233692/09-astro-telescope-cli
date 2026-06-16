package timeline

import (
	"fmt"
	"strings"
	"time"

	"astro-telescope-cli/pkg/model"
)

const (
	barChar     = "█"
	emptyChar   = "·"
	hourWidth   = 3
)

func RenderSchedule(schedule []*model.ScheduledObservation, startTime time.Time, horizonHours float64) string {
	if len(schedule) == 0 {
		return "No observations scheduled.\n"
	}

	siteMap := make(map[string][]*model.ScheduledObservation)
	siteNames := make(map[string]string)
	var siteIDs []string

	for _, obs := range schedule {
		siteMap[obs.Site.ID] = append(siteMap[obs.Site.ID], obs)
		if siteNames[obs.Site.ID] == "" {
			siteNames[obs.Site.ID] = obs.Site.Name
			siteIDs = append(siteIDs, obs.Site.ID)
		}
	}

	endTime := startTime.Add(time.Duration(horizonHours * float64(time.Hour)))
	totalHours := int(horizonHours)
	totalWidth := totalHours * hourWidth

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("╔")
	sb.WriteString(strings.Repeat("═", 20+totalWidth+2))
	sb.WriteString("╗\n")

	title := fmt.Sprintf("  OBSERVATION SCHEDULE — %s to %s",
		startTime.Format("2006-01-02 15:04"),
		endTime.Format("2006-01-02 15:04"))
	sb.WriteString(fmt.Sprintf("║ %-*s ║\n", 18+totalWidth, title))
	sb.WriteString("╠")
	sb.WriteString(strings.Repeat("═", 20+totalWidth+2))
	sb.WriteString("╣\n")

	header := fmt.Sprintf("%-16s", "Site")
	for h := 0; h < totalHours; h++ {
		hour := startTime.Add(time.Duration(h) * time.Hour)
		label := hour.Format("15")
		if len(label) == 1 {
			label = " " + label
		}
		header += " " + label
	}
	sb.WriteString(fmt.Sprintf("║ %s ║\n", header))
	sb.WriteString("║")
	sb.WriteString(strings.Repeat("-", 20+totalWidth))
	sb.WriteString("║\n")

	for _, siteID := range siteIDs {
		obsList := siteMap[siteID]
		name := siteNames[siteID]
		if len(name) > 14 {
			name = name[:14]
		}

		timeline := make([]string, totalWidth)
		for i := range timeline {
			timeline[i] = emptyChar
		}

		for _, obs := range obsList {
			startMin := obs.Start.Sub(startTime).Minutes()
			endMin := obs.End.Sub(startTime).Minutes()

			startIdx := int((startMin / 60.0) * float64(hourWidth))
			endIdx := int((endMin / 60.0) * float64(hourWidth))

			if startIdx < 0 {
				startIdx = 0
			}
			if endIdx > totalWidth {
				endIdx = totalWidth
			}

			for i := startIdx; i < endIdx && i < totalWidth; i++ {
				if i >= 0 {
					timeline[i] = barChar
				}
			}
		}

		siteLine := fmt.Sprintf("%-16s%s", name, strings.Join(timeline, ""))
		sb.WriteString(fmt.Sprintf("║ %s ║\n", siteLine))
	}

	sb.WriteString("╚")
	sb.WriteString(strings.Repeat("═", 20+totalWidth+2))
	sb.WriteString("╝\n")

	sb.WriteString("\n")
	sb.WriteString("Legend:\n")
	sb.WriteString(fmt.Sprintf("  %s = Observation in progress\n", barChar))
	sb.WriteString(fmt.Sprintf("  %s = Available\n", emptyChar))
	sb.WriteString("\n")

	sb.WriteString("Scheduled Observations:\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	for i, obs := range schedule {
		sb.WriteString(fmt.Sprintf(
			"  %2d. %-15s @ %-15s  %s → %s  (Prio: %d, Alt: %.1f°, Score: %.3f)\n",
			i+1,
			obs.Target.Name,
			obs.Site.Name,
			obs.Start.Format("15:04"),
			obs.End.Format("15:04"),
			obs.Target.Priority,
			obs.AltAtStart,
			obs.Score,
		))
	}
	sb.WriteString("\n")

	return sb.String()
}
