package gameplay

import "github.com/mlange-42/arche/ecs"

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }
type PreviousPosition struct{ X, Y float64 }
type Radius struct{ Value float64 }

type Health struct{ Value int }
type Damage struct{ Value int }
type Lifetime struct{ Frames int }
type FireCooldown struct{ Frames int }
type NetworkID struct{ Value uint32 }
type OwnerPlayerID struct{ Value uint32 }

type PlayerTag struct{}
type EnemyTag struct{}
type BulletTag struct{}
type PlayerBulletTag struct{}
type EnemyBulletTag struct{}
type AutoFire struct{}

type SpawnTimer struct{ Frames int }
type Score struct{ Value int32 }
type GameState struct{ Running bool }

type TargetSnapshot struct {
	Entity ecs.Entity
	X      float64
	Y      float64
}

type HitTargetSnapshot struct {
	Entity ecs.Entity
	X      float64
	Y      float64
	Radius float64
}

type BulletSnapshot struct {
	Entity ecs.Entity
	Damage int
	X      float64
	Y      float64
	PrevX  float64
	PrevY  float64
	Radius float64
}

type BulletSpawn struct {
	Position      Position
	Velocity      Velocity
	OwnerPlayerID uint32
}

type HitEvent struct {
	Bullet ecs.Entity
	Target ecs.Entity
	Damage int
}

type componentIDs struct {
	position      ecs.ID
	velocity      ecs.ID
	prevPosition  ecs.ID
	radius        ecs.ID
	health        ecs.ID
	damage        ecs.ID
	lifetime      ecs.ID
	fireCooldown  ecs.ID
	networkID     ecs.ID
	ownerPlayerID ecs.ID

	playerTag       ecs.ID
	enemyTag        ecs.ID
	bulletTag       ecs.ID
	playerBulletTag ecs.ID
	enemyBulletTag  ecs.ID
	autoFireTag     ecs.ID
}
