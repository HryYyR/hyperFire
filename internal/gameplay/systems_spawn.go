package gameplay

import (
	"math"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updateSpawnLocked() {
	timer := r.getSpawnTimerLocked()
	timer.Frames--
	if timer.Frames > 0 {
		return
	}

	if r.countAliveEnemiesLocked() < r.cfg.MaxEnemies {
		r.spawnEnemyLocked()
	}
	timer.Frames = r.currentSpawnIntervalFramesLocked()
}

func (r *Runtime) spawnEnemyLocked() {
	enemyClass := EnemyClassGunner
	if r.rng.Intn(2) == 0 {
		enemyClass = EnemyClassBlade
	}
	enemyTier := r.rollEnemyTierLocked()

	spawnPos, ok := r.findEnemySpawnPositionLocked()
	if !ok {
		return
	}

	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.knockback,
		r.ids.knockbackResistance,
		r.ids.enemyClass,
		r.ids.enemyTier,
		r.ids.enemyLevel,
		r.ids.enemySpawnState,
		r.ids.aggroTargetPlayerID,
		r.ids.aggroWatchState,
		r.ids.enemyMoveState,
		r.ids.enemyLifecycle,
		r.ids.rollStats,
		r.ids.rollState,
		r.ids.rollLock,
		r.ids.skillInventory,
		r.ids.activeBuffs,
		r.ids.health,
		r.ids.maxHealth,
		r.ids.radius,
		r.ids.dropValue,
		r.ids.fireCooldown,
		r.ids.networkID,
		r.ids.lastHitByPlayerID,
		r.ids.enemyTag,
	)

	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	knockback := (*Knockback)(r.world.Get(entity, r.ids.knockback))
	knockbackResistance := (*KnockbackResistance)(r.world.Get(entity, r.ids.knockbackResistance))
	class := (*EnemyClass)(r.world.Get(entity, r.ids.enemyClass))
	tier := (*EnemyTier)(r.world.Get(entity, r.ids.enemyTier))
	level := (*EnemyLevel)(r.world.Get(entity, r.ids.enemyLevel))
	spawnState := (*EnemySpawnState)(r.world.Get(entity, r.ids.enemySpawnState))
	aggroTarget := (*AggroTargetPlayerID)(r.world.Get(entity, r.ids.aggroTargetPlayerID))
	aggroWatch := (*AggroWatchState)(r.world.Get(entity, r.ids.aggroWatchState))
	moveState := (*EnemyMoveState)(r.world.Get(entity, r.ids.enemyMoveState))
	lifecycle := (*EnemyLifecycle)(r.world.Get(entity, r.ids.enemyLifecycle))
	rollStats := (*RollStats)(r.world.Get(entity, r.ids.rollStats))
	rollState := (*RollState)(r.world.Get(entity, r.ids.rollState))
	rollLock := (*RollLock)(r.world.Get(entity, r.ids.rollLock))
	skillInventory := (*SkillInventory)(r.world.Get(entity, r.ids.skillInventory))
	activeBuffs := (*ActiveBuffs)(r.world.Get(entity, r.ids.activeBuffs))
	health := (*Health)(r.world.Get(entity, r.ids.health))
	maxHealth := (*MaxHealth)(r.world.Get(entity, r.ids.maxHealth))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	dropValue := (*DropValue)(r.world.Get(entity, r.ids.dropValue))
	fireCooldown := (*FireCooldown)(r.world.Get(entity, r.ids.fireCooldown))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))
	lastHitByPlayerID := (*LastHitByPlayerID)(r.world.Get(entity, r.ids.lastHitByPlayerID))

	pos.X = spawnPos.X
	pos.Y = spawnPos.Y
	vel.X = 0
	vel.Y = 0
	knockback.X = 0
	knockback.Y = 0
	knockback.Frames = 0
	class.Value = enemyClass
	tier.Value = enemyTier
	level.Value = r.rollEnemyLevelLocked(enemyTier)
	spawnState.TotalFrames = r.enemySpawnDelayFramesLocked(enemyTier)
	spawnState.RemainingFrames = spawnState.TotalFrames
	knockbackResistance.Value = r.enemyKnockbackResistanceLocked(enemyClass)
	aggroTarget.Value = 0
	clearAggroWatchState(aggroWatch)
	r.resetEnemyMoveStateLocked(moveState)
	lifecycle.AgeFrames = 0
	lifecycle.HasTouchedPlayer = false
	*rollStats = r.enemyRollStatsLocked(enemyClass)
	resetRollState(rollState, rollStats)
	rollLock.Frames = 0
	clearSkillInventory(skillInventory)
	clearActiveBuffs(activeBuffs)
	r.assignEnemySpawnSkillsLocked(entity, enemyClass, enemyTier, level.Value)
	if r.rng.Intn(2) == 0 {
		health.Value = r.enemyHealthByLevelLocked(enemyTier, level.Value, r.cfg.EnemyHighHealth)
		dropValue.Value = r.cfg.EnemyExpDropHigh
	} else {
		health.Value = r.enemyHealthByLevelLocked(enemyTier, level.Value, r.cfg.EnemyLowHealth)
		dropValue.Value = r.cfg.EnemyExpDropLow
	}
	maxHealth.Value = health.Value
	radius.Value = r.cfg.EnemyRadius
	fireCooldown.Frames = r.enemyAttackCooldownFramesLocked(enemyTier, enemyClass, level.Value)
	networkID.Value = r.allocNetIDLocked()
	lastHitByPlayerID.Value = 0
	r.onEnemySpawnedLocked(enemyTier)
}

func (r *Runtime) rollEnemyTierLocked() uint8 {
	horde := r.getHordeStateLocked()
	if horde.Active && horde.GuaranteedBossPending {
		return EnemyTierBoss
	}

	minionWeight := r.cfg.EnemyMinionSpawnWeight
	eliteWeight := r.cfg.EnemyEliteSpawnWeight
	bossWeight := 0
	if horde.Active {
		bossWeight = r.cfg.HordeBossSpawnWeight
	}

	totalWeight := minionWeight + eliteWeight + bossWeight
	if totalWeight <= 0 {
		return EnemyTierMinion
	}

	roll := r.rng.Intn(totalWeight)
	if roll < minionWeight {
		return EnemyTierMinion
	}
	roll -= minionWeight
	if roll < eliteWeight {
		return EnemyTierElite
	}
	return EnemyTierBoss
}

func (r *Runtime) rollEnemyLevelLocked(tier uint8) uint32 {
	level := r.enemyTierMinLevelLocked(tier)
	for level < r.cfg.EnemyMaxLevel && r.rng.Float64() < r.cfg.EnemyBonusLevelChance {
		level++
	}
	return level
}

func (r *Runtime) enemyTierMinLevelLocked(tier uint8) uint32 {
	switch tier {
	case EnemyTierElite:
		return r.cfg.EnemyEliteMinLevel
	case EnemyTierBoss:
		return r.cfg.EnemyBossMinLevel
	case EnemyTierMinion:
		fallthrough
	default:
		return r.cfg.EnemyMinionMinLevel
	}
}

func (r *Runtime) enemySpawnDelayFramesLocked(tier uint8) int {
	switch tier {
	case EnemyTierElite:
		return r.params.EnemyEliteSpawnDelayFrames
	case EnemyTierBoss:
		return r.params.EnemyBossSpawnDelayFrames
	case EnemyTierMinion:
		fallthrough
	default:
		return r.params.EnemyMinionSpawnDelayFrames
	}
}

func (r *Runtime) advanceEnemySpawnStatesLocked() {
	filter := generic.NewFilter1[EnemySpawnState]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		spawnState := query.Get()
		if spawnState.RemainingFrames <= 0 {
			continue
		}
		spawnState.RemainingFrames--
		if spawnState.RemainingFrames < 0 {
			spawnState.RemainingFrames = 0
		}
	}
}

func (r *Runtime) currentSpawnIntervalFramesLocked() int {
	horde := r.getHordeStateLocked()
	if horde.Active {
		return r.currentHordeSpawnIntervalFramesLocked(horde)
	}
	return maxInt(1, r.params.SpawnTimerFrames)
}

func (r *Runtime) currentHordeSpawnIntervalFramesLocked(horde *HordeState) int {
	startFrames := maxInt(1, r.params.HordeSpawnTimerFrames)
	peakFrames := maxInt(1, r.params.HordePeakSpawnTimerFrames)
	if horde == nil || !horde.Active {
		return startFrames
	}
	if horde.TotalFrames <= 0 || peakFrames >= startFrames {
		return peakFrames
	}

	progress := 1 - float64(horde.RemainingFrames)/float64(horde.TotalFrames)
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}
	if progress >= 0.5 {
		return peakFrames
	}

	ramp := progress / 0.5
	interval := float64(startFrames) + (float64(peakFrames)-float64(startFrames))*ramp
	return maxInt(1, int(math.Round(interval)))
}

func (r *Runtime) advanceHordeStateLocked() {
	horde := r.getHordeStateLocked()
	horde.Threshold = r.cfg.HordeThreshold
	if !horde.Active {
		return
	}

	if horde.RemainingFrames > 0 {
		horde.RemainingFrames--
	}
	if horde.RemainingFrames > 0 {
		return
	}

	horde.Active = false
	horde.RemainingFrames = 0
	horde.TotalFrames = 0
	horde.Value = 0
	horde.BossSpawnedThisWave = false
	horde.GuaranteedBossPending = false
}

func (r *Runtime) addHordeValueForEnemyKillLocked(tier uint8) {
	horde := r.getHordeStateLocked()
	if horde.Active {
		return
	}

	horde.Value += r.hordeValueForEnemyTierLocked(tier)
	if horde.Value < horde.Threshold {
		return
	}

	r.startHordeLocked(horde)
}

func (r *Runtime) hordeValueForEnemyTierLocked(tier uint8) int32 {
	switch tier {
	case EnemyTierElite:
		return r.cfg.HordeValuePerEliteKill
	case EnemyTierBoss:
		return r.cfg.HordeValuePerBossKill
	case EnemyTierMinion:
		fallthrough
	default:
		return r.cfg.HordeValuePerMinionKill
	}
}

func (r *Runtime) startHordeLocked(horde *HordeState) {
	if horde == nil {
		return
	}
	horde.Value = horde.Threshold
	horde.Active = true
	horde.TotalFrames = maxInt(1, r.params.HordeDurationFrames)
	horde.RemainingFrames = horde.TotalFrames
	horde.BossSpawnedThisWave = false
	horde.GuaranteedBossPending = true

	timer := r.getSpawnTimerLocked()
	if timer.Frames > 1 {
		timer.Frames = 1
	}
}

func (r *Runtime) onEnemySpawnedLocked(tier uint8) {
	if tier != EnemyTierBoss {
		return
	}
	horde := r.getHordeStateLocked()
	if !horde.Active {
		return
	}
	horde.BossSpawnedThisWave = true
	horde.GuaranteedBossPending = false
}

func (r *Runtime) assignEnemySpawnSkillsLocked(entity ecs.Entity, class uint8, tier uint8, level uint32) {
	skillCount := r.enemyStartingSkillCountLocked(level, class, tier)
	if skillCount <= 0 {
		return
	}

	for _, skillID := range r.chooseRandomEnemySkillsLocked(class, skillCount) {
		_, _ = r.tryGrantSkillLocked(entity, skillID)
	}
}

func (r *Runtime) enemyStartingSkillCountLocked(level uint32, class uint8, tier uint8) int {
	if tier == EnemyTierMinion {
		return 0
	}
	if level <= r.cfg.EnemyBaseLevel {
		if tier == EnemyTierBoss {
			return 1
		}
		return 0
	}
	count := int(level - r.cfg.EnemyBaseLevel)
	poolSize := len(r.enemySkillPoolLocked(class))
	if count > poolSize {
		count = poolSize
	}
	if tier == EnemyTierBoss && count < 1 && poolSize > 0 {
		count = 1
	}
	return count
}

func (r *Runtime) chooseRandomEnemySkillsLocked(class uint8, count int) []string {
	pool := r.enemySkillPoolLocked(class)
	if count <= 0 || len(pool) == 0 {
		return nil
	}
	if count > len(pool) {
		count = len(pool)
	}

	perm := r.rng.Perm(len(pool))
	result := make([]string, 0, count)
	for _, index := range perm[:count] {
		result = append(result, pool[index])
	}
	return result
}

func (r *Runtime) enemySkillPoolLocked(class uint8) []string {
	switch class {
	case EnemyClassBlade:
		return []string{
			"skill_toxic_rounds_1",
			"skill_frost_rounds_1",
			"skill_venom_frost_rounds",
		}
	case EnemyClassGunner:
		fallthrough
	default:
		return []string{
			"skill_toxic_rounds_1",
			"skill_frost_rounds_1",
			"skill_venom_frost_rounds",
			"skill_homing_rounds_1",
		}
	}
}

func (r *Runtime) updateEnemyCleanupLocked() {
	filter := generic.NewFilter5[Position, Health, EnemyClass, AggroTargetPlayerID, EnemyLifecycle]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	targets := r.snapshotPlayersLocked()
	var toRemove []ecs.Entity

	for query.Next() {
		pos, health, class, aggroTarget, lifecycle := query.Get()
		if health.Value <= 0 {
			continue
		}

		lifecycle.AgeFrames++
		if !lifecycle.HasTouchedPlayer && enemyHasTouchedPlayer(pos.X, pos.Y, class.Value, aggroTarget, targets, r.enemyCleanupTouchRadiusLocked(class.Value)) {
			lifecycle.HasTouchedPlayer = true
		}
		if lifecycle.AgeFrames < r.params.EnemyCleanupLifetimeFrames {
			continue
		}
		if shouldCleanupEnemy(pos.X, pos.Y, lifecycle, targets, r.cfg.EnemyCleanupDistance) {
			toRemove = append(toRemove, query.Entity())
		}
	}

	for _, entity := range toRemove {
		r.world.RemoveEntity(entity)
	}
}

func (r *Runtime) countAliveEnemiesLocked() int {
	filter := generic.NewFilter1[Health]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	count := 0
	for query.Next() {
		health := query.Get()
		if health.Value > 0 {
			count++
		}
	}
	return count
}

func (r *Runtime) findEnemySpawnPositionLocked() (Position, bool) {
	targets := r.snapshotPlayersLocked()
	if len(targets) == 0 || r.cfg.EnemySpawnMinDistance <= 0 {
		return Position{
			X: r.rng.Float64() * r.cfg.SpawnAreaWidth,
			Y: r.rng.Float64() * r.cfg.SpawnAreaHeight,
		}, true
	}

	bestPos := Position{}
	bestMinDistance := -1.0
	requiredDistance := r.cfg.EnemySpawnMinDistance

	for attempt := 0; attempt < r.cfg.EnemySpawnAttempts; attempt++ {
		candidate := Position{
			X: r.rng.Float64() * r.cfg.SpawnAreaWidth,
			Y: r.rng.Float64() * r.cfg.SpawnAreaHeight,
		}
		minDistance := minDistanceToTargets(candidate.X, candidate.Y, targets)
		if minDistance >= requiredDistance {
			return candidate, true
		}
		if minDistance > bestMinDistance {
			bestMinDistance = minDistance
			bestPos = candidate
		}
	}

	if bestMinDistance >= requiredDistance {
		return bestPos, true
	}
	return Position{}, false
}

func (r *Runtime) enemyCleanupTouchRadiusLocked(class uint8) float64 {
	return r.enemyAlertRadiusLocked(class)
}

func enemyHasTouchedPlayer(fromX, fromY float64, class uint8, aggroTarget *AggroTargetPlayerID, targets []TargetSnapshot, touchRadius float64) bool {
	if aggroTarget != nil && aggroTarget.Value != 0 {
		return true
	}
	if touchRadius <= 0 {
		return false
	}
	_, found := findNearestTargetInRadius(fromX, fromY, targets, touchRadius)
	return found
}

func shouldCleanupEnemy(fromX, fromY float64, lifecycle *EnemyLifecycle, targets []TargetSnapshot, cleanupDistance float64) bool {
	if lifecycle == nil {
		return false
	}
	if !lifecycle.HasTouchedPlayer {
		return true
	}
	if len(targets) == 0 {
		return true
	}
	if cleanupDistance <= 0 {
		return false
	}
	return minDistanceToTargets(fromX, fromY, targets) > cleanupDistance
}

func minDistanceToTargets(fromX, fromY float64, targets []TargetSnapshot) float64 {
	if len(targets) == 0 {
		return math.MaxFloat64
	}

	best := math.MaxFloat64
	for _, target := range targets {
		distance := math.Hypot(target.X-fromX, target.Y-fromY)
		if distance < best {
			best = distance
		}
	}
	return best
}
