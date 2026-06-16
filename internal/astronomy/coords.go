package astronomy

import (
	"math"
	"time"

	"astro-telescope-cli/pkg/model"
)

const (
	degToRad = math.Pi / 180.0
	radToDeg = 180.0 / math.Pi
	j2000    = 2451545.0
	julianCentury = 36525.0
	minAltitude  = 10.0
)

func JulianDate(t time.Time) float64 {
	tUTC := t.UTC()
	year := tUTC.Year()
	month := int(tUTC.Month())
	day := tUTC.Day()
	hour := float64(tUTC.Hour())
	minute := float64(tUTC.Minute())
	second := float64(tUTC.Second()) + float64(tUTC.Nanosecond())/1e9

	if month <= 2 {
		year--
		month += 12
	}

	A := int(float64(year) / 100)
	B := 2 - A + int(float64(A)/4)

	jd := float64(int(365.25*float64(year+4716))) +
		float64(int(30.6001*float64(month+1))) +
		float64(day) + (hour+minute/60.0+second/3600.0)/24.0 +
		float64(B) - 1524.5

	return jd
}

func GreenwichSiderealTime(t time.Time) float64 {
	jd := JulianDate(t)
	T := (jd - j2000) / julianCentury

	gmst := 280.46061837 +
		360.98564736629*(jd-j2000) +
		0.000387933*T*T -
		T*T*T/38710000.0

	gmst = math.Mod(gmst, 360.0)
	if gmst < 0 {
		gmst += 360.0
	}

	return gmst
}

func LocalSiderealTime(t time.Time, longitude float64) float64 {
	gmst := GreenwichSiderealTime(t)
	lst := gmst + longitude
	lst = math.Mod(lst, 360.0)
	if lst < 0 {
		lst += 360.0
	}
	return lst
}

func HourAngle(t time.Time, raDegrees, longitude float64) float64 {
	lst := LocalSiderealTime(t, longitude)
	ha := lst - raDegrees
	ha = math.Mod(ha, 360.0)
	if ha > 180 {
		ha -= 360
	}
	if ha < -180 {
		ha += 360
	}
	return ha
}

func EquatorialToHorizontal(t time.Time, ra, dec, lat, lon float64) model.AltAz {
	ha := HourAngle(t, ra, lon) * degToRad
	decRad := dec * degToRad
	latRad := lat * degToRad

	sinAlt := math.Sin(decRad)*math.Sin(latRad) +
		math.Cos(decRad)*math.Cos(latRad)*math.Cos(ha)
	alt := math.Asin(sinAlt)

	cosAlt := math.Cos(alt)
	if cosAlt < 1e-10 {
		cosAlt = 1e-10
	}

	sinAz := -math.Cos(decRad) * math.Sin(ha) / cosAlt
	cosAz := (math.Sin(decRad) - math.Sin(alt)*math.Sin(latRad)) / (cosAlt * math.Cos(latRad))

	az := math.Atan2(sinAz, cosAz)
	if az < 0 {
		az += 2 * math.Pi
	}

	return model.AltAz{
		Altitude:  alt * radToDeg,
		Azimuth:   az * radToDeg,
	}
}

func IsVisible(t time.Time, target *model.ObservationTarget, site *model.TelescopeSite) bool {
	altAz := EquatorialToHorizontal(t, target.RA, target.Dec, site.Latitude, site.Longitude)
	return altAz.Altitude >= minAltitude
}

func ComputeVisibility(target *model.ObservationTarget, site *model.TelescopeSite, startTime time.Time, duration time.Duration) *model.VisibilityResult {
	step := 5 * time.Minute
	result := &model.VisibilityResult{
		Target: target,
		Site:   site,
	}

	var (
		currentWindow *model.TimeWindow
		windows       []model.TimeWindow
		maxAlt        float64 = -90
		bestTime      time.Time
	)

	for t := startTime; t.Before(startTime.Add(duration)); t = t.Add(step) {
		altAz := EquatorialToHorizontal(t, target.RA, target.Dec, site.Latitude, site.Longitude)

		if altAz.Altitude > maxAlt {
			maxAlt = altAz.Altitude
			bestTime = t
		}

		if altAz.Altitude >= minAltitude {
			if currentWindow == nil {
				currentWindow = &model.TimeWindow{Start: t}
			}
			currentWindow.End = t.Add(step)
		} else {
			if currentWindow != nil {
				if currentWindow.End.Sub(currentWindow.Start) >= 10*time.Minute {
					windows = append(windows, *currentWindow)
				}
				currentWindow = nil
			}
		}
	}

	if currentWindow != nil {
		if currentWindow.End.Sub(currentWindow.Start) >= 10*time.Minute {
			windows = append(windows, *currentWindow)
		}
	}

	result.Windows = windows
	result.MaxAlt = maxAlt
	result.BestTime = bestTime

	return result
}

func FindBestObservationSlot(target *model.ObservationTarget, site *model.TelescopeSite, startTime time.Time, horizonHours float64) (time.Time, float64, bool) {
	horizon := startTime.Add(time.Duration(horizonHours * float64(time.Hour)))
	step := 5 * time.Minute

	var (
		bestTime  time.Time
		bestAlt   float64 = -90
		found     bool
	)

	for t := startTime; t.Add(target.Duration).Before(horizon); t = t.Add(step) {
		avgAlt := 0.0
		samples := 0
		valid := true

		for ct := t; ct.Before(t.Add(target.Duration)); ct = ct.Add(step) {
			altAz := EquatorialToHorizontal(ct, target.RA, target.Dec, site.Latitude, site.Longitude)
			if altAz.Altitude < minAltitude {
				valid = false
				break
			}
			avgAlt += altAz.Altitude
			samples++
		}

		if valid && samples > 0 {
			avgAlt /= float64(samples)
			if avgAlt > bestAlt {
				bestAlt = avgAlt
				bestTime = t
				found = true
			}
		}
	}

	return bestTime, bestAlt, found
}
