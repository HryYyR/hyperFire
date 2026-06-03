package gameplay

import (
	"math"

	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updatePlayerControlLocked() {
	filter := generic.NewFilter3[Velocity, OwnerPlayerID, Health]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		vel, owner, health := query.Get()
		entity := query.Entity()
		if health.Value <= 0 {
			vel.X = 0
			vel.Y = 0
			continue
		}

		input := r.latestInputs[owner.Value]
		moveSpeed := r.params.PlayerMoveSpeed * r.entityMoveSpeedMultiplierLocked(entity)
		vel.X = float64(clampMoveAxis(input.MoveX)) * moveSpeed
		vel.Y = float64(clampMoveAxis(input.MoveY)) * moveSpeed
	}
}

func (r *Runtime) updateEnemyAggroLocked() {
	targets := r.snapshotPlayersLocked()
	filter := generic.NewFilter5[Position, Health, EnemyClass, AggroTargetPlayerID, AggroWatchState]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)

	for query.Next() {
		pos, health, class, aggroTarget, aggroWatch := query.Get()
		if r.enemyIsSpawningLocked(query.Entity()) {
			aggroTarget.Value = 0
			clearAggroWatchState(aggroWatch)
			continue
		}
		if health.Value <= 0 {
			aggroTarget.Value = 0
			clearAggroWatchState(aggroWatch)
			continue
		}

		if aggroTarget.Value != 0 {
			target, found := r.findPlayerTargetByIDLocked(aggroTarget.Value)
			if found && r.enemyShouldKeepAggroLocked(pos.X, pos.Y, class.Value, target.X, target.Y) {
				clearAggroWatchState(aggroWatch)
				continue
			}
			aggroTarget.Value = 0
		}

		target, found := findNearestTargetInRadius(pos.X, pos.Y, targets, r.enemyAggroRadiusLocked(class.Value))
		if found && target.PlayerID != 0 {
			aggroTarget.Value = target.PlayerID
			clearAggroWatchState(aggroWatch)
			continue
		}

		target, found = findNearestTargetInRadius(pos.X, pos.Y, targets, r.enemyAlertRadiusLocked(class.Value))
		if !found || target.PlayerID == 0 {
			clearAggroWatchState(aggroWatch)
			continue
		}

		if aggroWatch.CandidatePlayerID != target.PlayerID {
			aggroWatch.CandidatePlayerID = target.PlayerID
			aggroWatch.Frames = 1
		} else {
			aggroWatch.Frames++
		}
		if aggroWatch.Frames >= r.params.EnemyAlertLockDelayFrames {
			aggroTarget.Value = target.PlayerID
			clearAggroWatchState(aggroWatch)
		}
	}
}

func (r *Runtime) updateEnemyMovementLocked() {
	targets := r.snapshotPlayersLocked()
	skillDropTargets := r.snapshotSkillDropTargetsLocked()
	filter := generic.NewFilter5[Position, Velocity, EnemyClass, AggroTargetPlayerID, EnemyMoveState]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		pos, vel, class, aggroTarget, moveState := query.Get()
		entity := query.Entity()
		if r.enemyIsSpawningLocked(entity) {
			vel.X = 0
			vel.Y = 0
			continue
		}
		target, found := r.selectEnemyChaseTargetLocked(entity, pos.X, pos.Y, class.Value, aggroTarget, targets)
		if found {
			vel.X, vel.Y = r.enemyCombatVelocityLocked(pos.X, pos.Y, target.X, target.Y, class.Value, moveState)
		} else if r.enemyCanUseSkillDropsLocked(entity) {
			if skillDropTarget, pickupFound := r.selectEnemySkillPickupTargetLocked(pos.X, pos.Y, skillDropTargets); pickupFound {
				vel.X, vel.Y = r.enemyPickupSeekVelocityLocked(pos.X, pos.Y, skillDropTarget.X, skillDropTarget.Y, class.Value)
			} else {
				vel.X, vel.Y = r.enemyPatrolVelocityLocked(class.Value, moveState)
			}
		} else {
			vel.X, vel.Y = r.enemyPatrolVelocityLocked(class.Value, moveState)
		}
		multiplier := r.entityMoveSpeedMultiplierLocked(entity)
		vel.X *= multiplier
		vel.Y *= multiplier
	}
}

func (r *Runtime) updatePlayerFireLocked() {
	filter := generic.NewFilter4[Position, FireCooldown, OwnerPlayerID, Health]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var toSpawn []BulletSpawn

	for query.Next() {
		pos, fireCooldown, owner, health := query.Get()
		sourceEntity := query.Entity()
		if health.Value <= 0 {
			continue
		}

		input := r.latestInputs[owner.Value]
		if fireCooldown.Frames > 0 {
			fireCooldown.Frames--
		}
		if !input.Fire || fireCooldown.Frames > 0 {
			continue
		}

		velX, velY, ok := calcAimVelocity(
			pos.X, pos.Y,
			float64(pos.X)+float64(input.AimDX),
			float64(pos.Y)+float64(input.AimDY),
			r.params.PlayerBulletSpeed,
		)
		if !ok {
			continue
		}

		onHitEffects := cloneEffectConfigs(r.collectOnHitEffectsLocked(sourceEntity))
		toSpawn = append(toSpawn, BulletSpawn{
			Position:      Position{X: pos.X, Y: pos.Y},
			Velocity:      Velocity{X: velX, Y: velY},
			OwnerPlayerID: owner.Value,
			OnHitEffects:  cloneEffectConfigs(onHitEffects),
		})
		toSpawn = append(toSpawn, r.collectPlayerBonusProjectilesLocked(sourceEntity, pos.X, pos.Y, owner.Value, velX, velY, onHitEffects)...)
		fireCooldown.Frames = r.params.PlayerFireCooldownFrames
	}

	for _, bullet := range toSpawn {
		r.spawnPlayerBulletLocked(bullet)
	}
}

func (r *Runtime) updateEnemyFireLocked() {
	targets := r.snapshotPlayersLocked()
	if len(targets) == 0 {
		return
	}

	filter := generic.NewFilter6[Position, FireCooldown, EnemyClass, EnemyTier, EnemyLevel, AggroTargetPlayerID]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	var toSpawn []BulletSpawn

	for query.Next() {
		pos, fireCooldown, class, tier, level, aggroTarget := query.Get()
		sourceEntity := query.Entity()
		if r.enemyIsSpawningLocked(sourceEntity) {
			continue
		}
		if class.Value != EnemyClassGunner {
			continue
		}
		if fireCooldown.Frames > 0 {
			fireCooldown.Frames--
		}
		if fireCooldown.Frames > 0 {
			continue
		}

		target, found := r.selectEnemyChaseTargetLocked(sourceEntity, pos.X, pos.Y, class.Value, aggroTarget, targets)
		if !found {
			continue
		}
		if !r.enemyCanAttackTargetLocked(pos.X, pos.Y, class.Value, target.X, target.Y) {
			continue
		}

		velX, velY, ok := calcAimVelocity(pos.X, pos.Y, target.X, target.Y, r.params.EnemyBulletSpeed)
		if !ok {
			continue
		}

		toSpawn = append(toSpawn, BulletSpawn{
			Position:       Position{X: pos.X, Y: pos.Y},
			Velocity:       Velocity{X: velX, Y: velY},
			DamageOverride: r.enemyBulletDamageByLevelLocked(tier.Value, level.Value),
			OnHitEffects:   cloneEffectConfigs(r.collectOnHitEffectsLocked(sourceEntity)),
		})
		fireCooldown.Frames = r.enemyAttackCooldownFramesLocked(tier.Value, class.Value, level.Value)
	}

	for _, bullet := range toSpawn {
		r.spawnEnemyBulletLocked(bullet)
	}
}

func (r *Runtime) updateEnemyMeleeLocked() {
	targets := r.snapshotLivePlayerHitTargetsLocked()
	if len(targets) == 0 {
		return
	}

	filter := generic.NewFilter6[Position, FireCooldown, EnemyClass, EnemyTier, EnemyLevel, AggroTargetPlayerID]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	var hitEvents []HitEvent

	for query.Next() {
		pos, fireCooldown, class, tier, level, aggroTarget := query.Get()
		sourceEntity := query.Entity()
		if r.enemyIsSpawningLocked(sourceEntity) {
			continue
		}
		if class.Value != EnemyClassBlade {
			continue
		}
		if fireCooldown.Frames > 0 {
			fireCooldown.Frames--
		}
		if fireCooldown.Frames > 0 {
			continue
		}

		hitAny := false
		preferredPlayerID := aggroTarget.Value
		for _, target := range targets {
			if preferredPlayerID != 0 && target.OwnerPlayerID != preferredPlayerID {
				continue
			}
			if !judgeHit(pos.X, pos.Y, target.X, target.Y, r.cfg.EnemyBladeAttackRadius, target.Radius) {
				continue
			}
			hitEvents = append(hitEvents, buildEnemyMeleeHitEvent(
				pos.X,
				pos.Y,
				target,
				r.enemyBladeDamageByLevelLocked(tier.Value, level.Value),
				r.params.EnemyBladeKnockbackPerTick,
				r.collectOnHitEffectsLocked(sourceEntity),
			))
			hitAny = true
		}
		if hitAny {
			fireCooldown.Frames = r.enemyAttackCooldownFramesLocked(tier.Value, class.Value, level.Value)
		}
	}

	r.applyHitEventsLocked(hitEvents)
}

func (r *Runtime) spawnPlayerBulletLocked(spawn BulletSpawn) {
	components := []ecs.ID{
		r.ids.position,
		r.ids.velocity,
		r.ids.prevPosition,
		r.ids.lifetime,
		r.ids.damage,
		r.ids.knockbackForce,
		r.ids.attackEffects,
		r.ids.radius,
		r.ids.networkID,
		r.ids.ownerPlayerID,
		r.ids.bulletTag,
		r.ids.playerBulletTag,
	}
	if spawn.Homing != nil {
		components = append(components, r.ids.homingProjectile)
	}
	entity := r.world.NewEntity(components...)

	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	prevPos := (*PreviousPosition)(r.world.Get(entity, r.ids.prevPosition))
	lifeTime := (*Lifetime)(r.world.Get(entity, r.ids.lifetime))
	damage := (*Damage)(r.world.Get(entity, r.ids.damage))
	knockbackForce := (*KnockbackForce)(r.world.Get(entity, r.ids.knockbackForce))
	attackEffects := (*AttackEffects)(r.world.Get(entity, r.ids.attackEffects))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))
	ownerPlayerID := (*OwnerPlayerID)(r.world.Get(entity, r.ids.ownerPlayerID))
	var homing *HomingProjectile
	if spawn.Homing != nil {
		homing = (*HomingProjectile)(r.world.Get(entity, r.ids.homingProjectile))
	}

	*pos = spawn.Position
	*vel = spawn.Velocity
	prevPos.X = spawn.Position.X
	prevPos.Y = spawn.Position.Y
	lifeTime.Frames = r.params.InitialBulletLifeFrames
	damage.Value = r.cfg.BulletDamage
	knockbackForce.Value = r.params.BulletKnockbackPerTick
	attackEffects.OnHit = cloneEffectConfigs(spawn.OnHitEffects)
	radius.Value = r.cfg.BulletRadius
	networkID.Value = r.allocNetIDLocked()
	ownerPlayerID.Value = spawn.OwnerPlayerID
	if homing != nil {
		homing.SearchRadius = spawn.Homing.SearchRadius
		homing.Speed = spawn.Homing.Speed
		homing.Target = ecs.Entity{}
	}
}

func (r *Runtime) spawnEnemyBulletLocked(spawn BulletSpawn) {
	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.prevPosition,
		r.ids.lifetime,
		r.ids.damage,
		r.ids.knockbackForce,
		r.ids.attackEffects,
		r.ids.radius,
		r.ids.networkID,
		r.ids.bulletTag,
		r.ids.enemyBulletTag,
	)

	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	prevPos := (*PreviousPosition)(r.world.Get(entity, r.ids.prevPosition))
	lifeTime := (*Lifetime)(r.world.Get(entity, r.ids.lifetime))
	damage := (*Damage)(r.world.Get(entity, r.ids.damage))
	knockbackForce := (*KnockbackForce)(r.world.Get(entity, r.ids.knockbackForce))
	attackEffects := (*AttackEffects)(r.world.Get(entity, r.ids.attackEffects))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))

	*pos = spawn.Position
	*vel = spawn.Velocity
	prevPos.X = spawn.Position.X
	prevPos.Y = spawn.Position.Y
	lifeTime.Frames = r.params.InitialBulletLifeFrames
	damage.Value = r.cfg.BulletDamage
	if spawn.DamageOverride > 0 {
		damage.Value = spawn.DamageOverride
	}
	knockbackForce.Value = r.params.BulletKnockbackPerTick
	attackEffects.OnHit = cloneEffectConfigs(spawn.OnHitEffects)
	radius.Value = r.cfg.BulletRadius
	networkID.Value = r.allocNetIDLocked()
}

func (r *Runtime) enemyAggroRadiusLocked(class uint8) float64 {
	scale := r.enemyHordeAggroScaleLocked()
	switch class {
	case EnemyClassBlade:
		return r.cfg.EnemyBladeAggroRadius * scale
	case EnemyClassGunner:
		return r.cfg.EnemyGunnerAggroRadius * scale
	default:
		return r.cfg.EnemyGunnerAggroRadius * scale
	}
}

func (r *Runtime) enemyAlertRadiusLocked(class uint8) float64 {
	scale := r.enemyHordeAlertScaleLocked()
	switch class {
	case EnemyClassBlade:
		return r.cfg.EnemyBladeAlertRadius * scale
	case EnemyClassGunner:
		return r.cfg.EnemyGunnerAlertRadius * scale
	default:
		return r.cfg.EnemyGunnerAlertRadius * scale
	}
}

func (r *Runtime) enemyHordeAggroScaleLocked() float64 {
	horde := r.getHordeStateLocked()
	if horde.Active {
		return r.cfg.HordeAggroRadiusScale
	}
	return 1
}

func (r *Runtime) enemyHordeAlertScaleLocked() float64 {
	horde := r.getHordeStateLocked()
	if horde.Active {
		return r.cfg.HordeAlertRadiusScale
	}
	return 1
}

func (r *Runtime) enemyMoveSpeedLocked(class uint8) float64 {
	switch class {
	case EnemyClassBlade:
		return r.params.EnemyBladeMoveSpeed
	case EnemyClassGunner:
		return r.params.EnemyGunnerMoveSpeed
	default:
		return r.params.EnemyGunnerMoveSpeed
	}
}

func (r *Runtime) enemyKnockbackResistanceLocked(class uint8) float64 {
	switch class {
	case EnemyClassBlade:
		return r.cfg.EnemyBladeKnockbackResistance
	case EnemyClassGunner:
		return r.cfg.EnemyGunnerKnockbackResistance
	default:
		return r.cfg.EnemyGunnerKnockbackResistance
	}
}

func (r *Runtime) resetEnemyMoveStateLocked(state *EnemyMoveState) {
	if state == nil {
		return
	}
	state.PatrolWaitFrames = r.randomPatrolWaitFramesLocked()
	state.PatrolMoveFrames = 0
	state.PatrolDirX = 0
	state.PatrolDirY = 0
	state.StrafeSign = r.randomSteerSignLocked()
	state.ArcSign = r.randomSteerSignLocked()
}

func (r *Runtime) enemyPatrolVelocityLocked(class uint8, state *EnemyMoveState) (float64, float64) {
	if state == nil {
		return 0, 0
	}

	speed := r.enemyMoveSpeedLocked(class) * r.cfg.EnemyPatrolSpeedScale
	if speed <= 0 {
		return 0, 0
	}

	if state.PatrolMoveFrames > 0 {
		state.PatrolMoveFrames--
		if state.PatrolMoveFrames == 0 {
			state.PatrolWaitFrames = r.params.EnemyPatrolWaitFrames
			state.PatrolDirX = 0
			state.PatrolDirY = 0
			return 0, 0
		}
		return state.PatrolDirX * speed, state.PatrolDirY * speed
	}

	if state.PatrolWaitFrames > 0 {
		state.PatrolWaitFrames--
		return 0, 0
	}

	// Patrol state lives on the entity so the motion remains stable across ticks
	// and can later compose with extra behaviors like dodge/roll.
	state.PatrolDirX, state.PatrolDirY = r.randomUnitVectorLocked()
	state.PatrolMoveFrames = r.params.EnemyPatrolMoveFrames
	if state.PatrolMoveFrames <= 0 {
		state.PatrolMoveFrames = 1
	}
	return state.PatrolDirX * speed, state.PatrolDirY * speed
}

func (r *Runtime) enemyCombatVelocityLocked(fromX, fromY, toX, toY float64, class uint8, state *EnemyMoveState) (float64, float64) {
	switch class {
	case EnemyClassGunner:
		return r.enemyGunnerStrafeVelocityLocked(fromX, fromY, toX, toY, state)
	case EnemyClassBlade:
		return r.enemyBladeArcVelocityLocked(fromX, fromY, toX, toY, state)
	default:
		velX, velY, ok := calcAimVelocity(fromX, fromY, toX, toY, r.enemyMoveSpeedLocked(class))
		if !ok {
			return 0, 0
		}
		return velX, velY
	}
}

func (r *Runtime) enemyGunnerStrafeVelocityLocked(fromX, fromY, toX, toY float64, state *EnemyMoveState) (float64, float64) {
	dirX, dirY, ok := normalizeVector(toX-fromX, toY-fromY)
	if !ok {
		return 0, 0
	}
	distance := math.Hypot(toX-fromX, toY-fromY)
	if distance > r.enemyGunnerPreferredRangeLocked() {
		velX, velY, ok := calcAimVelocity(fromX, fromY, toX, toY, r.enemyMoveSpeedLocked(EnemyClassGunner))
		if !ok {
			return 0, 0
		}
		return velX, velY
	}

	sign := r.ensureSteerSignLocked(state, true)
	perpX, perpY := perpendicularLeft(dirX, dirY)
	speed := r.enemyMoveSpeedLocked(EnemyClassGunner)
	return perpX * speed * sign, perpY * speed * sign
}

func (r *Runtime) enemyBladeArcVelocityLocked(fromX, fromY, toX, toY float64, state *EnemyMoveState) (float64, float64) {
	dirX, dirY, ok := normalizeVector(toX-fromX, toY-fromY)
	if !ok {
		return 0, 0
	}

	sign := r.ensureSteerSignLocked(state, false)
	perpX, perpY := perpendicularLeft(dirX, dirY)
	arcStrength := r.cfg.EnemyBladeArcStrength
	distance := math.Hypot(toX-fromX, toY-fromY)
	if distance <= r.cfg.EnemyBladeAttackRadius*2 {
		// Keep some curvature so movement feels alive, but fade it near melee range
		// to avoid orbiting around the target forever.
		arcStrength *= 0.25
	}

	moveX, moveY, ok := normalizeVector(dirX+perpX*arcStrength*sign, dirY+perpY*arcStrength*sign)
	if !ok {
		return 0, 0
	}

	speed := r.enemyMoveSpeedLocked(EnemyClassBlade)
	return moveX * speed, moveY * speed
}

func (r *Runtime) ensureSteerSignLocked(state *EnemyMoveState, strafe bool) float64 {
	if state == nil {
		return 1
	}
	if strafe {
		if state.StrafeSign == 0 {
			state.StrafeSign = r.randomSteerSignLocked()
		}
		return state.StrafeSign
	}
	if state.ArcSign == 0 {
		state.ArcSign = r.randomSteerSignLocked()
	}
	return state.ArcSign
}

func (r *Runtime) randomPatrolWaitFramesLocked() int {
	if r.params.EnemyPatrolWaitFrames <= 1 {
		return r.params.EnemyPatrolWaitFrames
	}
	return r.rng.Intn(r.params.EnemyPatrolWaitFrames)
}

func (r *Runtime) randomSteerSignLocked() float64 {
	if r.rng.Intn(2) == 0 {
		return -1
	}
	return 1
}

func (r *Runtime) randomUnitVectorLocked() (float64, float64) {
	angle := r.rng.Float64() * 2 * math.Pi
	return math.Cos(angle), math.Sin(angle)
}

func (r *Runtime) selectEnemySkillPickupTargetLocked(fromX, fromY float64, targets []TargetSnapshot) (TargetSnapshot, bool) {
	if len(targets) == 0 || r.cfg.EnemySkillPickupSeekRadius <= 0 {
		return TargetSnapshot{}, false
	}
	return findNearestTargetInRadius(fromX, fromY, targets, r.cfg.EnemySkillPickupSeekRadius)
}

func (r *Runtime) enemyPickupSeekVelocityLocked(fromX, fromY, toX, toY float64, class uint8) (float64, float64) {
	velX, velY, ok := calcAimVelocity(fromX, fromY, toX, toY, r.enemyMoveSpeedLocked(class))
	if !ok {
		return 0, 0
	}
	return velX, velY
}

func buildEnemyMeleeHitEvent(fromX, fromY float64, target HitTargetSnapshot, damage int, knockback float64, onHitEffects []skillcfg.EffectConfig) HitEvent {
	knockbackX, knockbackY, ok := calcAimVelocity(fromX, fromY, target.X, target.Y, knockback)
	if !ok {
		knockbackX = 0
		knockbackY = 0
	}
	return HitEvent{
		Bullet:       ecs.Entity{},
		Target:       target.Entity,
		Damage:       damage,
		KnockbackX:   knockbackX,
		KnockbackY:   knockbackY,
		OnHitEffects: cloneEffectConfigs(onHitEffects),
	}
}

func (r *Runtime) collectPlayerBonusProjectilesLocked(source ecs.Entity, posX, posY float64, ownerPlayerID uint32, velX, velY float64, onHitEffects []skillcfg.EffectConfig) []BulletSpawn {
	triggeredEffects := r.collectShotTriggeredEffectsLocked(source)
	if len(triggeredEffects) == 0 {
		return nil
	}

	dirX, dirY, ok := normalizeVector(velX, velY)
	if !ok {
		return nil
	}

	var spawns []BulletSpawn
	for _, effect := range triggeredEffects {
		if effect.Kind != skillcfg.EffectKindBonusHomingShotOnFire || effect.BonusHomingShotOnFire == nil {
			continue
		}

		speed := r.params.PlayerBulletSpeed * effect.BonusHomingShotOnFire.BulletSpeedScale
		spawns = append(spawns, BulletSpawn{
			Position:      Position{X: posX, Y: posY},
			Velocity:      Velocity{X: dirX * speed, Y: dirY * speed},
			OwnerPlayerID: ownerPlayerID,
			OnHitEffects:  cloneEffectConfigs(onHitEffects),
			Homing: &HomingProjectile{
				SearchRadius: effect.BonusHomingShotOnFire.SearchRadius,
				Speed:        speed,
			},
		})
	}
	return spawns
}

func (r *Runtime) selectEnemyChaseTargetLocked(entity ecs.Entity, fromX, fromY float64, class uint8, aggroTarget *AggroTargetPlayerID, targets []TargetSnapshot) (TargetSnapshot, bool) {
	if aggroTarget == nil || aggroTarget.Value == 0 {
		target, found := findNearestTargetInRadius(fromX, fromY, targets, r.enemyAggroRadiusLocked(class))
		if !found || target.PlayerID == 0 {
			return TargetSnapshot{}, false
		}
		aggroTarget.Value = target.PlayerID
		if !entity.IsZero() && r.world.Has(entity, r.ids.aggroWatchState) {
			aggroWatch := (*AggroWatchState)(r.world.Get(entity, r.ids.aggroWatchState))
			clearAggroWatchState(aggroWatch)
		}
		return target, true
	}
	target, found := r.findPlayerTargetByIDLocked(aggroTarget.Value)
	if found && r.enemyShouldKeepAggroLocked(fromX, fromY, class, target.X, target.Y) {
		return target, true
	}
	aggroTarget.Value = 0
	return TargetSnapshot{}, false
}

func (r *Runtime) enemyShouldKeepAggroLocked(fromX, fromY float64, class uint8, targetX, targetY float64) bool {
	if class == EnemyClassBlade {
		return true
	}
	return math.Hypot(targetX-fromX, targetY-fromY) <= r.enemyAggroLeashRadiusLocked(class)
}

func (r *Runtime) enemyAggroLeashRadiusLocked(class uint8) float64 {
	return r.enemyAggroRadiusLocked(class) * r.cfg.EnemyAggroLeashScale
}

func (r *Runtime) enemyGunnerPreferredRangeLocked() float64 {
	return r.cfg.EnemyGunnerAggroRadius * r.cfg.EnemyGunnerPreferredRangeScale
}

func (r *Runtime) enemyCanAttackTargetLocked(fromX, fromY float64, class uint8, targetX, targetY float64) bool {
	switch class {
	case EnemyClassGunner:
		return math.Hypot(targetX-fromX, targetY-fromY) <= r.enemyAggroRadiusLocked(class)
	case EnemyClassBlade:
		return math.Hypot(targetX-fromX, targetY-fromY) <= r.cfg.EnemyBladeAttackRadius
	default:
		return false
	}
}

func (r *Runtime) findPlayerTargetByIDLocked(playerID uint32) (TargetSnapshot, bool) {
	entity, ok := r.playerEntities[playerID]
	if !ok || !r.world.Alive(entity) {
		return TargetSnapshot{}, false
	}
	health := (*Health)(r.world.Get(entity, r.ids.health))
	if health.Value <= 0 {
		return TargetSnapshot{}, false
	}
	pos := (*Position)(r.world.Get(entity, r.ids.position))
	return TargetSnapshot{
		Entity:   entity,
		PlayerID: playerID,
		X:        pos.X,
		Y:        pos.Y,
	}, true
}

func clearAggroWatchState(state *AggroWatchState) {
	if state == nil {
		return
	}
	state.CandidatePlayerID = 0
	state.Frames = 0
}

func (r *Runtime) enemyIsSpawningLocked(entity ecs.Entity) bool {
	if entity.IsZero() || !r.world.Alive(entity) || !r.world.Has(entity, r.ids.enemySpawnState) {
		return false
	}
	spawnState := (*EnemySpawnState)(r.world.Get(entity, r.ids.enemySpawnState))
	return spawnState.RemainingFrames > 0
}
