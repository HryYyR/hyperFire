package gameplay

import (
	"math"

	"agentDemo/internal/netproto"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updateExpOrbSeekLocked() {
	targets := r.snapshotPlayersLocked()
	if len(targets) == 0 {
		return
	}

	filter := generic.NewFilter2[Position, Velocity]().With(generic.T2[PickupTag, ExpOrbTag]()...)
	query := filter.Query(&r.world)
	for query.Next() {
		pos, vel := query.Get()
		target, found := findNearestTargetInRadius(pos.X, pos.Y, targets, r.cfg.ExpOrbMagnetRadius)
		if !found {
			vel.X = 0
			vel.Y = 0
			continue
		}

		velX, velY, ok := calcAimVelocity(pos.X, pos.Y, target.X, target.Y, r.params.ExpOrbMoveSpeed)
		if !ok {
			vel.X = 0
			vel.Y = 0
			continue
		}

		vel.X = velX
		vel.Y = velY
	}
}

func (r *Runtime) applyExpPickupLocked() {
	pickupFilter := generic.NewFilter3[Position, Radius, PickupValue]().With(generic.T2[PickupTag, ExpOrbTag]()...)
	playerFilter := generic.NewFilter4[Position, Radius, OwnerPlayerID, Health]().With(generic.T[PlayerTag]())

	pickups := r.snapshotExpOrbsLocked(pickupFilter)
	if len(pickups) == 0 {
		return
	}

	players := r.snapshotPlayerPickupsLocked(playerFilter)
	if len(players) == 0 {
		return
	}

	pickupsToRemove := make(map[uint32]struct{})
	for _, pickup := range pickups {
		for _, player := range players {
			if !judgeHit(pickup.X, pickup.Y, player.X, player.Y, pickup.Radius, player.Radius) {
				continue
			}
			if _, exists := pickupsToRemove[pickup.Entity.ID()]; exists {
				break
			}
			playerExp := (*Experience)(r.world.Get(player.Entity, r.ids.experience))
			playerExp.Value += pickup.Value
			r.lastPickupEvents = append(r.lastPickupEvents, r.buildPickupEventLocked(
				player.Entity,
				pickup.Entity,
				Position{X: pickup.X, Y: pickup.Y},
				pickup.Value,
				"",
				true,
			))
			pickupsToRemove[pickup.Entity.ID()] = struct{}{}
			break
		}
	}

	if len(pickupsToRemove) == 0 {
		return
	}

	filter := generic.NewFilter1[PickupValue]().With(generic.T2[PickupTag, ExpOrbTag]()...)
	query := filter.Query(&r.world)
	toRemove := make([]ecs.Entity, 0, len(pickupsToRemove))
	for query.Next() {
		_ = query.Get()
		entity := query.Entity()
		if _, ok := pickupsToRemove[entity.ID()]; ok {
			// Arche queries lock structural changes on the world.
			// Collect entities first, then remove them after iteration finishes.
			toRemove = append(toRemove, entity)
		}
	}
	for _, entity := range toRemove {
		r.world.RemoveEntity(entity)
	}
}

func (r *Runtime) applySkillPickupLocked() {
	pickupFilter := generic.NewFilter3[Position, Radius, SkillDrop]().With(generic.T2[PickupTag, SkillDropTag]()...)
	playerFilter := generic.NewFilter4[Position, Radius, OwnerPlayerID, Health]().With(generic.T[PlayerTag]())
	enemyFilter := generic.NewFilter3[Position, Radius, Health]().With(generic.T[EnemyTag]())

	drops := r.snapshotSkillDropsLocked(pickupFilter)
	if len(drops) == 0 {
		return
	}

	players := r.snapshotPlayerPickupsLocked(playerFilter)
	enemies := r.snapshotPickupCollectorsLocked(enemyFilter)
	if len(players) == 0 && len(enemies) == 0 {
		return
	}

	toRemove := make(map[uint32]struct{})
	for _, drop := range drops {
		if collector, collected := r.collectSkillDropForPlayersLocked(drop, players); collected {
			r.lastPickupEvents = append(r.lastPickupEvents, r.buildPickupEventLocked(
				collector,
				drop.Entity,
				Position{X: drop.X, Y: drop.Y},
				0,
				drop.SkillID,
				true,
			))
			toRemove[drop.Entity.ID()] = struct{}{}
			continue
		}
		if collector, collected := r.collectSkillDropForEnemiesLocked(drop, enemies); collected {
			r.lastPickupEvents = append(r.lastPickupEvents, r.buildPickupEventLocked(
				collector,
				drop.Entity,
				Position{X: drop.X, Y: drop.Y},
				0,
				drop.SkillID,
				true,
			))
			toRemove[drop.Entity.ID()] = struct{}{}
		}
	}

	if len(toRemove) == 0 {
		return
	}

	filter := generic.NewFilter1[SkillDrop]().With(generic.T2[PickupTag, SkillDropTag]()...)
	query := filter.Query(&r.world)
	var entities []ecs.Entity
	for query.Next() {
		_ = query.Get()
		entity := query.Entity()
		if _, ok := toRemove[entity.ID()]; ok {
			entities = append(entities, entity)
		}
	}
	for _, entity := range entities {
		r.world.RemoveEntity(entity)
	}
}

func (r *Runtime) collectSkillDropForPlayersLocked(drop SkillDropSnapshot, players []PlayerPickupSnapshot) (ecs.Entity, bool) {
	for _, player := range players {
		if !judgeHit(drop.X, drop.Y, player.X, player.Y, drop.Radius, player.Radius) {
			continue
		}
		valid, _ := r.tryGrantSkillLocked(player.Entity, drop.SkillID)
		if valid {
			return player.Entity, true
		}
	}
	return ecs.Entity{}, false
}

func (r *Runtime) collectSkillDropForEnemiesLocked(drop SkillDropSnapshot, enemies []CollectorSnapshot) (ecs.Entity, bool) {
	for _, enemy := range enemies {
		if !r.enemyCanUseSkillDropsLocked(enemy.Entity) {
			continue
		}
		if !judgeHit(drop.X, drop.Y, enemy.X, enemy.Y, drop.Radius, enemy.Radius) {
			continue
		}
		valid, _ := r.tryGrantSkillLocked(enemy.Entity, drop.SkillID)
		if valid {
			return enemy.Entity, true
		}
	}
	return ecs.Entity{}, false
}

func (r *Runtime) spawnExpOrbLocked(pos Position, value int32) {
	if value <= 0 {
		return
	}

	if r.mergeExpOrbLocked(pos, value) {
		return
	}

	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.radius,
		r.ids.lifetime,
		r.ids.pickupValue,
		r.ids.networkID,
		r.ids.pickupTag,
		r.ids.expOrbTag,
	)

	orbPos := (*Position)(r.world.Get(entity, r.ids.position))
	orbVel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	orbRadius := (*Radius)(r.world.Get(entity, r.ids.radius))
	orbLifetime := (*Lifetime)(r.world.Get(entity, r.ids.lifetime))
	orbValue := (*PickupValue)(r.world.Get(entity, r.ids.pickupValue))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))

	*orbPos = pos
	orbVel.X = 0
	orbVel.Y = 0
	orbRadius.Value = r.cfg.ExpOrbRadius
	orbLifetime.Frames = r.params.ExpOrbLifetimeFrames
	orbValue.Value = value
	networkID.Value = r.allocNetIDLocked()
}

func (r *Runtime) mergeExpOrbLocked(pos Position, value int32) bool {
	filter := generic.NewFilter4[Position, Lifetime, PickupValue, NetworkID]().With(generic.T2[PickupTag, ExpOrbTag]()...)
	query := filter.Query(&r.world)

	bestDist2 := r.cfg.ExpOrbMergeRadius * r.cfg.ExpOrbMergeRadius
	var bestEntity ecs.Entity
	found := false

	for query.Next() {
		orbPos, _, _, _ := query.Get()
		dx := orbPos.X - pos.X
		dy := orbPos.Y - pos.Y
		dist2 := dx*dx + dy*dy
		if dist2 > bestDist2 {
			continue
		}
		bestDist2 = dist2
		bestEntity = query.Entity()
		found = true
	}

	if !found {
		return false
	}

	orbPos := (*Position)(r.world.Get(bestEntity, r.ids.position))
	orbLifetime := (*Lifetime)(r.world.Get(bestEntity, r.ids.lifetime))
	orbValue := (*PickupValue)(r.world.Get(bestEntity, r.ids.pickupValue))

	totalValue := orbValue.Value + value
	if totalValue <= 0 {
		return true
	}

	weight := float64(value) / float64(totalValue)
	orbPos.X += (pos.X - orbPos.X) * weight
	orbPos.Y += (pos.Y - orbPos.Y) * weight
	orbValue.Value = totalValue
	orbLifetime.Frames = int(math.Max(float64(orbLifetime.Frames), float64(r.params.ExpOrbLifetimeFrames)))
	return true
}

func (r *Runtime) spawnSkillDropLocked(pos Position, skillID string) {
	if skillID == "" {
		return
	}

	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.radius,
		r.ids.lifetime,
		r.ids.skillDrop,
		r.ids.networkID,
		r.ids.pickupTag,
		r.ids.skillDropTag,
	)

	dropPos := (*Position)(r.world.Get(entity, r.ids.position))
	dropVel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	dropRadius := (*Radius)(r.world.Get(entity, r.ids.radius))
	dropLifetime := (*Lifetime)(r.world.Get(entity, r.ids.lifetime))
	dropSkill := (*SkillDrop)(r.world.Get(entity, r.ids.skillDrop))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))

	*dropPos = pos
	dropVel.X = 0
	dropVel.Y = 0
	dropRadius.Value = r.cfg.SkillDropRadius
	dropLifetime.Frames = r.params.SkillDropLifetimeFrames
	dropSkill.SkillID = skillID
	networkID.Value = r.allocNetIDLocked()
}

func (r *Runtime) buildPickupEventLocked(collector ecs.Entity, pickup ecs.Entity, pos Position, expValue int32, skillID string, granted bool) *netproto.PickupEvent {
	event := &netproto.PickupEvent{
		Pos:      &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
		ExpValue: expValue,
		SkillId:  skillID,
		Granted:  granted,
	}

	if !collector.IsZero() && r.world.Alive(collector) {
		if r.world.Has(collector, r.ids.networkID) {
			collectorNetID := (*NetworkID)(r.world.Get(collector, r.ids.networkID))
			event.CollectorNetId = collectorNetID.Value
		}
		event.CollectorKind = r.entityKindLocked(collector)
		event.CollectorPlayerId = r.ownerPlayerIDLocked(collector)
	}

	if !pickup.IsZero() && r.world.Alive(pickup) {
		if r.world.Has(pickup, r.ids.networkID) {
			pickupNetID := (*NetworkID)(r.world.Get(pickup, r.ids.networkID))
			event.PickupNetId = pickupNetID.Value
		}
		event.PickupKind = r.entityKindLocked(pickup)
	}

	return event
}

func (r *Runtime) enemyCanUseSkillDropsLocked(entity ecs.Entity) bool {
	if entity.IsZero() || !r.world.Alive(entity) || !r.world.Has(entity, r.ids.enemyTier) {
		return false
	}
	tier := (*EnemyTier)(r.world.Get(entity, r.ids.enemyTier))
	return tier.Value != EnemyTierMinion
}
