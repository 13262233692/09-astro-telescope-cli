package model

import "time"

type TelescopeSite struct {
	ID         string
	Name       string
	Latitude   float64
	Longitude  float64
	Elevation  float64
	Health     float64
}

type ObservationTarget struct {
	ID         string
	Name       string
	RA         float64
	Dec        float64
	Duration   time.Duration
	Priority   int
}

type CalibrationParams struct {
	ObsDate    time.Time
	Telescope  string
	Instrument string
	Exposure   float64
	Gain       float64
	Temp       float64
	Custom     map[string]string
}

type AltAz struct {
	Altitude  float64
	Azimuth   float64
}

type TimeWindow struct {
	Start time.Time
	End   time.Time
}

type VisibilityResult struct {
	Target    *ObservationTarget
	Site      *TelescopeSite
	Windows   []TimeWindow
	MaxAlt    float64
	BestTime  time.Time
}

type ScheduledObservation struct {
	Target    *ObservationTarget
	Site      *TelescopeSite
	Start     time.Time
	End       time.Time
	Score     float64
	AltAtStart float64
}
