package gameplay

import (
	"agentDemo/internal/netproto"
	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) snapshotPlayersLocked() []TargetSnapshot {
	filter := generic.NewFilter2[Position, Health]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var targets []TargetSnapshot
	for query.Next() {
		pos, health := query.Get()
		if health.Value <= 0 {
			continue
		}
		targets = append(targets, TargetSnapshot{
			Entity:   query.Entity(),
			PlayerID: r.ownerPlayerIDLocked(query.Entity()),
			X:        pos.X,
			Y:        pos.Y,
		})
	}
	return targets
}

func (r *Runtime) snapshotLivePlayerHitTargetsLocked() []HitTargetSnapshot {
	filter := generic.NewFilter4[Position, Radius, Health, OwnerPlayerID]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var targets []HitTargetSnapshot
	for query.Next() {
		pos, radius, health, owner := query.Get()
		if health.Value <= 0 {
			continue
		}
		targets = append(targets, HitTargetSnapshot{
			Entity:        query.Entity(),
			OwnerPlayerID: owner.Value,
			X:             pos.X,
			Y:             pos.Y,
			Radius:        radius.Value,
		})
	}
	return targets
}

func (r *Runtime) snapshotLiveEnemyTargetsLocked() []TargetSnapshot {
	filter := generic.NewFilter2[Position, Health]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	var targets []TargetSnapshot
	for query.Next() {
		pos, health := query.Get()
		if health.Value <= 0 {
			continue
		}
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
			Entity:        query.Entity(),
			OwnerPlayerID: 0,
			X:             pos.X,
			Y:             pos.Y,
			Radius:        radius.Value,
		})
	}
	return targets
}

func (r *Runtime) snapshotBulletsLocked(filter *generic.Filter5[Damage, KnockbackForce, Position, Radius, PreviousPosition]) []BulletSnapshot {
	query := filter.Query(&r.world)
	var bullets []BulletSnapshot
	for query.Next() {
		damage, knockbackForce, pos, radius, prevPos := query.Get()
		entity := query.Entity()
		attackEffects := (*AttackEffects)(r.world.Get(entity, r.ids.attackEffects))
		bullets = append(bullets, BulletSnapshot{
			Entity:        entity,
			OwnerPlayerID: r.ownerPlayerIDLocked(entity),
			Damage:        damage.Value,
			Knockback:     knockbackForce.Value,
			OnHitEffects:  cloneEffectConfigs(attackEffects.OnHit),
			X:             pos.X,
			Y:             pos.Y,
			PrevX:         prevPos.X,
			PrevY:         prevPos.Y,
			Radius:        radius.Value,
		})
	}
	return bullets
}

func (r *Runtime) snapshotExpOrbsLocked(filter *generic.Filter3[Position, Radius, PickupValue]) []ExpOrbSnapshot {
	query := filter.Query(&r.world)
	var orbs []ExpOrbSnapshot
	for query.Next() {
		pos, radius, value := query.Get()
		orbs = append(orbs, ExpOrbSnapshot{
			Entity: query.Entity(),
			X:      pos.X,
			Y:      pos.Y,
			Radius: radius.Value,
			Value:  value.Value,
		})
	}
	return orbs
}

func (r *Runtime) snapshotSkillDropsLocked(filter *generic.Filter3[Position, Radius, SkillDrop]) []SkillDropSnapshot {
	query := filter.Query(&r.world)
	var drops []SkillDropSnapshot
	for query.Next() {
		pos, radius, skillDrop := query.Get()
		drops = append(drops, SkillDropSnapshot{
			Entity:  query.Entity(),
			SkillID: skillDrop.SkillID,
			X:       pos.X,
			Y:       pos.Y,
			Radius:  radius.Value,
		})
	}
	return drops
}

func (r *Runtime) snapshotPlayerPickupsLocked(filter *generic.Filter4[Position, Radius, OwnerPlayerID, Health]) []PlayerPickupSnapshot {
	query := filter.Query(&r.world)
	var players []PlayerPickupSnapshot
	for query.Next() {
		pos, radius, owner, health := query.Get()
		if health.Value <= 0 {
			continue
		}
		players = append(players, PlayerPickupSnapshot{
			Entity:   query.Entity(),
			PlayerID: owner.Value,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   radius.Value,
		})
	}
	return players
}

func (r *Runtime) snapshotPickupCollectorsLocked(filter *generic.Filter3[Position, Radius, Health]) []CollectorSnapshot {
	query := filter.Query(&r.world)
	var collectors []CollectorSnapshot
	for query.Next() {
		pos, radius, health := query.Get()
		if health.Value <= 0 {
			continue
		}
		collectors = append(collectors, CollectorSnapshot{
			Entity: query.Entity(),
			X:      pos.X,
			Y:      pos.Y,
			Radius: radius.Value,
		})
	}
	return collectors
}

func (r *Runtime) snapshotSkillDropTargetsLocked() []TargetSnapshot {
	filter := generic.NewFilter2[Position, SkillDrop]().With(generic.T2[PickupTag, SkillDropTag]()...)
	query := filter.Query(&r.world)
	var targets []TargetSnapshot
	for query.Next() {
		pos, _ := query.Get()
		targets = append(targets, TargetSnapshot{
			Entity: query.Entity(),
			X:      pos.X,
			Y:      pos.Y,
		})
	}
	return targets
}

func (r *Runtime) exportPlayersLocked() []*netproto.EntityState {
	filter := generic.NewFilter9[Position, Velocity, Knockback, Health, MaxHealth, Experience, Radius, NetworkID, OwnerPlayerID]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, knockback, health, maxHealth, experience, radius, networkID, owner := query.Get()
		rollState := (*RollState)(r.world.Get(query.Entity(), r.ids.rollState))
		totalVelX, totalVelY := combinedVelocity(vel, knockback, rollState)
		entities = append(entities, &netproto.EntityState{
			NetId:         networkID.Value,
			Kind:          netproto.EntityKind_ENTITY_KIND_PLAYER,
			OwnerPlayerId: owner.Value,
			Pos:           &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:           &netproto.Vec2{X: float32(totalVelX), Y: float32(totalVelY)},
			Hp:            int32(health.Value),
			HpMax:         int32(maxHealth.Value),
			Radius:        float32(radius.Value),
			Exp:           experience.Value,
			Buffs:         r.exportBuffStatesLocked(query.Entity()),
		})
	}
	return entities
}

func (r *Runtime) exportEnemiesLocked() []*netproto.EntityState {
	filter := generic.NewFilter10[Position, Velocity, Knockback, Health, MaxHealth, Radius, NetworkID, EnemyClass, EnemyTier, EnemySpawnState]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, knockback, health, maxHealth, radius, networkID, class, tier, spawnState := query.Get()
		entity := query.Entity()
		rollState := (*RollState)(r.world.Get(query.Entity(), r.ids.rollState))
		aggroTarget := (*AggroTargetPlayerID)(r.world.Get(entity, r.ids.aggroTargetPlayerID))
		aggroWatch := (*AggroWatchState)(r.world.Get(entity, r.ids.aggroWatchState))
		totalVelX, totalVelY := combinedVelocity(vel, knockback, rollState)
		entities = append(entities, &netproto.EntityState{
			NetId:               networkID.Value,
			Kind:                netproto.EntityKind_ENTITY_KIND_ENEMY,
			Pos:                 &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:                 &netproto.Vec2{X: float32(totalVelX), Y: float32(totalVelY)},
			Hp:                  int32(health.Value),
			HpMax:               int32(maxHealth.Value),
			Radius:              float32(radius.Value),
			EnemyClass:          r.enemyClassProtoLocked(class.Value),
			Buffs:               r.exportBuffStatesLocked(entity),
			EnemyAggroState:     r.enemyAggroStateProtoLocked(aggroTarget, aggroWatch),
			AggroTargetPlayerId: r.enemyAggroTargetPlayerIDLocked(aggroTarget, aggroWatch),
			AggroWatchFrames:    uint32(clampNonNegativeInt(r.enemyAggroWatchFramesLocked(aggroWatch))),
			AggroWatchTotalFrames: uint32(clampNonNegativeInt(
				r.enemyAggroWatchTotalFramesLocked(aggroTarget, aggroWatch),
			)),
			EnemyTier: r.enemyTierProtoLocked(tier.Value),
			SpawnRemainingFrames: uint32(clampNonNegativeInt(
				spawnState.RemainingFrames,
			)),
			SpawnTotalFrames: uint32(clampNonNegativeInt(
				spawnState.TotalFrames,
			)),
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
		entity := query.Entity()
		entities = append(entities, &netproto.EntityState{
			NetId:             networkID.Value,
			Kind:              netproto.EntityKind_ENTITY_KIND_BULLET_PLAYER,
			OwnerPlayerId:     owner.Value,
			Pos:               &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:               &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Radius:            float32(radius.Value),
			ProjectileSubtype: r.projectileSubtypeLocked(entity),
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
			NetId:             networkID.Value,
			Kind:              netproto.EntityKind_ENTITY_KIND_BULLET_ENEMY,
			Pos:               &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:               &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Radius:            float32(radius.Value),
			ProjectileSubtype: netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_NORMAL,
		})
	}
	return entities
}

func (r *Runtime) exportExpOrbsLocked() []*netproto.EntityState {
	filter := generic.NewFilter4[Position, Velocity, Radius, NetworkID]().With(generic.T2[PickupTag, ExpOrbTag]()...)
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, radius, networkID := query.Get()
		entities = append(entities, &netproto.EntityState{
			NetId:  networkID.Value,
			Kind:   netproto.EntityKind_ENTITY_KIND_PICKUP_EXP,
			Pos:    &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:    &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Radius: float32(radius.Value),
		})
	}
	return entities
}

func (r *Runtime) exportSkillDropsLocked() []*netproto.EntityState {
	filter := generic.NewFilter5[Position, Velocity, Radius, NetworkID, SkillDrop]().With(generic.T2[PickupTag, SkillDropTag]()...)
	query := filter.Query(&r.world)
	var entities []*netproto.EntityState
	for query.Next() {
		pos, vel, radius, networkID, skillDrop := query.Get()
		entities = append(entities, &netproto.EntityState{
			NetId:   networkID.Value,
			Kind:    netproto.EntityKind_ENTITY_KIND_PICKUP_SKILL,
			Pos:     &netproto.Vec2{X: float32(pos.X), Y: float32(pos.Y)},
			Vel:     &netproto.Vec2{X: float32(vel.X), Y: float32(vel.Y)},
			Radius:  float32(radius.Value),
			SkillId: skillDrop.SkillID,
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
	case r.world.Has(entity, r.ids.expOrbTag):
		return netproto.EntityKind_ENTITY_KIND_PICKUP_EXP
	case r.world.Has(entity, r.ids.skillDropTag):
		return netproto.EntityKind_ENTITY_KIND_PICKUP_SKILL
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

func combinedVelocity(base *Velocity, knockback *Knockback, rollState *RollState) (float64, float64) {
	moveX := base.X
	moveY := base.Y
	if rollState != nil && rollState.ActiveFrames > 0 {
		moveX = rollState.DirX * rollState.Speed
		moveY = rollState.DirY * rollState.Speed
	}
	if knockback == nil || knockback.Frames <= 0 {
		return moveX, moveY
	}
	return moveX + knockback.X, moveY + knockback.Y
}

func (r *Runtime) enemyClassProtoLocked(class uint8) netproto.EnemyClass {
	switch class {
	case EnemyClassGunner:
		return netproto.EnemyClass_ENEMY_CLASS_GUNNER
	case EnemyClassBlade:
		return netproto.EnemyClass_ENEMY_CLASS_BLADE
	default:
		return netproto.EnemyClass_ENEMY_CLASS_UNKNOWN
	}
}

func (r *Runtime) enemyTierProtoLocked(tier uint8) netproto.EnemyTier {
	switch tier {
	case EnemyTierMinion:
		return netproto.EnemyTier_ENEMY_TIER_MINION
	case EnemyTierElite:
		return netproto.EnemyTier_ENEMY_TIER_ELITE
	case EnemyTierBoss:
		return netproto.EnemyTier_ENEMY_TIER_BOSS
	default:
		return netproto.EnemyTier_ENEMY_TIER_UNKNOWN
	}
}

func (r *Runtime) projectileSubtypeLocked(entity ecs.Entity) netproto.ProjectileSubtype {
	if entity.IsZero() || !r.world.Alive(entity) {
		return netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_UNKNOWN
	}
	if !r.world.Has(entity, r.ids.bulletTag) {
		return netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_UNKNOWN
	}
	if r.world.Has(entity, r.ids.homingProjectile) {
		return netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_HOMING
	}
	return netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_NORMAL
}

func (r *Runtime) enemyAggroStateProtoLocked(aggroTarget *AggroTargetPlayerID, aggroWatch *AggroWatchState) netproto.EnemyAggroState {
	switch {
	case aggroTarget != nil && aggroTarget.Value != 0:
		return netproto.EnemyAggroState_ENEMY_AGGRO_STATE_LOCKED
	case aggroWatch != nil && aggroWatch.CandidatePlayerID != 0:
		return netproto.EnemyAggroState_ENEMY_AGGRO_STATE_WATCHING
	default:
		return netproto.EnemyAggroState_ENEMY_AGGRO_STATE_IDLE
	}
}

func (r *Runtime) enemyAggroTargetPlayerIDLocked(aggroTarget *AggroTargetPlayerID, aggroWatch *AggroWatchState) uint32 {
	if aggroTarget != nil && aggroTarget.Value != 0 {
		return aggroTarget.Value
	}
	if aggroWatch != nil {
		return aggroWatch.CandidatePlayerID
	}
	return 0
}

func (r *Runtime) enemyAggroWatchFramesLocked(aggroWatch *AggroWatchState) int {
	if aggroWatch == nil || aggroWatch.CandidatePlayerID == 0 {
		return 0
	}
	return aggroWatch.Frames
}

func (r *Runtime) enemyAggroWatchTotalFramesLocked(aggroTarget *AggroTargetPlayerID, aggroWatch *AggroWatchState) int {
	if aggroTarget != nil && aggroTarget.Value != 0 {
		return 0
	}
	if aggroWatch == nil || aggroWatch.CandidatePlayerID == 0 {
		return 0
	}
	return r.params.EnemyAlertLockDelayFrames
}

func (r *Runtime) exportBuffStatesLocked(entity ecs.Entity) []*netproto.BuffState {
	if !r.world.Has(entity, r.ids.activeBuffs) {
		return nil
	}

	activeBuffs := (*ActiveBuffs)(r.world.Get(entity, r.ids.activeBuffs))
	if activeBuffs == nil || len(activeBuffs.Items) == 0 {
		return nil
	}

	result := make([]*netproto.BuffState, 0, len(activeBuffs.Items))
	for _, buff := range activeBuffs.Items {
		stacks := positiveOrOne(buff.Stacks)
		result = append(result, &netproto.BuffState{
			Kind:                buffKindProto(buff.Status),
			Category:            buffCategoryProto(buff.Category),
			RemainingFrames:     uint32(clampNonNegativeInt(buff.RemainingFrames)),
			TickIntervalFrames:  uint32(clampNonNegativeInt(buff.TickIntervalFrames)),
			TickFramesRemaining: uint32(clampNonNegativeInt(buff.TickFramesRemaining)),
			DamagePerTick:       int32(buff.DamagePerTick * stacks),
			MoveSpeedMultiplier: float32(buff.MoveSpeedMultiplier),
			Stacks:              uint32(clampNonNegativeInt(stacks)),
			MaxStacks:           uint32(clampNonNegativeInt(maxInt(buff.MaxStacks, stacks))),
		})
	}
	return result
}

func buffKindProto(status skillcfg.StatusKind) netproto.BuffKind {
	switch status {
	case skillcfg.StatusKindPoison:
		return netproto.BuffKind_BUFF_KIND_POISON
	case skillcfg.StatusKindChill:
		return netproto.BuffKind_BUFF_KIND_CHILL
	default:
		return netproto.BuffKind_BUFF_KIND_UNKNOWN
	}
}

func buffCategoryProto(category skillcfg.BuffCategory) netproto.BuffCategory {
	switch category {
	case skillcfg.BuffCategoryDot:
		return netproto.BuffCategory_BUFF_CATEGORY_DOT
	case skillcfg.BuffCategoryControl:
		return netproto.BuffCategory_BUFF_CATEGORY_CONTROL
	default:
		return netproto.BuffCategory_BUFF_CATEGORY_UNKNOWN
	}
}

func clampNonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func (r *Runtime) exportHordeStatusLocked() *netproto.HordeStatus {
	horde := r.getHordeStateLocked()
	if horde == nil {
		return nil
	}
	return &netproto.HordeStatus{
		Value:           horde.Value,
		Threshold:       horde.Threshold,
		Active:          horde.Active,
		RemainingFrames: uint32(clampNonNegativeInt(horde.RemainingFrames)),
	}
}
