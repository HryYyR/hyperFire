package gameplay

import (
	"math"
	"time"
)

const defaultTickHz = 60

// Config stores the human-facing gameplay knobs. Anything tied to frame count
// is derived from this struct so changing tick rate does not silently change game feel.
type Config struct {
	TickHz uint32

	EnemyRadius      float64
	PlayerRadius     float64
	BulletRadius     float64
	EnemyAggroRadius float64

	PlayerHealth    int
	EnemyLowHealth  int
	EnemyHighHealth int
	BulletDamage    int

	EnemyMoveUnitsPerSecond         float64
	PlayerMoveUnitsPerSecond        float64
	PlayerBulletUnitsPerSecond      float64
	EnemyBulletUnitsPerSecond       float64
	PlayerBulletCollisionSampleStep float64

	InitialBulletLife  time.Duration
	PlayerFireCooldown time.Duration
	EnemyFireCooldown  time.Duration
	SpawnInterval      time.Duration

	SpawnBaseX      float64
	SpawnBaseY      float64
	SpawnSpacingX   float64
	SpawnAreaWidth  float64
	SpawnAreaHeight float64
}

// runtimeParams contains the tick-based values actually consumed by systems.
type runtimeParams struct {
	InitialBulletLifeFrames  int
	EnemyMoveSpeed           float64
	PlayerMoveSpeed          float64
	PlayerBulletSpeed        float64
	EnemyBulletSpeed         float64
	PlayerFireCooldownFrames int
	EnemyFireCooldownFrames  int
	SpawnTimerFrames         int
}

func DefaultConfig(tickHz uint32) Config {
	if tickHz == 0 {
		tickHz = defaultTickHz
	}

	return Config{
		TickHz: tickHz,

		EnemyRadius:      1,
		PlayerRadius:     1,
		BulletRadius:     0.5,
		EnemyAggroRadius: 24.0,

		PlayerHealth:    100,
		EnemyLowHealth:  10,
		EnemyHighHealth: 20,
		BulletDamage:    10,

		EnemyMoveUnitsPerSecond:         28.0,
		PlayerMoveUnitsPerSecond:        52.0,
		PlayerBulletUnitsPerSecond:      256.0,
		EnemyBulletUnitsPerSecond:       196.0,
		PlayerBulletCollisionSampleStep: 0.5,

		InitialBulletLife:  10 * time.Second,
		PlayerFireCooldown: 180 * time.Millisecond,
		EnemyFireCooldown:  1800 * time.Millisecond,
		SpawnInterval:      4 * time.Second,

		SpawnBaseX:      20.0,
		SpawnBaseY:      20.0,
		SpawnSpacingX:   8.0,
		SpawnAreaWidth:  100.0,
		SpawnAreaHeight: 100.0,
	}
}

func (c Config) normalized() Config {
	if c.TickHz == 0 {
		c.TickHz = defaultTickHz
	}
	if c.PlayerBulletCollisionSampleStep <= 0 {
		c.PlayerBulletCollisionSampleStep = c.BulletRadius
	}
	return c
}

func (c Config) runtimeParams() runtimeParams {
	c = c.normalized()
	return runtimeParams{
		InitialBulletLifeFrames:  framesFromDuration(c.InitialBulletLife, c.TickHz),
		EnemyMoveSpeed:           unitsPerTick(c.EnemyMoveUnitsPerSecond, c.TickHz),
		PlayerMoveSpeed:          unitsPerTick(c.PlayerMoveUnitsPerSecond, c.TickHz),
		PlayerBulletSpeed:        unitsPerTick(c.PlayerBulletUnitsPerSecond, c.TickHz),
		EnemyBulletSpeed:         unitsPerTick(c.EnemyBulletUnitsPerSecond, c.TickHz),
		PlayerFireCooldownFrames: framesFromDuration(c.PlayerFireCooldown, c.TickHz),
		EnemyFireCooldownFrames:  framesFromDuration(c.EnemyFireCooldown, c.TickHz),
		SpawnTimerFrames:         framesFromDuration(c.SpawnInterval, c.TickHz),
	}
}

func framesFromDuration(duration time.Duration, tickHz uint32) int {
	frames := int(math.Round(duration.Seconds() * float64(tickHz)))
	if frames < 1 {
		return 1
	}
	return frames
}

func unitsPerTick(unitsPerSecond float64, tickHz uint32) float64 {
	return unitsPerSecond / float64(tickHz)
}
