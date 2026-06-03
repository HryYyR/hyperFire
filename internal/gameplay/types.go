package gameplay

import (
	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
)

type Position struct{ X, Y float64 }
type SpawnPoint struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }
type Knockback struct {
	X      float64
	Y      float64
	Frames int
}
type LifeState struct{ Dead bool }
type PreviousPosition struct{ X, Y float64 }
type Radius struct{ Value float64 }
type EnemyClass struct{ Value uint8 }
type EnemyTier struct{ Value uint8 }
type EnemyLevel struct{ Value uint32 }
type EnemySpawnState struct {
	RemainingFrames int
	TotalFrames     int
}
type AggroTargetPlayerID struct{ Value uint32 }
type AggroWatchState struct {
	CandidatePlayerID uint32
	Frames            int
}
type EnemyMoveState struct {
	PatrolWaitFrames int
	PatrolMoveFrames int
	PatrolDirX       float64
	PatrolDirY       float64
	StrafeSign       float64
	ArcSign          float64
}
type EnemyLifecycle struct {
	AgeFrames        int
	HasTouchedPlayer bool
}
type RollStats struct {
	DurationFrames int
	CooldownFrames int
	MaxCharges     int
	Distance       float64
}
type RollState struct {
	ActiveFrames   int
	CooldownFrames int
	Charges        int
	DirX           float64
	DirY           float64
	Speed          float64
	InputHeld      bool
}
type RollLock struct{ Frames int }
type PlayerLevel struct{ Value uint32 }
type SkillProgress struct {
	SkillID    string
	Level      int
	ShotsFired int
}
type SkillInventory struct{ Skills []SkillProgress }
type PendingSkillChoices struct {
	NextSequence uint32
	Queue        []PendingSkillChoice
}
type PendingSkillChoice struct {
	Sequence     uint32
	TargetLevel  uint32
	SkillOptions []string
}
type AttackEffects struct{ OnHit []skillcfg.EffectConfig }
type ActiveBuffs struct{ Items []BuffInstance }

type BuffInstance struct {
	Category            skillcfg.BuffCategory
	StackingRule        skillcfg.BuffStackingRule
	Status              skillcfg.StatusKind
	RemainingFrames     int
	TickIntervalFrames  int
	TickFramesRemaining int
	DamagePerTick       int
	MoveSpeedMultiplier float64
	Stacks              int
	MaxStacks           int
}
type HomingProjectile struct {
	SearchRadius float64
	Speed        float64
	Target       ecs.Entity
}
type SkillDrop struct{ SkillID string }

type Health struct{ Value int }
type MaxHealth struct{ Value int }
type Experience struct{ Value int32 }
type Damage struct{ Value int }
type KnockbackForce struct{ Value float64 }
type KnockbackResistance struct{ Value float64 }
type Lifetime struct{ Frames int }
type FireCooldown struct{ Frames int }
type NetworkID struct{ Value uint32 }
type OwnerPlayerID struct{ Value uint32 }
type LastHitByPlayerID struct{ Value uint32 }
type PickupValue struct{ Value int32 }
type DropValue struct{ Value int32 }

type PlayerTag struct{}
type EnemyTag struct{}
type BulletTag struct{}
type PlayerBulletTag struct{}
type EnemyBulletTag struct{}
type PickupTag struct{}
type ExpOrbTag struct{}
type SkillDropTag struct{}
type AutoFire struct{}

type SpawnTimer struct{ Frames int }
type GameState struct{ Running bool }
type HordeState struct {
	Value                 int32
	Threshold             int32
	Active                bool
	RemainingFrames       int
	TotalFrames           int
	BossSpawnedThisWave   bool
	GuaranteedBossPending bool
}

const (
	EnemyClassUnknown uint8 = iota
	EnemyClassGunner
	EnemyClassBlade
)

const (
	EnemyTierUnknown uint8 = iota
	EnemyTierMinion
	EnemyTierElite
	EnemyTierBoss
)

type TargetSnapshot struct {
	Entity   ecs.Entity
	PlayerID uint32
	X        float64
	Y        float64
}

type HitTargetSnapshot struct {
	Entity        ecs.Entity
	OwnerPlayerID uint32
	X             float64
	Y             float64
	Radius        float64
}

type BulletSnapshot struct {
	Entity        ecs.Entity
	OwnerPlayerID uint32
	Damage        int
	Knockback     float64
	OnHitEffects  []skillcfg.EffectConfig
	X             float64
	Y             float64
	PrevX         float64
	PrevY         float64
	Radius        float64
}

type ExpOrbSnapshot struct {
	Entity ecs.Entity
	X      float64
	Y      float64
	Radius float64
	Value  int32
}

type PlayerPickupSnapshot struct {
	Entity   ecs.Entity
	PlayerID uint32
	X        float64
	Y        float64
	Radius   float64
}

type CollectorSnapshot struct {
	Entity ecs.Entity
	X      float64
	Y      float64
	Radius float64
}

type SkillDropSnapshot struct {
	Entity  ecs.Entity
	SkillID string
	X       float64
	Y       float64
	Radius  float64
}

type BulletSpawn struct {
	Position       Position
	Velocity       Velocity
	OwnerPlayerID  uint32
	DamageOverride int
	OnHitEffects   []skillcfg.EffectConfig
	Homing         *HomingProjectile
}

type SkillChoiceResult struct {
	PlayerID       uint32
	ChoiceSequence uint32
	Accepted       bool
	Granted        bool
	Message        string
}

type HitEvent struct {
	Bullet       ecs.Entity
	Target       ecs.Entity
	Damage       int
	KnockbackX   float64
	KnockbackY   float64
	OnHitEffects []skillcfg.EffectConfig
}

type componentIDs struct {
	position            ecs.ID
	spawnPoint          ecs.ID
	velocity            ecs.ID
	knockback           ecs.ID
	lifeState           ecs.ID
	prevPosition        ecs.ID
	radius              ecs.ID
	enemyClass          ecs.ID
	enemyTier           ecs.ID
	enemyLevel          ecs.ID
	enemySpawnState     ecs.ID
	aggroTargetPlayerID ecs.ID
	aggroWatchState     ecs.ID
	enemyMoveState      ecs.ID
	enemyLifecycle      ecs.ID
	rollStats           ecs.ID
	rollState           ecs.ID
	rollLock            ecs.ID
	playerLevel         ecs.ID
	skillInventory      ecs.ID
	pendingSkillChoices ecs.ID
	attackEffects       ecs.ID
	activeBuffs         ecs.ID
	homingProjectile    ecs.ID
	skillDrop           ecs.ID
	health              ecs.ID
	maxHealth           ecs.ID
	experience          ecs.ID
	damage              ecs.ID
	knockbackForce      ecs.ID
	knockbackResistance ecs.ID
	lifetime            ecs.ID
	fireCooldown        ecs.ID
	networkID           ecs.ID
	ownerPlayerID       ecs.ID
	lastHitByPlayerID   ecs.ID
	pickupValue         ecs.ID
	dropValue           ecs.ID

	playerTag       ecs.ID
	enemyTag        ecs.ID
	bulletTag       ecs.ID
	playerBulletTag ecs.ID
	enemyBulletTag  ecs.ID
	pickupTag       ecs.ID
	expOrbTag       ecs.ID
	skillDropTag    ecs.ID
	autoFireTag     ecs.ID
}
