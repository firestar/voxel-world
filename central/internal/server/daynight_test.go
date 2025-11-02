package server

import (
	"testing"
	"time"
)

func TestDayNightCycleProgress(t *testing.T) {
	cycle := newDayNightCycle(24*time.Minute, 6)
	start := cycle.start
	state := cycle.State(start)
	if state.TimeOfDay < 5.9 || state.TimeOfDay > 6.1 {
		t.Fatalf("initial timeOfDay = %.2f, want around 6", state.TimeOfDay)
	}
	later := start.Add(6 * time.Minute)
	midday := cycle.State(later)
	if midday.TimeOfDay < 11.9 || midday.TimeOfDay > 12.1 {
		t.Fatalf("midday timeOfDay = %.2f, want around 12", midday.TimeOfDay)
	}
	if midday.SunPosition.Y <= 0 {
		t.Fatalf("sun should be above horizon at midday: %+v", midday.SunPosition)
	}
	night := cycle.State(start.Add(23 * time.Minute))
	if night.SunPosition.Y >= 0 {
		t.Fatalf("sun should be below horizon at night: %+v", night.SunPosition)
	}
	if night.AmbientIntensity >= midday.AmbientIntensity {
		t.Fatalf("ambient should be lower at night: night %.2f midday %.2f", night.AmbientIntensity, midday.AmbientIntensity)
	}
}
