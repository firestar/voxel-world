package environment

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

type WeatherKind string

const (
	WeatherClear WeatherKind = "clear"
	WeatherRain  WeatherKind = "rain"
	WeatherStorm WeatherKind = "storm"
)

type Phase string

const (
	PhaseDawn  Phase = "dawn"
	PhaseDay   Phase = "day"
	PhaseDusk  Phase = "dusk"
	PhaseNight Phase = "night"
)

type Config struct {
	DayLength          time.Duration `json:"dayLength"`
	WeatherMinDuration time.Duration `json:"weatherMinDuration"`
	WeatherMaxDuration time.Duration `json:"weatherMaxDuration"`
	StormChance        float64       `json:"stormChance"`
	RainChance         float64       `json:"rainChance"`
	WindBase           float64       `json:"windBase"`
	WindVariance       float64       `json:"windVariance"`
	Seed               int64         `json:"seed"`
}

type State struct {
	TimeOfDay float64
	Phase     Phase
	Lighting  LightingState
	Weather   WeatherState
	Physics   PhysicsModifiers
	Behavior  BehaviorModifiers
}

type LightingState struct {
	Ambient     float64
	SunAngle    float64
	FogDensity  float64
	WeatherTint float64
}

type WeatherState struct {
	Kind          WeatherKind
	Intensity     float64
	WindSpeed     float64
	WindDirection float64
	Precipitation float64
}

type PhysicsModifiers struct {
	GravityScale        float64
	DragScale           float64
	GroundFrictionScale float64
}

type BehaviorModifiers struct {
	MobilityScale   float64
	VisibilityScale float64
	MoraleShift     float64
}

type Environment struct {
	mu           sync.Mutex
	cfg          Config
	rng          *rand.Rand
	state        State
	dayProgress  float64
	weatherTimer time.Duration
}

func New(cfg Config) *Environment {
	cfg = applyDefaults(cfg)
	rng := rand.New(rand.NewSource(cfg.Seed))
	env := &Environment{
		cfg: cfg,
		rng: rng,
	}
	env.state.TimeOfDay = 12.0
	env.dayProgress = 0.5
	env.state.Phase = PhaseDay
	env.state.Weather = WeatherState{Kind: WeatherClear, Intensity: 0, WindSpeed: cfg.WindBase, WindDirection: 0, Precipitation: 0}
	env.state.Lighting = computeLighting(env.dayProgress, env.state.Weather, env.state.Phase)
	env.state.Physics = computePhysics(env.state.Weather)
	env.state.Behavior = computeBehavior(env.dayProgress, env.state.Weather, env.state.Phase)
	env.weatherTimer = env.randomWeatherDuration()
	return env
}

func applyDefaults(cfg Config) Config {
	if cfg.DayLength <= 0 {
		cfg.DayLength = 20 * time.Minute
	}
	if cfg.WeatherMinDuration <= 0 {
		cfg.WeatherMinDuration = 90 * time.Second
	}
	if cfg.WeatherMaxDuration < cfg.WeatherMinDuration {
		cfg.WeatherMaxDuration = cfg.WeatherMinDuration + 2*time.Minute
	}
	if cfg.StormChance < 0 {
		cfg.StormChance = 0
	}
	if cfg.RainChance < 0 {
		cfg.RainChance = 0
	}
	if cfg.StormChance+cfg.RainChance > 1 {
		total := cfg.StormChance + cfg.RainChance
		cfg.StormChance /= total
		cfg.RainChance /= total
	}
	if cfg.WindBase < 0 {
		cfg.WindBase = 0
	}
	if cfg.WindVariance < 0 {
		cfg.WindVariance = 0
	}
	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UnixNano()
	}
	return cfg
}

func (e *Environment) Step(delta time.Duration) State {
	if delta <= 0 {
		delta = 16 * time.Millisecond
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	fraction := float64(delta) / float64(e.cfg.DayLength)
	e.dayProgress += fraction
	for e.dayProgress >= 1 {
		e.dayProgress -= 1
	}
	hours := e.dayProgress * 24
	phase := determinePhase(hours)

	e.weatherTimer -= delta
	if e.weatherTimer <= 0 {
		e.state.Weather = e.rollWeather()
		e.weatherTimer = e.randomWeatherDuration()
	}

	e.state.TimeOfDay = hours
	e.state.Phase = phase
	e.state.Lighting = computeLighting(e.dayProgress, e.state.Weather, phase)
	e.state.Physics = computePhysics(e.state.Weather)
	e.state.Behavior = computeBehavior(e.dayProgress, e.state.Weather, phase)
	return e.state
}

func (e *Environment) CurrentState() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

func (e *Environment) rollWeather() WeatherState {
	roll := e.rng.Float64()
	var kind WeatherKind
	switch {
	case roll < e.cfg.StormChance:
		kind = WeatherStorm
	case roll < e.cfg.StormChance+e.cfg.RainChance:
		kind = WeatherRain
	default:
		kind = WeatherClear
	}

	intensity := 0.0
	precipitation := 0.0
	wind := e.cfg.WindBase + e.rng.Float64()*e.cfg.WindVariance
	if kind == WeatherRain {
		intensity = 0.35 + e.rng.Float64()*0.4
		precipitation = intensity
	} else if kind == WeatherStorm {
		intensity = 0.65 + e.rng.Float64()*0.35
		precipitation = intensity
		wind += e.cfg.WindVariance * 0.7
	}
	direction := e.rng.Float64() * 2 * math.Pi
	return WeatherState{
		Kind:          kind,
		Intensity:     clamp01(intensity),
		WindSpeed:     wind,
		WindDirection: direction,
		Precipitation: clamp01(precipitation),
	}
}

func (e *Environment) randomWeatherDuration() time.Duration {
	if e.cfg.WeatherMaxDuration <= e.cfg.WeatherMinDuration {
		return e.cfg.WeatherMinDuration
	}
	span := e.cfg.WeatherMaxDuration - e.cfg.WeatherMinDuration
	return e.cfg.WeatherMinDuration + time.Duration(e.rng.Float64()*float64(span))
}

func determinePhase(hour float64) Phase {
	switch {
	case hour >= 5 && hour < 7:
		return PhaseDawn
	case hour >= 7 && hour < 18:
		return PhaseDay
	case hour >= 18 && hour < 21:
		return PhaseDusk
	default:
		return PhaseNight
	}
}

func computeLighting(progress float64, weather WeatherState, phase Phase) LightingState {
	sunAngle := progress * 2 * math.Pi
	sunHeight := math.Cos((progress - 0.5) * 2 * math.Pi)
	if sunHeight < 0 {
		sunHeight = 0
	}
	ambient := 0.12 + 0.88*sunHeight
	if phase == PhaseNight {
		ambient = 0.08 + 0.12*sunHeight
	}
	ambient *= 1 - 0.35*weather.Intensity
	fog := 0.02 + 0.25*weather.Intensity
	tint := 0.0
	switch weather.Kind {
	case WeatherRain:
		tint = 0.15 * weather.Intensity
	case WeatherStorm:
		tint = 0.35 * weather.Intensity
	}
	return LightingState{
		Ambient:     clamp01(ambient),
		SunAngle:    sunAngle,
		FogDensity:  clamp01(fog),
		WeatherTint: clamp01(tint),
	}
}

func computePhysics(weather WeatherState) PhysicsModifiers {
	gravity := 1.0 + 0.06*weather.Intensity
	drag := 1.0 + 0.5*weather.Intensity
	friction := 1.0 - 0.3*weather.Intensity
	if friction < 0.3 {
		friction = 0.3
	}
	return PhysicsModifiers{
		GravityScale:        gravity,
		DragScale:           drag,
		GroundFrictionScale: friction,
	}
}

func computeBehavior(progress float64, weather WeatherState, phase Phase) BehaviorModifiers {
	ambient := 0.12 + 0.88*math.Cos((progress-0.5)*2*math.Pi)
	if ambient < 0 {
		ambient = 0
	}
	visibility := 0.4 + 0.6*ambient
	mobility := 1.0 - 0.25*weather.Intensity
	morale := 0.1
	switch phase {
	case PhaseNight:
		visibility *= 0.7
		morale -= 0.05
	case PhaseDawn:
		morale += 0.05
	case PhaseDay:
		morale += 0.1
	case PhaseDusk:
		visibility *= 0.85
	}
	morale -= 0.2 * weather.Intensity
	if mobility < 0.4 {
		mobility = 0.4
	}
	return BehaviorModifiers{
		MobilityScale:   mobility,
		VisibilityScale: clamp01(visibility),
		MoraleShift:     morale,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
