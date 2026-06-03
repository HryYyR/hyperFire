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

	playerFilter := generic.NewFilter3[Position, Velocity, Knockback]().With(generic.T[PlayerTag]())
	playerQuery := playerFilter.Query(&r.world)
	for playerQuery.Next() {
		pos, vel, knockback := playerQuery.Get()
		rollState := (*RollState)(r.world.Get(playerQuery.Entity(), r.ids.rollState))
		moveX, moveY := combinedVelocity(vel, knockback, rollState)
		pos.X += moveX
		pos.Y += moveY
		advanceKnockback(knockback)
		advanceRoll(rollState)
	}

	enemyFilter := generic.NewFilter3[Position, Velocity, Knockback]().With(generic.T[EnemyTag]())
	enemyQuery := enemyFilter.Query(&r.world)
	for enemyQuery.Next() {
		pos, vel, knockback := enemyQuery.Get()
		rollState := (*RollState)(r.world.Get(enemyQuery.Entity(), r.ids.rollState))
		moveX, moveY := combinedVelocity(vel, knockback, rollState)
		pos.X += moveX
		pos.Y += moveY
		advanceKnockback(knockback)
		advanceRoll(rollState)
	}

	pickupFilter := generic.NewFilter2[Position, Velocity]().With(generic.T[PickupTag]())
	pickupQuery := pickupFilter.Query(&r.world)
	for pickupQuery.Next() {
		pos, vel := pickupQuery.Get()
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
	bulletFilter := generic.NewFilter5[Damage, KnockbackForce, Position, Radius, PreviousPosition]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	bullets := r.snapshotBulletsLocked(bulletFilter)
	if len(bullets) == 0 {
		return
	}

	enemyFilter := generic.NewFilter2[Position, Radius]().With(generic.T[EnemyTag]())
	targets := r.snapshotHitTargetsLocked(enemyFilter)
	targets = append(targets, r.snapshotLivePlayerHitTargetsLocked()...)
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
	bulletFilter := generic.NewFilter5[Damage, KnockbackForce, Position, Radius, PreviousPosition]().With(generic.T2[BulletTag, EnemyBulletTag]()...)
	bullets := r.snapshotBulletsLocked(bulletFilter)
	if len(bullets) == 0 {
		return
	}

	targets := r.snapshotLivePlayerHitTargetsLocked()
	if len(targets) == 0 {
		return
	}

	hitEvents, bulletsToRemove := collectSweepHitEvents(bullets, targets)
	r.applyHitEventsLocked(hitEvents)
	r.removeEntitiesLocked(bulletsToRemove)
}

func (r *Runtime) applyBulletDamageLocked(bulletFilter *generic.Filter5[Damage, KnockbackForce, Position, Radius, PreviousPosition], targetFilter *generic.Filter2[Position, Radius]) {
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
		r.markEnemyAggroLocked(hitEvent)
		r.recordPlayerDamageSourceLocked(hitEvent)

		if r.world.Has(hitEvent.Target, r.ids.knockback) {
			knockback := (*Knockback)(r.world.Get(hitEvent.Target, r.ids.knockback))
			scale := r.knockbackScaleLocked(hitEvent.Target)
			knockbackX := hitEvent.KnockbackX * scale
			knockbackY := hitEvent.KnockbackY * scale
			knockback.X += knockbackX
			knockback.Y += knockbackY
			if (knockbackX != 0 || knockbackY != 0) && knockback.Frames < r.params.KnockbackDurationFrames {
				knockback.Frames = r.params.KnockbackDurationFrames
			}
		}
		if hitEvent.Damage > 0 || hitEvent.KnockbackX != 0 || hitEvent.KnockbackY != 0 {
			r.disableRollForHitLocked(hitEvent.Target)
		}

		health := (*Health)(r.world.Get(hitEvent.Target, r.ids.health))
		health.Value -= hitEvent.Damage
		r.applyOnHitEffectsLocked(hitEvent.Target, hitEvent.OnHitEffects)
		r.lastImpacts = append(r.lastImpacts, r.buildImpactEventLocked(hitEvent, health.Value <= 0))
	}
}

func (r *Runtime) knockbackScaleLocked(target ecs.Entity) float64 {
	if !r.world.Has(target, r.ids.knockbackResistance) {
		return 1
	}

	resistance := (*KnockbackResistance)(r.world.Get(target, r.ids.knockbackResistance))
	return 1 - resistance.Value
}

func (r *Runtime) buildImpactEventLocked(hitEvent HitEvent, targetDestroyed bool) *netproto.ImpactEvent {
	targetNetworkID := (*NetworkID)(r.world.Get(hitEvent.Target, r.ids.networkID))
	targetPos := (*Position)(r.world.Get(hitEvent.Target, r.ids.position))
	bulletNetID := uint32(0)
	bulletKind := netproto.EntityKind_ENTITY_KIND_UNKNOWN
	sourcePlayerID := uint32(0)
	if !hitEvent.Bullet.IsZero() && r.world.Alive(hitEvent.Bullet) {
		bulletNetworkID := (*NetworkID)(r.world.Get(hitEvent.Bullet, r.ids.networkID))
		bulletNetID = bulletNetworkID.Value
		bulletKind = r.entityKindLocked(hitEvent.Bullet)
		sourcePlayerID = r.ownerPlayerIDLocked(hitEvent.Bullet)
	}

	return &netproto.ImpactEvent{
		BulletNetId:     bulletNetID,
		BulletKind:      bulletKind,
		SourcePlayerId:  sourcePlayerID,
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
	type playerExpDrop struct {
		pos   Position
		value int32
	}

	playerFilter := generic.NewFilter6[Position, Velocity, Knockback, Health, Experience, LifeState]().With(generic.T[PlayerTag]())
	playerQuery := playerFilter.Query(&r.world)
	var playerDrops []playerExpDrop
	var deadPlayerIDs []uint32
	for playerQuery.Next() {
		pos, vel, knockback, health, experience, lifeState := playerQuery.Get()
		if health.Value > 0 {
			lifeState.Dead = false
			continue
		}
		if lifeState.Dead {
			continue
		}

		lifeState.Dead = true
		vel.X = 0
		vel.Y = 0
		knockback.X = 0
		knockback.Y = 0
		knockback.Frames = 0
		if r.world.Has(playerQuery.Entity(), r.ids.rollState) {
			rollState := (*RollState)(r.world.Get(playerQuery.Entity(), r.ids.rollState))
			rollStats := (*RollStats)(r.world.Get(playerQuery.Entity(), r.ids.rollStats))
			rollLock := (*RollLock)(r.world.Get(playerQuery.Entity(), r.ids.rollLock))
			resetRollState(rollState, rollStats)
			rollLock.Frames = 0
		}
		r.resetPlayerProgressionStateLocked(playerQuery.Entity())

		dropValue := experience.Value / 2
		experience.Value -= dropValue
		if dropValue > 0 {
			playerDrops = append(playerDrops, playerExpDrop{
				pos:   Position{X: pos.X, Y: pos.Y},
				value: dropValue,
			})
		}
		if r.world.Has(playerQuery.Entity(), r.ids.ownerPlayerID) {
			owner := (*OwnerPlayerID)(r.world.Get(playerQuery.Entity(), r.ids.ownerPlayerID))
			deadPlayerIDs = append(deadPlayerIDs, owner.Value)
		}
	}

	if len(deadPlayerIDs) > 0 {
		r.clearEnemyAggroForPlayersLocked(deadPlayerIDs)
	}

	for _, drop := range playerDrops {
		r.spawnExpOrbLocked(drop.pos, drop.value)
	}

	filter := generic.NewFilter5[Position, Health, DropValue, SkillInventory, EnemyTier]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	type deadEnemy struct {
		entity    ecs.Entity
		pos       Position
		value     int32
		tier      uint8
		killer    uint32
		skillDrop []string
	}
	var toRemove []deadEnemy
	for query.Next() {
		pos, health, dropValue, skills, tier := query.Get()
		if health.Value <= 0 {
			toRemove = append(toRemove, deadEnemy{
				entity:    query.Entity(),
				pos:       Position{X: pos.X, Y: pos.Y},
				value:     dropValue.Value,
				tier:      tier.Value,
				killer:    r.lastHitByPlayerIDLocked(query.Entity()),
				skillDrop: r.rollEnemySkillDropsLocked(skills, tier.Value),
			})
		}
	}

	for _, dead := range toRemove {
		r.world.RemoveEntity(dead.entity)
		r.addHordeValueForEnemyKillLocked(dead.tier)
		r.applyPlayerKillRewardLocked(dead.killer)
		r.spawnExpOrbLocked(dead.pos, dead.value)
		for _, skillID := range dead.skillDrop {
			r.spawnSkillDropLocked(dead.pos, skillID)
		}
	}
}

func (r *Runtime) clearEnemyAggroForPlayersLocked(playerIDs []uint32) {
	if len(playerIDs) == 0 {
		return
	}

	filter := generic.NewFilter2[AggroTargetPlayerID, AggroWatchState]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		aggroTarget, aggroWatch := query.Get()
		if aggroTarget.Value == 0 {
			if aggroWatch.CandidatePlayerID != 0 && containsUint32(playerIDs, aggroWatch.CandidatePlayerID) {
				clearAggroWatchState(aggroWatch)
			}
			continue
		}
		if containsUint32(playerIDs, aggroTarget.Value) {
			aggroTarget.Value = 0
			clearAggroWatchState(aggroWatch)
		}
	}
}

func (r *Runtime) updateGameStateLocked() {
	r.getGameStateLocked().Running = len(r.playerEntities) > 0
}

func advanceKnockback(knockback *Knockback) {
	if knockback == nil || knockback.Frames <= 0 {
		return
	}

	knockback.Frames--
	if knockback.Frames > 0 {
		return
	}

	knockback.X = 0
	knockback.Y = 0
}

func (r *Runtime) markEnemyAggroLocked(hitEvent HitEvent) {
	if hitEvent.Bullet.IsZero() || !r.world.Alive(hitEvent.Bullet) {
		return
	}
	if !r.world.Has(hitEvent.Bullet, r.ids.playerBulletTag) {
		return
	}
	if !r.world.Has(hitEvent.Target, r.ids.enemyTag) || !r.world.Has(hitEvent.Target, r.ids.aggroTargetPlayerID) {
		return
	}

	playerID := r.ownerPlayerIDLocked(hitEvent.Bullet)
	if playerID == 0 {
		return
	}

	aggroTarget := (*AggroTargetPlayerID)(r.world.Get(hitEvent.Target, r.ids.aggroTargetPlayerID))
	aggroTarget.Value = playerID
	if r.world.Has(hitEvent.Target, r.ids.aggroWatchState) {
		aggroWatch := (*AggroWatchState)(r.world.Get(hitEvent.Target, r.ids.aggroWatchState))
		clearAggroWatchState(aggroWatch)
	}
}

func (r *Runtime) recordPlayerDamageSourceLocked(hitEvent HitEvent) {
	if hitEvent.Target.IsZero() || !r.world.Alive(hitEvent.Target) || !r.world.Has(hitEvent.Target, r.ids.enemyTag) || !r.world.Has(hitEvent.Target, r.ids.lastHitByPlayerID) {
		return
	}

	playerID := uint32(0)
	if !hitEvent.Bullet.IsZero() && r.world.Alive(hitEvent.Bullet) {
		playerID = r.ownerPlayerIDLocked(hitEvent.Bullet)
	}
	if playerID == 0 {
		return
	}

	lastHit := (*LastHitByPlayerID)(r.world.Get(hitEvent.Target, r.ids.lastHitByPlayerID))
	lastHit.Value = playerID
}

func (r *Runtime) lastHitByPlayerIDLocked(entity ecs.Entity) uint32 {
	if entity.IsZero() || !r.world.Alive(entity) || !r.world.Has(entity, r.ids.lastHitByPlayerID) {
		return 0
	}
	lastHit := (*LastHitByPlayerID)(r.world.Get(entity, r.ids.lastHitByPlayerID))
	return lastHit.Value
}

func (r *Runtime) applyPlayerKillRewardLocked(playerID uint32) {
	if playerID == 0 {
		return
	}

	entity, ok := r.playerEntities[playerID]
	if !ok || !r.world.Alive(entity) {
		return
	}
	r.applyPlayerKillHealLocked(entity)
}

func (r *Runtime) rollEnemySkillDropsLocked(inventory *SkillInventory, tier uint8) []string {
	if tier == EnemyTierMinion {
		return nil
	}

	available := inventorySkillIDs(inventory)
	if len(available) == 0 {
		return nil
	}

	if tier != EnemyTierBoss && r.rng.Float64() >= r.cfg.EnemySkillDropChance {
		return nil
	}

	if len(available) == 1 {
		return available
	}

	dropCount := 1
	if tier == EnemyTierBoss {
		dropCount += r.rng.Intn(len(available) - 1)
	} else if r.rng.Intn(2) == 0 {
		dropCount++
		if dropCount > len(available) {
			dropCount = len(available)
		}
	}
	perm := r.rng.Perm(len(available))
	result := make([]string, 0, dropCount)
	for _, index := range perm[:dropCount] {
		result = append(result, available[index])
	}
	return result
}
