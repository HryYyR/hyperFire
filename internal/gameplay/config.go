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

	EnemyRadius                float64
	PlayerRadius               float64
	BulletRadius               float64
	ExpOrbRadius               float64
	SkillDropRadius            float64
	EnemyGunnerAggroRadius     float64
	EnemyBladeAggroRadius      float64
	EnemyGunnerAlertRadius     float64
	EnemyBladeAlertRadius      float64
	EnemyBladeAttackRadius     float64
	EnemyCleanupDistance       float64
	EnemySpawnMinDistance      float64
	EnemySkillPickupSeekRadius float64

	PlayerHealth                   int
	EnemyLowHealth                 int
	EnemyHighHealth                int
	BulletDamage                   int
	EnemyBladeDamage               int
	EnemyEliteBaseHealthScale      float64
	EnemyBossBaseHealthScale       float64
	EnemyHealthGrowthPerLevel      float64
	EnemyDamageGrowthPerLevel      float64
	EnemyAttackSpeedGrowthPerLevel float64
	EnemyEliteHealthGrowthScale    float64
	EnemyBossHealthGrowthScale     float64
	EnemyEliteDamageGrowthScale    float64
	EnemyBossDamageGrowthScale     float64
	EnemyEliteAttackSpeedScale     float64
	EnemyBossAttackSpeedScale      float64
	EnemyExpDropLow                int32
	EnemyExpDropHigh               int32

	PlayerKnockbackResistance      float64
	EnemyGunnerKnockbackResistance float64
	EnemyBladeKnockbackResistance  float64
	EnemyBladeArcStrength          float64
	EnemyPatrolSpeedScale          float64
	EnemyAggroLeashScale           float64
	EnemyGunnerPreferredRangeScale float64
	PlayerRollDistance             float64
	EnemyGunnerRollDistance        float64
	EnemyBladeRollDistance         float64

	EnemyGunnerMoveUnitsPerSecond     float64
	EnemyBladeMoveUnitsPerSecond      float64
	PlayerMoveUnitsPerSecond          float64
	PlayerBulletUnitsPerSecond        float64
	EnemyBulletUnitsPerSecond         float64
	BulletKnockbackUnitsPerSecond     float64
	EnemyBladeKnockbackUnitsPerSecond float64
	PlayerBulletCollisionSampleStep   float64
	ExpOrbMoveUnitsPerSecond          float64
	ExpOrbMagnetRadius                float64
	ExpOrbMergeRadius                 float64

	InitialBulletLife        time.Duration
	KnockbackDuration        time.Duration
	RollDuration             time.Duration
	RollLockOnHit            time.Duration
	PlayerFireCooldown       time.Duration
	PlayerRollCooldown       time.Duration
	EnemyGunnerFireCooldown  time.Duration
	EnemyGunnerRollCooldown  time.Duration
	EnemyBladeAttackCooldown time.Duration
	EnemyBladeRollCooldown   time.Duration
	EnemyAlertLockDelay      time.Duration
	EnemyPatrolWait          time.Duration
	EnemyPatrolMove          time.Duration
	SpawnInterval            time.Duration
	HordeSpawnInterval       time.Duration
	HordePeakSpawnInterval   time.Duration
	HordeDuration            time.Duration
	ExpOrbLifetime           time.Duration
	SkillDropLifetime        time.Duration
	EnemyCleanupLifetime     time.Duration

	PlayerRollMaxCharges      int
	EnemyGunnerRollMaxCharges int
	EnemyBladeRollMaxCharges  int
	EnemyMinionSpawnWeight    int
	EnemyEliteSpawnWeight     int
	EnemyBossSpawnWeight      int
	MaxEnemies                int
	EnemySpawnAttempts        int
	EnemyBaseLevel            uint32
	EnemyMinionMinLevel       uint32
	EnemyEliteMinLevel        uint32
	EnemyBossMinLevel         uint32
	EnemyMaxLevel             uint32
	EnemyBonusLevelChance     float64
	EnemyMinionSpawnDelay     time.Duration
	EnemyEliteSpawnDelay      time.Duration
	EnemyBossSpawnDelay       time.Duration
	HordeThreshold            int32
	HordeValuePerMinionKill   int32
	HordeValuePerEliteKill    int32
	HordeValuePerBossKill     int32
	HordeAggroRadiusScale     float64
	HordeAlertRadiusScale     float64
	HordeBossSpawnWeight      int
	EnemySkillDropChance      float64

	SpawnBaseX      float64
	SpawnBaseY      float64
	SpawnSpacingX   float64
	SpawnAreaWidth  float64
	SpawnAreaHeight float64
}

// runtimeParams contains the tick-based values actually consumed by systems.
type runtimeParams struct {
	InitialBulletLifeFrames        int
	ExpOrbLifetimeFrames           int
	SkillDropLifetimeFrames        int
	EnemyGunnerMoveSpeed           float64
	EnemyBladeMoveSpeed            float64
	PlayerMoveSpeed                float64
	PlayerBulletSpeed              float64
	EnemyBulletSpeed               float64
	BulletKnockbackPerTick         float64
	EnemyBladeKnockbackPerTick     float64
	ExpOrbMoveSpeed                float64
	KnockbackDurationFrames        int
	RollDurationFrames             int
	RollLockOnHitFrames            int
	PlayerFireCooldownFrames       int
	PlayerRollCooldownFrames       int
	EnemyGunnerFireCooldownFrames  int
	EnemyGunnerRollCooldownFrames  int
	EnemyBladeAttackCooldownFrames int
	EnemyBladeRollCooldownFrames   int
	EnemyAlertLockDelayFrames      int
	EnemyPatrolWaitFrames          int
	EnemyPatrolMoveFrames          int
	SpawnTimerFrames               int
	HordeSpawnTimerFrames          int
	HordePeakSpawnTimerFrames      int
	HordeDurationFrames            int
	EnemyCleanupLifetimeFrames     int
	EnemyMinionSpawnDelayFrames    int
	EnemyEliteSpawnDelayFrames     int
	EnemyBossSpawnDelayFrames      int
}

func DefaultConfig(tickHz uint32) Config {
	if tickHz == 0 {
		tickHz = defaultTickHz
	}

	return Config{
		TickHz: tickHz,

		EnemyRadius:                1,
		PlayerRadius:               1,
		BulletRadius:               0.5,
		ExpOrbRadius:               0.45,
		SkillDropRadius:            0.55,
		EnemyGunnerAggroRadius:     30.0,
		EnemyBladeAggroRadius:      20.0,
		EnemyGunnerAlertRadius:     42.0,
		EnemyBladeAlertRadius:      30.0,
		EnemyBladeAttackRadius:     2.2,
		EnemyCleanupDistance:       60.0,
		EnemySpawnMinDistance:      18.0,
		EnemySkillPickupSeekRadius: 14.0,

		PlayerHealth:                   100,
		EnemyLowHealth:                 30,
		EnemyHighHealth:                50,
		BulletDamage:                   10,
		EnemyBladeDamage:               16,
		EnemyEliteBaseHealthScale:      1.2,
		EnemyBossBaseHealthScale:       2.8,
		EnemyHealthGrowthPerLevel:      0.25,
		EnemyDamageGrowthPerLevel:      0.15,
		EnemyAttackSpeedGrowthPerLevel: 0.10,
		EnemyEliteHealthGrowthScale:    1.2,
		EnemyBossHealthGrowthScale:     2.2,
		EnemyEliteDamageGrowthScale:    1.15,
		EnemyBossDamageGrowthScale:     1.35,
		EnemyEliteAttackSpeedScale:     1.1,
		EnemyBossAttackSpeedScale:      1.25,
		EnemyExpDropLow:                1,
		EnemyExpDropHigh:               2,

		PlayerKnockbackResistance:      0,
		EnemyGunnerKnockbackResistance: 0,
		EnemyBladeKnockbackResistance:  0.6,
		EnemyBladeArcStrength:          0.45,
		EnemyPatrolSpeedScale:          0.45,
		EnemyAggroLeashScale:           2.0,
		EnemyGunnerPreferredRangeScale: 0.65,
		PlayerRollDistance:             16.0,
		EnemyGunnerRollDistance:        10.0,
		EnemyBladeRollDistance:         12.0,

		EnemyGunnerMoveUnitsPerSecond:     20.0,
		EnemyBladeMoveUnitsPerSecond:      30.0,
		PlayerMoveUnitsPerSecond:          24.0,
		PlayerBulletUnitsPerSecond:        256.0,
		EnemyBulletUnitsPerSecond:         196.0,
		BulletKnockbackUnitsPerSecond:     48.0,
		EnemyBladeKnockbackUnitsPerSecond: 72.0,
		PlayerBulletCollisionSampleStep:   0.5,
		ExpOrbMoveUnitsPerSecond:          28.0,
		ExpOrbMagnetRadius:                10.0,
		ExpOrbMergeRadius:                 2.0,

		InitialBulletLife:        10 * time.Second,
		KnockbackDuration:        120 * time.Millisecond,
		RollDuration:             180 * time.Millisecond,
		RollLockOnHit:            240 * time.Millisecond,
		PlayerFireCooldown:       180 * time.Millisecond,
		PlayerRollCooldown:       1200 * time.Millisecond,
		EnemyGunnerFireCooldown:  1800 * time.Millisecond,
		EnemyGunnerRollCooldown:  1800 * time.Millisecond,
		EnemyBladeAttackCooldown: 900 * time.Millisecond,
		EnemyBladeRollCooldown:   900 * time.Millisecond,
		EnemyAlertLockDelay:      600 * time.Millisecond,
		EnemyPatrolWait:          1200 * time.Millisecond,
		EnemyPatrolMove:          900 * time.Millisecond,
		SpawnInterval:            3500 * time.Millisecond,
		HordeSpawnInterval:       1200 * time.Millisecond,
		HordePeakSpawnInterval:   500 * time.Millisecond,
		HordeDuration:            20 * time.Second,
		ExpOrbLifetime:           20 * time.Second,
		SkillDropLifetime:        25 * time.Second,
		EnemyCleanupLifetime:     15 * time.Second,

		PlayerRollMaxCharges:      1,
		EnemyGunnerRollMaxCharges: 1,
		EnemyBladeRollMaxCharges:  1,
		EnemyMinionSpawnWeight:    80,
		EnemyEliteSpawnWeight:     17,
		EnemyBossSpawnWeight:      3,
		MaxEnemies:                48,
		EnemySpawnAttempts:        16,
		EnemyBaseLevel:            1,
		EnemyMinionMinLevel:       1,
		EnemyEliteMinLevel:        2,
		EnemyBossMinLevel:         4,
		EnemyMaxLevel:             4,
		EnemyBonusLevelChance:     0.35,
		EnemyMinionSpawnDelay:     500 * time.Millisecond,
		EnemyEliteSpawnDelay:      time.Second,
		EnemyBossSpawnDelay:       2 * time.Second,
		HordeThreshold:            20,
		HordeValuePerMinionKill:   1,
		HordeValuePerEliteKill:    3,
		HordeValuePerBossKill:     8,
		HordeAggroRadiusScale:     1.8,
		HordeAlertRadiusScale:     1.8,
		HordeBossSpawnWeight:      18,
		EnemySkillDropChance:      0.4,

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
	if c.BulletKnockbackUnitsPerSecond < 0 {
		c.BulletKnockbackUnitsPerSecond = 0
	}
	if c.EnemyBladeKnockbackUnitsPerSecond < 0 {
		c.EnemyBladeKnockbackUnitsPerSecond = 0
	}
	if c.PlayerBulletCollisionSampleStep <= 0 {
		c.PlayerBulletCollisionSampleStep = c.BulletRadius
	}
	if c.EnemyBladeAttackRadius <= 0 {
		c.EnemyBladeAttackRadius = c.EnemyRadius * 2
	}
	if c.EnemyGunnerAlertRadius < c.EnemyGunnerAggroRadius {
		c.EnemyGunnerAlertRadius = c.EnemyGunnerAggroRadius
	}
	if c.EnemyBladeAlertRadius < c.EnemyBladeAggroRadius {
		c.EnemyBladeAlertRadius = c.EnemyBladeAggroRadius
	}
	if c.EnemyCleanupDistance <= 0 {
		c.EnemyCleanupDistance = math.Max(c.SpawnAreaWidth, c.SpawnAreaHeight) * 0.6
	}
	if c.EnemySpawnMinDistance < 0 {
		c.EnemySpawnMinDistance = 0
	}
	if c.EnemySkillPickupSeekRadius < 0 {
		c.EnemySkillPickupSeekRadius = 0
	}
	if c.HordeAggroRadiusScale < 1 {
		c.HordeAggroRadiusScale = 1
	}
	if c.HordeAlertRadiusScale < 1 {
		c.HordeAlertRadiusScale = 1
	}
	if c.ExpOrbMergeRadius <= 0 {
		c.ExpOrbMergeRadius = c.ExpOrbRadius * 4
	}
	if c.SkillDropRadius <= 0 {
		c.SkillDropRadius = c.ExpOrbRadius
	}
	if c.EnemyBladeArcStrength < 0 {
		c.EnemyBladeArcStrength = 0
	}
	if c.EnemyPatrolSpeedScale < 0 {
		c.EnemyPatrolSpeedScale = 0
	}
	if c.EnemyAggroLeashScale < 1 {
		c.EnemyAggroLeashScale = 1
	}
	if c.EnemyGunnerPreferredRangeScale <= 0 {
		c.EnemyGunnerPreferredRangeScale = 0.65
	}
	if c.PlayerRollDistance < 0 {
		c.PlayerRollDistance = 0
	}
	if c.EnemyGunnerRollDistance < 0 {
		c.EnemyGunnerRollDistance = 0
	}
	if c.EnemyBladeRollDistance < 0 {
		c.EnemyBladeRollDistance = 0
	}
	if c.PlayerRollMaxCharges < 1 {
		c.PlayerRollMaxCharges = 1
	}
	if c.EnemyGunnerRollMaxCharges < 1 {
		c.EnemyGunnerRollMaxCharges = 1
	}
	if c.EnemyBladeRollMaxCharges < 1 {
		c.EnemyBladeRollMaxCharges = 1
	}
	if c.EnemyMinionSpawnWeight < 0 {
		c.EnemyMinionSpawnWeight = 0
	}
	if c.EnemyEliteSpawnWeight < 0 {
		c.EnemyEliteSpawnWeight = 0
	}
	if c.EnemyBossSpawnWeight < 0 {
		c.EnemyBossSpawnWeight = 0
	}
	if c.HordeBossSpawnWeight < 0 {
		c.HordeBossSpawnWeight = 0
	}
	if c.MaxEnemies < 1 {
		c.MaxEnemies = 1
	}
	if c.EnemySpawnAttempts < 1 {
		c.EnemySpawnAttempts = 1
	}
	if c.EnemyHealthGrowthPerLevel < 0 {
		c.EnemyHealthGrowthPerLevel = 0
	}
	if c.EnemyDamageGrowthPerLevel < 0 {
		c.EnemyDamageGrowthPerLevel = 0
	}
	if c.EnemyAttackSpeedGrowthPerLevel < 0 {
		c.EnemyAttackSpeedGrowthPerLevel = 0
	}
	if c.EnemyEliteBaseHealthScale <= 0 {
		c.EnemyEliteBaseHealthScale = 1
	}
	if c.EnemyBossBaseHealthScale <= 0 {
		c.EnemyBossBaseHealthScale = 1
	}
	if c.EnemyEliteHealthGrowthScale <= 0 {
		c.EnemyEliteHealthGrowthScale = 1
	}
	if c.EnemyBossHealthGrowthScale <= 0 {
		c.EnemyBossHealthGrowthScale = 1
	}
	if c.EnemyEliteDamageGrowthScale <= 0 {
		c.EnemyEliteDamageGrowthScale = 1
	}
	if c.EnemyBossDamageGrowthScale <= 0 {
		c.EnemyBossDamageGrowthScale = 1
	}
	if c.EnemyEliteAttackSpeedScale <= 0 {
		c.EnemyEliteAttackSpeedScale = 1
	}
	if c.EnemyBossAttackSpeedScale <= 0 {
		c.EnemyBossAttackSpeedScale = 1
	}
	if c.EnemyBaseLevel < 1 {
		c.EnemyBaseLevel = 1
	}
	if c.EnemyMinionMinLevel < c.EnemyBaseLevel {
		c.EnemyMinionMinLevel = c.EnemyBaseLevel
	}
	if c.EnemyEliteMinLevel < c.EnemyBaseLevel {
		c.EnemyEliteMinLevel = c.EnemyBaseLevel
	}
	if c.EnemyBossMinLevel < c.EnemyBaseLevel {
		c.EnemyBossMinLevel = c.EnemyBaseLevel
	}
	if c.EnemyMaxLevel < c.EnemyBaseLevel {
		c.EnemyMaxLevel = c.EnemyBaseLevel
	}
	if c.EnemyMinionMinLevel > c.EnemyMaxLevel {
		c.EnemyMinionMinLevel = c.EnemyMaxLevel
	}
	if c.EnemyEliteMinLevel > c.EnemyMaxLevel {
		c.EnemyEliteMinLevel = c.EnemyMaxLevel
	}
	if c.EnemyBossMinLevel > c.EnemyMaxLevel {
		c.EnemyBossMinLevel = c.EnemyMaxLevel
	}
	if c.HordeThreshold < 1 {
		c.HordeThreshold = 1
	}
	if c.HordePeakSpawnInterval <= 0 {
		c.HordePeakSpawnInterval = c.HordeSpawnInterval
	}
	if c.HordePeakSpawnInterval > c.HordeSpawnInterval {
		c.HordePeakSpawnInterval = c.HordeSpawnInterval
	}
	if c.HordeValuePerMinionKill < 0 {
		c.HordeValuePerMinionKill = 0
	}
	if c.HordeValuePerEliteKill < 0 {
		c.HordeValuePerEliteKill = 0
	}
	if c.HordeValuePerBossKill < 0 {
		c.HordeValuePerBossKill = 0
	}
	if c.EnemyMinionSpawnWeight+c.EnemyEliteSpawnWeight+c.EnemyBossSpawnWeight <= 0 {
		c.EnemyMinionSpawnWeight = 1
	}
	c.EnemySkillDropChance = clampUnit(c.EnemySkillDropChance)
	c.EnemyBonusLevelChance = clampUnit(c.EnemyBonusLevelChance)
	c.PlayerKnockbackResistance = clampUnit(c.PlayerKnockbackResistance)
	c.EnemyGunnerKnockbackResistance = clampUnit(c.EnemyGunnerKnockbackResistance)
	c.EnemyBladeKnockbackResistance = clampUnit(c.EnemyBladeKnockbackResistance)
	return c
}

func (c Config) runtimeParams() runtimeParams {
	c = c.normalized()
	return runtimeParams{
		InitialBulletLifeFrames:        framesFromDuration(c.InitialBulletLife, c.TickHz),
		ExpOrbLifetimeFrames:           framesFromDuration(c.ExpOrbLifetime, c.TickHz),
		SkillDropLifetimeFrames:        framesFromDuration(c.SkillDropLifetime, c.TickHz),
		EnemyGunnerMoveSpeed:           unitsPerTick(c.EnemyGunnerMoveUnitsPerSecond, c.TickHz),
		EnemyBladeMoveSpeed:            unitsPerTick(c.EnemyBladeMoveUnitsPerSecond, c.TickHz),
		PlayerMoveSpeed:                unitsPerTick(c.PlayerMoveUnitsPerSecond, c.TickHz),
		PlayerBulletSpeed:              unitsPerTick(c.PlayerBulletUnitsPerSecond, c.TickHz),
		EnemyBulletSpeed:               unitsPerTick(c.EnemyBulletUnitsPerSecond, c.TickHz),
		BulletKnockbackPerTick:         unitsPerTick(c.BulletKnockbackUnitsPerSecond, c.TickHz),
		EnemyBladeKnockbackPerTick:     unitsPerTick(c.EnemyBladeKnockbackUnitsPerSecond, c.TickHz),
		ExpOrbMoveSpeed:                unitsPerTick(c.ExpOrbMoveUnitsPerSecond, c.TickHz),
		KnockbackDurationFrames:        framesFromDuration(c.KnockbackDuration, c.TickHz),
		RollDurationFrames:             framesFromDuration(c.RollDuration, c.TickHz),
		RollLockOnHitFrames:            framesFromDuration(c.RollLockOnHit, c.TickHz),
		PlayerFireCooldownFrames:       framesFromDuration(c.PlayerFireCooldown, c.TickHz),
		PlayerRollCooldownFrames:       framesFromDuration(c.PlayerRollCooldown, c.TickHz),
		EnemyGunnerFireCooldownFrames:  framesFromDuration(c.EnemyGunnerFireCooldown, c.TickHz),
		EnemyGunnerRollCooldownFrames:  framesFromDuration(c.EnemyGunnerRollCooldown, c.TickHz),
		EnemyBladeAttackCooldownFrames: framesFromDuration(c.EnemyBladeAttackCooldown, c.TickHz),
		EnemyBladeRollCooldownFrames:   framesFromDuration(c.EnemyBladeRollCooldown, c.TickHz),
		EnemyAlertLockDelayFrames:      framesFromDuration(c.EnemyAlertLockDelay, c.TickHz),
		EnemyPatrolWaitFrames:          framesFromDuration(c.EnemyPatrolWait, c.TickHz),
		EnemyPatrolMoveFrames:          framesFromDuration(c.EnemyPatrolMove, c.TickHz),
		SpawnTimerFrames:               framesFromDuration(c.SpawnInterval, c.TickHz),
		HordeSpawnTimerFrames:          framesFromDuration(c.HordeSpawnInterval, c.TickHz),
		HordePeakSpawnTimerFrames:      framesFromDuration(c.HordePeakSpawnInterval, c.TickHz),
		HordeDurationFrames:            framesFromDuration(c.HordeDuration, c.TickHz),
		EnemyCleanupLifetimeFrames:     framesFromDuration(c.EnemyCleanupLifetime, c.TickHz),
		EnemyMinionSpawnDelayFrames:    framesFromDuration(c.EnemyMinionSpawnDelay, c.TickHz),
		EnemyEliteSpawnDelayFrames:     framesFromDuration(c.EnemyEliteSpawnDelay, c.TickHz),
		EnemyBossSpawnDelayFrames:      framesFromDuration(c.EnemyBossSpawnDelay, c.TickHz),
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

func clampUnit(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
