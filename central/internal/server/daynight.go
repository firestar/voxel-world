package server

import (
	"math"
	"time"
)

type dayNightCycle struct {
	start           time.Time
	dayLength       time.Duration
	initialFraction float64
	orbitRadius     float64
}

type DayNightState struct {
	TimeOfDay         float64 `json:"timeOfDay"`
	Progress          float64 `json:"progress"`
	Phase             string  `json:"phase"`
	SunAngle          float64 `json:"sunAngle"`
	SunPosition       Vector3 `json:"sunPosition"`
	SunLightIntensity float64 `json:"sunLightIntensity"`
	AmbientIntensity  float64 `json:"ambientIntensity"`
}

type Vector3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func newDayNightCycle(dayLength time.Duration, initialHour float64) *dayNightCycle {
	if dayLength <= 0 {
		dayLength = 20 * time.Minute
	}
	initialFraction := initialHour / 24
	if initialFraction < 0 {
		initialFraction = 0
	}
	if initialFraction >= 1 {
		initialFraction = math.Mod(initialFraction, 1)
	}
	return &dayNightCycle{
		start:           time.Now(),
		dayLength:       dayLength,
		initialFraction: initialFraction,
		orbitRadius:     2000,
	}
}

func (c *dayNightCycle) State(now time.Time) DayNightState {
	if c == nil {
		return DayNightState{}
	}
	elapsed := now.Sub(c.start)
	if elapsed < 0 {
		elapsed = 0
	}
	cycleFraction := float64(elapsed) / float64(c.dayLength)
	progress := math.Mod(c.initialFraction+cycleFraction, 1)
	timeOfDay := progress * 24
	sunAngle := progress * 2 * math.Pi
	sunCycle := math.Mod(progress+0.75, 1)
	orbital := sunCycle * 2 * math.Pi
	x := math.Cos(orbital) * c.orbitRadius
	y := math.Sin(orbital) * c.orbitRadius
	z := 0.0
	elevationFactor := math.Max(0, math.Sin(orbital))
	ambient := 0.2 + 0.6*elevationFactor
	sunlight := 0.15 + 0.85*elevationFactor
	return DayNightState{
		TimeOfDay:         timeOfDay,
		Progress:          progress,
		Phase:             phaseForHour(timeOfDay),
		SunAngle:          sunAngle,
		SunPosition:       Vector3{X: x, Y: y, Z: z},
		SunLightIntensity: sunlight,
		AmbientIntensity:  ambient,
	}
}

func phaseForHour(hour float64) string {
	switch {
	case hour >= 5 && hour < 7:
		return "dawn"
	case hour >= 7 && hour < 18:
		return "day"
	case hour >= 18 && hour < 21:
		return "dusk"
	default:
		return "night"
	}
}
