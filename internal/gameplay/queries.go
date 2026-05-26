package gameplay

import (
	"agentDemo/internal/netproto"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) snapshotPlayersLocked() []TargetSnapshot {
	filter := generic.NewFilter1[Position]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var targets []TargetSnapshot
	for query.Next() {
		pos := query.Get()
		targets = append(targets, TargetSnapshot{
			Entity: query.Entity(),
			X:      pos.X,
			Y:      pos.Y,
		})
	}
	return targets
}

func (r *Runtime) snapshotHitTargetsLocked(filter *generic.Filter2[Position, Radius]) []HitTargetSnapshot {
	query := filter.Query(&r.world)
	var targets []HitTargetSnapshot
	for query.Next() {
		pos, radius := query.Get()
		targets = append(targets, HitTargetSnapshot{
			Entity: query.Entity(),
			X:      pos.X,
			Y:      pos.Y,
			Radius: radius.Value,
		})
	}
	return targets
}

func (r *Runtime) snapshotBulletsLocked(filter *generic.Filter4[Damage, Position, Radius, PreviousPosition]) []BulletSnapshot {
	query := filter.Query(&r.world)
	var bullets []BulletSnapshot
	for query.Next() {
		damage, pos, radius, prevPos := query.Get()
		bullets = append(bullets, BulletSnapshot{
			Entity: query.Entity(),
			Damage: damage.Value,
			X:      pos.X,
			Y:      pos.Y,
			PrevX:  prevPos.X,
			PrevY:  prevPos.Y,
			Radius: radius.Value,
		})
	}
	return bullets
}

func (r *Runtime) exportPlayersLocked() []*netproto.EntityState {
	filter := generic.NewFilter6[Position, Velocity, Health, Radius, NetworkID, OwnerPlayerID]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, health, radius, networkID, owner := query.Get()
		entities = append(entities, &netproto.EntityState{
			NetId:         networkID.Value,
			Kind:          netproto.EntityKind_ENTITY_KIND_PLAYER,
			OwnerPlayerId: owner.Value,
			Pos:           &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:           &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Hp:            int32(health.Value),
			Radius:        float32(radius.Value),
		})
	}
	return entities
}

func (r *Runtime) exportEnemiesLocked() []*netproto.EntityState {
	filter := generic.NewFilter5[Position, Velocity, Health, Radius, NetworkID]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, health, radius, networkID := query.Get()
		entities = append(entities, &netproto.EntityState{
			NetId:  networkID.Value,
			Kind:   netproto.EntityKind_ENTITY_KIND_ENEMY,
			Pos:    &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:    &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Hp:     int32(health.Value),
			Radius: float32(radius.Value),
		})
	}
	return entities
}

func (r *Runtime) exportPlayerBulletsLocked() []*netproto.EntityState {
	filter := generic.NewFilter5[Position, Velocity, Radius, NetworkID, OwnerPlayerID]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, radius, networkID, owner := query.Get()
		entities = append(entities, &netproto.EntityState{
			NetId:         networkID.Value,
			Kind:          netproto.EntityKind_ENTITY_KIND_BULLET_PLAYER,
			OwnerPlayerId: owner.Value,
			Pos:           &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:           &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Radius:        float32(radius.Value),
		})
	}
	return entities
}

func (r *Runtime) exportEnemyBulletsLocked() []*netproto.EntityState {
	filter := generic.NewFilter4[Position, Velocity, Radius, NetworkID]().With(generic.T2[BulletTag, EnemyBulletTag]()...)
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, radius, networkID := query.Get()
		entities = append(entities, &netproto.EntityState{
			NetId:  networkID.Value,
			Kind:   netproto.EntityKind_ENTITY_KIND_BULLET_ENEMY,
			Pos:    &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:    &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Radius: float32(radius.Value),
		})
	}
	return entities
}

func (r *Runtime) entityKindLocked(entity ecs.Entity) netproto.EntityKind {
	switch {
	case r.world.Has(entity, r.ids.playerTag):
		return netproto.EntityKind_ENTITY_KIND_PLAYER
	case r.world.Has(entity, r.ids.enemyTag):
		return netproto.EntityKind_ENTITY_KIND_ENEMY
	case r.world.Has(entity, r.ids.playerBulletTag):
		return netproto.EntityKind_ENTITY_KIND_BULLET_PLAYER
	case r.world.Has(entity, r.ids.enemyBulletTag):
		return netproto.EntityKind_ENTITY_KIND_BULLET_ENEMY
	default:
		return netproto.EntityKind_ENTITY_KIND_UNKNOWN
	}
}

func (r *Runtime) ownerPlayerIDLocked(entity ecs.Entity) uint32 {
	if !r.world.Has(entity, r.ids.ownerPlayerID) {
		return 0
	}
	owner := (*OwnerPlayerID)(r.world.Get(entity, r.ids.ownerPlayerID))
	return owner.Value
}
