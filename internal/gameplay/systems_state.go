package gameplay

import (
	"agentDemo/internal/netproto"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updateMovementLocked() {
	bulletFilter := generic.NewFilter3[Position, Velocity, PreviousPosition]().With(generic.T[BulletTag]())
	bulletQuery := bulletFilter.Query(&r.world)
	for bulletQuery.Next() {
		pos, vel, prevPos := bulletQuery.Get()
		prevPos.X = pos.X
		prevPos.Y = pos.Y
		pos.X += vel.X
		pos.Y += vel.Y
	}

	playerFilter := generic.NewFilter2[Position, Velocity]().With(generic.T[PlayerTag]())
	playerQuery := playerFilter.Query(&r.world)
	for playerQuery.Next() {
		pos, vel := playerQuery.Get()
		pos.X += vel.X
		pos.Y += vel.Y
	}

	enemyFilter := generic.NewFilter2[Position, Velocity]().With(generic.T[EnemyTag]())
	enemyQuery := enemyFilter.Query(&r.world)
	for enemyQuery.Next() {
		pos, vel := enemyQuery.Get()
		pos.X += vel.X
		pos.Y += vel.Y
	}
}

func (r *Runtime) updateLifetimeLocked() {
	filter := generic.NewFilter1[Lifetime]()
	query := filter.Query(&r.world)
	var toRemove []ecs.Entity

	for query.Next() {
		lifeTime := query.Get()
		lifeTime.Frames--
		if lifeTime.Frames <= 0 {
			toRemove = append(toRemove, query.Entity())
		}
	}

	for _, entity := range toRemove {
		r.world.RemoveEntity(entity)
	}
}

func (r *Runtime) applyPlayerBulletDamageLocked() {
	bulletFilter := generic.NewFilter4[Damage, Position, Radius, PreviousPosition]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	targetFilter := generic.NewFilter2[Position, Radius]().With(generic.T[EnemyTag]())
	bullets := r.snapshotBulletsLocked(bulletFilter)
	if len(bullets) == 0 {
		return
	}

	targets := r.snapshotHitTargetsLocked(targetFilter)
	if len(targets) == 0 {
		return
	}

	// Player bullets sample along their frame path. This keeps the visible hit
	// point closer to endpoint collision while still preventing obvious tunneling.
	hitEvents, bulletsToRemove := collectSampledHitEvents(bullets, targets, r.cfg.PlayerBulletCollisionSampleStep)
	r.applyHitEventsLocked(hitEvents)
	r.removeEntitiesLocked(bulletsToRemove)
}

func (r *Runtime) applyEnemyBulletDamageLocked() {
	bulletFilter := generic.NewFilter4[Damage, Position, Radius, PreviousPosition]().With(generic.T2[BulletTag, EnemyBulletTag]()...)
	targetFilter := generic.NewFilter2[Position, Radius]().With(generic.T[PlayerTag]())
	r.applyBulletDamageLocked(bulletFilter, targetFilter)
}

func (r *Runtime) applyBulletDamageLocked(bulletFilter *generic.Filter4[Damage, Position, Radius, PreviousPosition], targetFilter *generic.Filter2[Position, Radius]) {
	bullets := r.snapshotBulletsLocked(bulletFilter)
	if len(bullets) == 0 {
		return
	}

	targets := r.snapshotHitTargetsLocked(targetFilter)
	if len(targets) == 0 {
		return
	}

	hitEvents, bulletsToRemove := collectSweepHitEvents(bullets, targets)
	r.applyHitEventsLocked(hitEvents)
	r.removeEntitiesLocked(bulletsToRemove)
}

func (r *Runtime) applyHitEventsLocked(hitEvents []HitEvent) {
	if len(hitEvents) == 0 {
		return
	}

	for _, hitEvent := range hitEvents {
		health := (*Health)(r.world.Get(hitEvent.Target, r.ids.health))
		health.Value -= hitEvent.Damage
		r.lastImpacts = append(r.lastImpacts, r.buildImpactEventLocked(hitEvent, health.Value <= 0))
	}
}

func (r *Runtime) buildImpactEventLocked(hitEvent HitEvent, targetDestroyed bool) *netproto.ImpactEvent {
	bulletNetworkID := (*NetworkID)(r.world.Get(hitEvent.Bullet, r.ids.networkID))
	targetNetworkID := (*NetworkID)(r.world.Get(hitEvent.Target, r.ids.networkID))
	targetPos := (*Position)(r.world.Get(hitEvent.Target, r.ids.position))

	return &netproto.ImpactEvent{
		BulletNetId:     bulletNetworkID.Value,
		BulletKind:      r.entityKindLocked(hitEvent.Bullet),
		SourcePlayerId:  r.ownerPlayerIDLocked(hitEvent.Bullet),
		TargetNetId:     targetNetworkID.Value,
		TargetKind:      r.entityKindLocked(hitEvent.Target),
		Pos:             &netproto.Vec2{X: float32(targetPos.X), Y: float32(targetPos.Y)},
		Damage:          int32(hitEvent.Damage),
		TargetDestroyed: targetDestroyed,
	}
}

func (r *Runtime) removeEntitiesLocked(entities map[uint32]ecs.Entity) {
	for _, entity := range entities {
		r.world.RemoveEntity(entity)
	}
}

func (r *Runtime) cleanupDeadLocked() {
	filter := generic.NewFilter1[Health]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	var toRemove []ecs.Entity
	for query.Next() {
		health := query.Get()
		if health.Value <= 0 {
			toRemove = append(toRemove, query.Entity())
		}
	}

	score := r.getScoreLocked()
	for _, entity := range toRemove {
		r.world.RemoveEntity(entity)
		score.Value++
	}
}

func (r *Runtime) updateGameStateLocked() {
	filter := generic.NewFilter1[Health]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	hasLivePlayer := false
	for query.Next() {
		health := query.Get()
		if health.Value > 0 {
			hasLivePlayer = true
			query.Close()
			break
		}
	}
	r.getGameStateLocked().Running = hasLivePlayer
}
