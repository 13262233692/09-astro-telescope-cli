package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"astro-telescope-cli/internal/astronomy"
	"astro-telescope-cli/internal/dag"
	"astro-telescope-cli/internal/fits"
	"astro-telescope-cli/internal/scheduler"
	"astro-telescope-cli/internal/timeline"
	"astro-telescope-cli/pkg/model"
)

func defaultSites() []*model.TelescopeSite {
	return []*model.TelescopeSite{
		{ID: "VLA", Name: "VLA (New Mexico)", Latitude: 34.0784, Longitude: -107.6184, Elevation: 2124, Health: 95.0},
		{ID: "GBT", Name: "GBT (West Virginia)", Latitude: 38.4331, Longitude: -79.8398, Elevation: 824, Health: 88.0},
		{ID: "Arecibo", Name: "Arecibo (Puerto Rico)", Latitude: 18.3464, Longitude: -66.7528, Elevation: 498, Health: 72.0},
		{ID: "Meerkat", Name: "MeerKAT (South Africa)", Latitude: -30.7137, Longitude: 21.4430, Elevation: 1086, Health: 91.0},
		{ID: "ASKAP", Name: "ASKAP (Australia)", Latitude: -26.6969, Longitude: 116.6367, Elevation: 377, Health: 85.0},
		{ID: "Effelsberg", Name: "Effelsberg (Germany)", Latitude: 50.5248, Longitude: 6.8828, Elevation: 319, Health: 78.0},
	}
}

func defaultTargets() []*model.ObservationTarget {
	return []*model.ObservationTarget{
		{ID: "M31", Name: "Andromeda Galaxy", RA: 10.6847, Dec: 41.2688, Duration: 60 * time.Minute, Priority: 8},
		{ID: "Crab", Name: "Crab Nebula", RA: 83.6331, Dec: 22.0145, Duration: 45 * time.Minute, Priority: 7},
		{ID: "M42", Name: "Orion Nebula", RA: 83.8221, Dec: -5.3911, Duration: 30 * time.Minute, Priority: 6},
		{ID: "CenA", Name: "Centaurus A", RA: 201.3651, Dec: -43.0193, Duration: 90 * time.Minute, Priority: 9},
		{ID: "M51", Name: "Whirlpool Galaxy", RA: 202.4695, Dec: 47.1953, Duration: 50 * time.Minute, Priority: 5},
		{ID: "VirA", Name: "Virgo A (M87)", RA: 187.7059, Dec: 12.3911, Duration: 75 * time.Minute, Priority: 8},
		{ID: "CasA", Name: "Cassiopeia A", RA: 350.8664, Dec: 58.8117, Duration: 40 * time.Minute, Priority: 7},
		{ID: "NGC5128", Name: "NGC 5128", RA: 201.3651, Dec: -43.0193, Duration: 60 * time.Minute, Priority: 4},
	}
}

func parseTargetsFile(path string) ([]*model.ObservationTarget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var targets []*model.ObservationTarget
	lines := strings.Split(string(data), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 5 {
			fmt.Printf("Warning: line %d has insufficient fields, skipping: %s\n", i+1, line)
			continue
		}

		ra, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			fmt.Printf("Warning: line %d invalid RA: %s\n", i+1, parts[1])
			continue
		}

		dec, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			fmt.Printf("Warning: line %d invalid Dec: %s\n", i+1, parts[2])
			continue
		}

		durMin, err := strconv.Atoi(parts[3])
		if err != nil {
			fmt.Printf("Warning: line %d invalid duration: %s\n", i+1, parts[3])
			continue
		}

		prio, err := strconv.Atoi(parts[4])
		if err != nil {
			fmt.Printf("Warning: line %d invalid priority: %s\n", i+1, parts[4])
			continue
		}

		targets = append(targets, &model.ObservationTarget{
			ID:       fmt.Sprintf("T%d", i+1),
			Name:     parts[0],
			RA:       ra,
			Dec:      dec,
			Duration: time.Duration(durMin) * time.Minute,
			Priority: prio,
		})
	}

	return targets, nil
}

func cmdParseFITS(path string) {
	fmt.Printf("Parsing FITS header: %s\n\n", path)

	hdr, err := fits.ParseHeaderFile(path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("FITS Header Cards:")
	fmt.Println(strings.Repeat("-", 80))
	for _, card := range hdr.Cards {
		if card.Keyword == "END" {
			continue
		}
		if card.Comment != "" {
			fmt.Printf("  %-8s = %-20s / %s\n", card.Keyword, card.Value, card.Comment)
		} else {
			fmt.Printf("  %-8s = %s\n", card.Keyword, card.Value)
		}
	}

	fmt.Println()
	fmt.Println("Extracted Calibration Parameters:")
	fmt.Println(strings.Repeat("-", 80))
	params, err := hdr.ToCalibrationParams()
	if err != nil {
		fmt.Printf("Error extracting calibration params: %v\n", err)
	} else {
		fmt.Printf("  Observation Date: %s\n", params.ObsDate.Format(time.RFC3339))
		fmt.Printf("  Telescope:        %s\n", params.Telescope)
		fmt.Printf("  Instrument:       %s\n", params.Instrument)
		fmt.Printf("  Exposure Time:    %.2f s\n", params.Exposure)
		fmt.Printf("  Gain:             %.2f e-/ADU\n", params.Gain)
		fmt.Printf("  Temperature:      %.2f °C\n", params.Temp)
		if len(params.Custom) > 0 {
			fmt.Println("  Custom Fields:")
			for k, v := range params.Custom {
				fmt.Printf("    %-16s = %s\n", k, v)
			}
		}
	}
}

func cmdVisibility(siteID string, targetID string, sites []*model.TelescopeSite, targets []*model.ObservationTarget) {
	var site *model.TelescopeSite
	for _, s := range sites {
		if s.ID == siteID || s.Name == siteID {
			site = s
			break
		}
	}
	if site == nil {
		fmt.Printf("Site not found: %s\n", siteID)
		os.Exit(1)
	}

	var target *model.ObservationTarget
	for _, t := range targets {
		if t.ID == targetID || t.Name == targetID {
			target = t
			break
		}
	}
	if target == nil {
		fmt.Printf("Target not found: %s\n", targetID)
		os.Exit(1)
	}

	startTime := time.Now().UTC().Truncate(time.Hour)
	duration := 24 * time.Hour

	fmt.Printf("Computing visibility for %s from %s\n", target.Name, site.Name)
	fmt.Printf("  Time range: %s to %s\n\n",
		startTime.Format("2006-01-02 15:04 UTC"),
		startTime.Add(duration).Format("2006-01-02 15:04 UTC"))

	vis := astronomy.ComputeVisibility(target, site, startTime, duration)

	fmt.Printf("Maximum Altitude: %.2f° at %s\n", vis.MaxAlt, vis.BestTime.Format("2006-01-02 15:04 UTC"))
	fmt.Printf("Observable Time Windows (altitude >= 10°):\n")
	if len(vis.Windows) == 0 {
		fmt.Println("  (none)")
	} else {
		for i, w := range vis.Windows {
			fmt.Printf("  Window %d: %s → %s (%s)\n",
				i+1,
				w.Start.Format("15:04"),
				w.End.Format("15:04"),
				w.End.Sub(w.Start).Round(time.Minute))
		}
	}

	fmt.Println()
	fmt.Println("Altitude trace (hour: altitude):")
	for h := 0; h < 24; h += 2 {
		t := startTime.Add(time.Duration(h) * time.Hour)
		altAz := astronomy.EquatorialToHorizontal(t, target.RA, target.Dec, site.Latitude, site.Longitude)
		barLen := 0
		if altAz.Altitude > 0 {
			barLen = int(altAz.Altitude / 2)
		}
		marker := " "
		if altAz.Altitude >= 10 {
			marker = "*"
		}
		fmt.Printf("  %02d:00  %5.1f° %s%s\n",
			h, altAz.Altitude,
			strings.Repeat("█", barLen),
			marker)
	}
}

func cmdSchedule(sites []*model.TelescopeSite, targets []*model.ObservationTarget, horizonHours float64) {
	startTime := time.Now().UTC().Truncate(time.Hour)

	fmt.Printf("Running observation scheduler...\n")
	fmt.Printf("  Start time:    %s\n", startTime.Format("2006-01-02 15:04 UTC"))
	fmt.Printf("  Horizon:       %.0f hours\n", horizonHours)
	fmt.Printf("  Telescopes:    %d\n", len(sites))
	fmt.Printf("  Targets:       %d\n", len(targets))
	fmt.Println()

	sched := scheduler.NewScheduler(sites, targets)
	result := sched.Schedule(startTime, horizonHours)

	fmt.Printf("Scheduled %d of %d targets\n\n", len(result), len(targets))

	chart := timeline.RenderSchedule(result, startTime, horizonHours)
	fmt.Print(chart)

	if len(result) < len(targets) {
		scheduled := make(map[string]bool)
		for _, r := range result {
			scheduled[r.Target.ID] = true
		}
		fmt.Println("Unscheduled targets:")
		for _, t := range targets {
			if !scheduled[t.ID] {
				fmt.Printf("  - %s (Prio: %d, Duration: %s)\n",
					t.Name, t.Priority, t.Duration)
			}
		}
		fmt.Println()
	}
}

func cmdGenSample(path string) {
	err := fits.GenerateSampleFITSHeader(path)
	if err != nil {
		fmt.Printf("Error generating sample: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated sample FITS header file: %s\n", path)
}

func cmdListSites(sites []*model.TelescopeSite) {
	fmt.Println("Available Telescope Sites:")
	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("  %-10s %-25s %10s %11s %10s %8s\n",
		"ID", "Name", "Lat(°)", "Lon(°)", "Elev(m)", "Health%")
	fmt.Println(strings.Repeat("-", 90))
	for _, s := range sites {
		fmt.Printf("  %-10s %-25s %10.4f %11.4f %10.1f %8.1f\n",
			s.ID, s.Name, s.Latitude, s.Longitude, s.Elevation, s.Health)
	}
	fmt.Println()
}

func cmdListTargets(targets []*model.ObservationTarget) {
	fmt.Println("Available Observation Targets:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("  %-10s %-25s %10s %10s %10s %8s\n",
		"ID", "Name", "RA(°)", "Dec(°)", "Duration", "Priority")
	fmt.Println(strings.Repeat("-", 80))
	for _, t := range targets {
		fmt.Printf("  %-10s %-25s %10.4f %10.4f %8s %8d\n",
			t.ID, t.Name, t.RA, t.Dec, t.Duration, t.Priority)
	}
	fmt.Println()
}

func cmdVLBI(sites []*model.TelescopeSite, targets []*model.ObservationTarget, horizonHours float64, faultSite string) {
	startTime := time.Now().UTC().Truncate(time.Hour)

	fmt.Printf("Building VLBI Observation DAG...\n")
	fmt.Printf("  Start time:    %s\n", startTime.Format("2006-01-02 15:04 UTC"))
	fmt.Printf("  Horizon:       %.0f hours\n", horizonHours)
	fmt.Printf("  Telescopes:    %d\n", len(sites))
	fmt.Printf("  Targets:       %d\n", len(targets))
	fmt.Println()

	config := dag.DefaultVLBIConfig()

	fmt.Println("  Building schedule...")
	observationDAG := dag.BuildVLBIDAG(sites, targets, startTime, horizonHours, config)

	fmt.Printf("DAG built: %d nodes, %d edges\n\n", len(observationDAG.Nodes), len(observationDAG.Edges))

	fmt.Print(dag.RenderDAGTree(observationDAG))
	fmt.Print(dag.RenderDAGProgressBar(observationDAG))
	fmt.Print(dag.RenderVLBIGroups(observationDAG))

	var siteIDs []string
	for _, s := range sites {
		siteIDs = append(siteIDs, s.ID)
	}
	fmt.Print(dag.RenderSiteHealthBar(observationDAG, siteIDs))

	fmt.Print(dag.RenderTimeline(observationDAG, startTime, horizonHours))

	if faultSite != "" {
		fmt.Println()
		fmt.Printf("!! Simulating fault at site: %s !!\n\n", faultSite)

		var reason string
		found := false
		for _, s := range sites {
			if s.ID == faultSite {
				reason = fmt.Sprintf("Antenna malfunction at %s", s.Name)
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Unknown site: %s\n", faultSite)
			return
		}

		fmt.Println("  Propagating fault through DAG...")
		result := observationDAG.PropagateFault(faultSite, reason)

		fmt.Print(dag.RenderFaultPropagation(result))
		fmt.Print(dag.RenderDAGTree(observationDAG))
		fmt.Print(dag.RenderDAGProgressBar(observationDAG))
		fmt.Print(dag.RenderVLBIGroups(observationDAG))
		fmt.Print(dag.RenderSiteHealthBar(observationDAG, siteIDs))
		fmt.Print(dag.RenderTimeline(observationDAG, startTime, horizonHours))

		if len(result.ReschedNodes) > 0 {
			fmt.Println()
			fmt.Println("  Rescheduling affected observations...")

			totalSteps := len(result.ReschedNodes)
			for i, nid := range result.ReschedNodes {
				pct := (i + 1) * 100 / totalSteps
				fmt.Printf("  [~~] Rescheduling: %3d%%  %s\n", pct, nid)
			}

			for _, nid := range result.ReschedNodes {
				node, ok := observationDAG.GetNode(nid)
				if ok {
					node.Status = dag.StatusReady
				}
			}

			fmt.Println()
			fmt.Println("  [OK] Rescheduling complete. Updated DAG status:")
			fmt.Print(dag.RenderDAGProgressBar(observationDAG))
			fmt.Print(dag.RenderSiteHealthBar(observationDAG, siteIDs))
		}
	}
}

func cmdBatchParse(dir string, workers int) {
	fmt.Printf("Batch parsing FITS files in: %s\n", dir)
	fmt.Printf("  Worker count:   %d\n\n", workers)

	startTime := time.Now()
	config := fits.BatchConfig{
		Workers:    workers,
		Extensions: []string{".fits", ".FITS", ".fit", ".FIT"},
	}

	fmt.Println("Collecting files...")
	files, err := fits.CollectFITSFiles(dir, config.Extensions)
	if err != nil {
		fmt.Printf("Error collecting files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Found %d FITS files\n\n", len(files))

	fmt.Println("Parsing with Worker Pool...")
	results, stats := fits.ParseBatch(files, config)

	elapsed := time.Since(startTime)

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  Batch Parse Results")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  Total files:     %d\n", stats.Total)
	fmt.Printf("  Succeeded:       %d\n", stats.Succeeded)
	fmt.Printf("  Failed:          %d\n", stats.Failed)
	fmt.Printf("  Max open files:  %d (peak concurrent FD)\n", stats.MaxOpenFD)
	fmt.Printf("  Elapsed time:    %s\n", elapsed.Round(time.Millisecond))
	if len(files) > 0 {
		fmt.Printf("  Avg per file:    %s\n", (elapsed / time.Duration(len(files))).Round(time.Microsecond))
	}
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	if stats.Failed > 0 {
		fmt.Println("Failed files (first 5):")
		count := 0
		for _, r := range results {
			if r.Error != nil && count < 5 {
				fmt.Printf("  %s: %v\n", r.Path, r.Error)
				count++
			}
		}
		fmt.Println()
	}
}

func cmdGenBatch(dir string, count int) {
	fmt.Printf("Generating %d sample FITS files in: %s\n", count, dir)

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	startTime := time.Now()
	generated := 0

	for i := 0; i < count; i++ {
		filename := fmt.Sprintf("calib_%06d.fits", i)
		path := filepath.Join(dir, filename)
		err := fits.GenerateSampleFITSHeader(path)
		if err != nil {
			fmt.Printf("Warning: failed to generate %s: %v\n", path, err)
			continue
		}
		generated++

		if generated%1000 == 0 {
			fmt.Printf("  Generated %d / %d...\n", generated, count)
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\nGenerated %d files in %s\n", generated, elapsed.Round(time.Millisecond))
	fmt.Printf("  Directory: %s\n", dir)
	fmt.Println()
}

func printUsage() {
	fmt.Println("Astro Telescope CLI — Radio Telescope Observation Scheduler")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  astro-cli <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  schedule              Run the observation scheduler (default)")
	fmt.Println("  vlbi                  Build VLBI DAG with synchronized dependencies")
	fmt.Println("  fits <path>           Parse a FITS header file")
	fmt.Println("  batch <dir>           Batch parse all FITS files in a directory (Worker Pool)")
	fmt.Println("  genbatch <dir>        Generate sample FITS files for testing")
	fmt.Println("  visibility <site> <target>  Compute target visibility from a site")
	fmt.Println("  gensample <path>      Generate a sample FITS header file")
	fmt.Println("  listsites             List all telescope sites")
	fmt.Println("  listtargets           List all observation targets")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -targets <file>       Path to targets file (format: name RA Dec DurationMin Priority)")
	fmt.Println("  -horizon <hours>      Scheduling horizon in hours (default: 24)")
	fmt.Println("  -site <id>            Site ID for visibility command")
	fmt.Println("  -target <id>          Target ID for visibility command")
	fmt.Println("  -workers <n>          Number of workers for batch parsing (default: 32)")
	fmt.Println("  -count <n>            Number of files for genbatch (default: 1000)")
	fmt.Println("  -fault <site_id>      Simulate fault at site (for vlbi command)")
	fmt.Println()
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	fs := flag.NewFlagSet("options", flag.ExitOnError)
	targetsFile := fs.String("targets", "", "Targets file path")
	horizonHours := fs.Float64("horizon", 24, "Scheduling horizon in hours")
	siteID := fs.String("site", "", "Site ID")
	targetID := fs.String("target", "", "Target ID")
	workers := fs.Int("workers", 32, "Number of worker goroutines for batch processing")
	batchCount := fs.Int("count", 1000, "Number of sample FITS files to generate")
	faultSite := fs.String("fault", "", "Simulate fault at site ID (for vlbi command)")
	fs.Parse(args)

	sites := defaultSites()
	targets := defaultTargets()

	if *targetsFile != "" {
		t, err := parseTargetsFile(*targetsFile)
		if err != nil {
			fmt.Printf("Error reading targets file: %v\n", err)
			os.Exit(1)
		}
		if len(t) > 0 {
			targets = t
		}
	}

	switch cmd {
	case "help", "-h", "--help":
		printUsage()

	case "schedule":
		cmdSchedule(sites, targets, *horizonHours)

	case "fits":
		path := ""
		if fs.NArg() > 0 {
			path = fs.Arg(0)
		} else if len(args) > 0 {
			path = args[0]
		}
		if path == "" {
			fmt.Println("Error: FITS file path required")
			os.Exit(1)
		}
		cmdParseFITS(path)

	case "visibility":
		sid := *siteID
		tid := *targetID
		if sid == "" && fs.NArg() > 0 {
			sid = fs.Arg(0)
		}
		if tid == "" && fs.NArg() > 1 {
			tid = fs.Arg(1)
		}
		if sid == "" || tid == "" {
			fmt.Println("Error: site and target required (use -site and -target flags or positional args)")
			os.Exit(1)
		}
		cmdVisibility(sid, tid, sites, targets)

	case "gensample":
		path := "sample.fits"
		if fs.NArg() > 0 {
			path = fs.Arg(0)
		} else if len(args) > 0 {
			path = args[0]
		}
		absPath, _ := filepath.Abs(path)
		cmdGenSample(absPath)

	case "listsites":
		cmdListSites(sites)

	case "listtargets":
		cmdListTargets(targets)

	case "vlbi":
		cmdVLBI(sites, targets, *horizonHours, *faultSite)

	case "batch":
		dir := ""
		if fs.NArg() > 0 {
			dir = fs.Arg(0)
		} else if len(args) > 0 {
			dir = args[0]
		}
		if dir == "" {
			fmt.Println("Error: directory path required for batch command")
			os.Exit(1)
		}
		cmdBatchParse(dir, *workers)

	case "genbatch":
		dir := "fits_samples"
		if fs.NArg() > 0 {
			dir = fs.Arg(0)
		} else if len(args) > 0 {
			dir = args[0]
		}
		cmdGenBatch(dir, *batchCount)

	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}
