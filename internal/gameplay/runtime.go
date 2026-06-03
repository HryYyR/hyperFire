package gameplay

import (
	"math/rand"
	"sync"

	"agentDemo/internal/levelcfg"
	"agentDemo/internal/netproto"
	"agentDemo/internal/session"
	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
)

type Runtime struct {
	mu sync.Mutex

	world ecs.World
	rng   *rand.Rand
	ids   componentIDs

	cfg    Config
	params runtimeParams
	skills *skillcfg.Catalog
	levels *levelcfg.Table

	tick      uint32
	nextNetID uint32

	playerEntities map[uint32]ecs.Entity
	latestInputs   map[uint32]session.InputState
	// lastImpacts only lives for the current tick and is copied into the snapshot.
	lastImpacts []*netproto.ImpactEvent
	// lastPickupEvents only lives for the current tick and is copied into the snapshot.
	lastPickupEvents []*netproto.PickupEvent
}

func NewRuntime(tickHz uint32) *Runtime {
	return NewRuntimeWithConfig(DefaultConfig(tickHz))
}

func NewRuntimeWithConfig(cfg Config) *Runtime {
	cfg = cfg.normalized()
	params := cfg.runtimeParams()
	world := ecs.NewWorld()
	runtime := &Runtime{
		world:          world,
		rng:            rand.New(rand.NewSource(42)),
		cfg:            cfg,
		params:         params,
		skills:         skillcfg.MustLoadEmbeddedCatalog(),
		levels:         levelcfg.MustLoadEmbeddedTable(),
		nextNetID:      1,
		playerEntities: make(map[uint32]ecs.Entity),
		latestInputs:   make(map[uint32]session.InputState),
	}
	runtime.initComponentIDs()
	runtime.initResources()
	return runtime
}

func (r *Runtime) AddPlayer(playerID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.playerEntities[playerID]; exists {
		return
	}

	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.spawnPoint,
		r.ids.velocity,
		r.ids.knockback,
		r.ids.knockbackResistance,
		r.ids.rollStats,
		r.ids.rollState,
		r.ids.rollLock,
		r.ids.playerLevel,
		r.ids.skillInventory,
		r.ids.pendingSkillChoices,
		r.ids.activeBuffs,
		r.ids.lifeState,
		r.ids.health,
		r.ids.maxHealth,
		r.ids.experience,
		r.ids.radius,
		r.ids.fireCooldown,
		r.ids.networkID,
		r.ids.ownerPlayerID,
		r.ids.playerTag,
	)

	index := float64(len(r.playerEntities))
	pos := (*Position)(r.world.Get(entity, r.ids.position))
	spawnPoint := (*SpawnPoint)(r.world.Get(entity, r.ids.spawnPoint))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	knockback := (*Knockback)(r.world.Get(entity, r.ids.knockback))
	knockbackResistance := (*KnockbackResistance)(r.world.Get(entity, r.ids.knockbackResistance))
	rollStats := (*RollStats)(r.world.Get(entity, r.ids.rollStats))
	rollState := (*RollState)(r.world.Get(entity, r.ids.rollState))
	rollLock := (*RollLock)(r.world.Get(entity, r.ids.rollLock))
	playerLevel := (*PlayerLevel)(r.world.Get(entity, r.ids.playerLevel))
	skillInventory := (*SkillInventory)(r.world.Get(entity, r.ids.skillInventory))
	pendingSkillChoices := (*PendingSkillChoices)(r.world.Get(entity, r.ids.pendingSkillChoices))
	activeBuffs := (*ActiveBuffs)(r.world.Get(entity, r.ids.activeBuffs))
	lifeState := (*LifeState)(r.world.Get(entity, r.ids.lifeState))
	health := (*Health)(r.world.Get(entity, r.ids.health))
	maxHealth := (*MaxHealth)(r.world.Get(entity, r.ids.maxHealth))
	experience := (*Experience)(r.world.Get(entity, r.ids.experience))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	fireCooldown := (*FireCooldown)(r.world.Get(entity, r.ids.fireCooldown))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))
	ownerPlayerID := (*OwnerPlayerID)(r.world.Get(entity, r.ids.ownerPlayerID))

	spawnPoint.X = r.cfg.SpawnBaseX + index*r.cfg.SpawnSpacingX
	spawnPoint.Y = r.cfg.SpawnBaseY
	pos.X = spawnPoint.X
	pos.Y = spawnPoint.Y
	vel.X = 0
	vel.Y = 0
	knockback.X = 0
	knockback.Y = 0
	knockback.Frames = 0
	knockbackResistance.Value = r.cfg.PlayerKnockbackResistance
	*rollStats = r.playerRollStatsLocked(entity)
	resetRollState(rollState, rollStats)
	rollLock.Frames = 0
	playerLevel.Value = r.levels.BaseLevel()
	clearSkillInventory(skillInventory)
	resetPendingSkillChoices(pendingSkillChoices)
	clearActiveBuffs(activeBuffs)
	lifeState.Dead = false
	maxHealth.Value = r.playerMaxHealthLocked(entity)
	health.Value = maxHealth.Value
	experience.Value = 0
	radius.Value = r.cfg.PlayerRadius
	fireCooldown.Frames = 0
	networkID.Value = r.allocNetIDLocked()
	ownerPlayerID.Value = playerID

	r.playerEntities[playerID] = entity
	r.latestInputs[playerID] = session.InputState{}
	r.getGameStateLocked().Running = true
}

func (r *Runtime) RespawnPlayer(playerID uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entity, exists := r.playerEntities[playerID]
	if !exists {
		return false
	}

	health := (*Health)(r.world.Get(entity, r.ids.health))
	experience := (*Experience)(r.world.Get(entity, r.ids.experience))
	if health.Value > 0 {
		return false
	}

	spawnPoint := (*SpawnPoint)(r.world.Get(entity, r.ids.spawnPoint))
	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	knockback := (*Knockback)(r.world.Get(entity, r.ids.knockback))
	rollState := (*RollState)(r.world.Get(entity, r.ids.rollState))
	rollLock := (*RollLock)(r.world.Get(entity, r.ids.rollLock))
	rollStats := (*RollStats)(r.world.Get(entity, r.ids.rollStats))
	lifeState := (*LifeState)(r.world.Get(entity, r.ids.lifeState))
	fireCooldown := (*FireCooldown)(r.world.Get(entity, r.ids.fireCooldown))
	maxHealth := (*MaxHealth)(r.world.Get(entity, r.ids.maxHealth))

	pos.X = spawnPoint.X
	pos.Y = spawnPoint.Y
	vel.X = 0
	vel.Y = 0
	knockback.X = 0
	knockback.Y = 0
	knockback.Frames = 0
	r.resetPlayerProgressionStateLocked(entity)
	*rollStats = r.playerRollStatsLocked(entity)
	resetRollState(rollState, rollStats)
	rollLock.Frames = 0
	lifeState.Dead = false
	maxHealth.Value = r.playerMaxHealthLocked(entity)
	health.Value = maxHealth.Value
	experience.Value = 0
	fireCooldown.Frames = 0
	r.latestInputs[playerID] = session.InputState{}
	r.getGameStateLocked().Running = true
	return true
}

func (r *Runtime) resetPlayerProgressionStateLocked(entity ecs.Entity) {
	if !r.world.Alive(entity) {
		return
	}
	if r.world.Has(entity, r.ids.playerLevel) {
		playerLevel := (*PlayerLevel)(r.world.Get(entity, r.ids.playerLevel))
		playerLevel.Value = r.levels.BaseLevel()
	}
	if r.world.Has(entity, r.ids.skillInventory) {
		clearSkillInventory((*SkillInventory)(r.world.Get(entity, r.ids.skillInventory)))
	}
	if r.world.Has(entity, r.ids.pendingSkillChoices) {
		resetPendingSkillChoices((*PendingSkillChoices)(r.world.Get(entity, r.ids.pendingSkillChoices)))
	}
	if r.world.Has(entity, r.ids.activeBuffs) {
		clearActiveBuffs((*ActiveBuffs)(r.world.Get(entity, r.ids.activeBuffs)))
	}
}

func (r *Runtime) RemovePlayer(playerID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entity, ok := r.playerEntities[playerID]
	if ok {
		r.world.RemoveEntity(entity)
	}
	delete(r.playerEntities, playerID)
	delete(r.latestInputs, playerID)
	if len(r.playerEntities) == 0 {
		r.getGameStateLocked().Running = false
	}
}

func (r *Runtime) SetInput(playerID uint32, input session.InputState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.playerEntities[playerID]; !ok {
		return
	}
	if current, ok := r.latestInputs[playerID]; ok && input.Seq < current.Seq {
		return
	}
	r.latestInputs[playerID] = input
}

func (r *Runtime) Tick() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tick++
	r.lastImpacts = r.lastImpacts[:0]
	r.lastPickupEvents = r.lastPickupEvents[:0]
	r.advanceHordeStateLocked()
	r.advanceEnemySpawnStatesLocked()
	r.updateSpawnLocked()
	r.updateRollRecoveryLocked()
	r.updateBuffsLocked()
	r.updatePlayerControlLocked()
	r.updateEnemyAggroLocked()
	r.updateEnemyMovementLocked()
	r.updatePlayerRollLocked()
	r.updateEnemyRollLocked()
	r.updatePlayerFireLocked()
	r.updateEnemyFireLocked()
	r.updateEnemyMeleeLocked()
	r.updateHomingProjectilesLocked()
	r.updateExpOrbSeekLocked()
	r.updateMovementLocked()
	r.applyPlayerBulletDamageLocked()
	r.applyEnemyBulletDamageLocked()
	r.applyExpPickupLocked()
	r.applySkillPickupLocked()
	r.updateLevelProgressionLocked()
	r.updateEnemyCleanupLocked()
	r.updateLifetimeLocked()
	r.cleanupDeadLocked()
	r.updateGameStateLocked()
}

func (r *Runtime) BuildSnapshotFor(playerID uint32) *netproto.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	lastSeq := uint32(0)
	if input, ok := r.latestInputs[playerID]; ok {
		lastSeq = input.Seq
	}

	entities := make([]*netproto.EntityState, 0)
	entities = append(entities, r.exportPlayersLocked()...)
	entities = append(entities, r.exportEnemiesLocked()...)
	entities = append(entities, r.exportPlayerBulletsLocked()...)
	entities = append(entities, r.exportEnemyBulletsLocked()...)
	entities = append(entities, r.exportExpOrbsLocked()...)
	entities = append(entities, r.exportSkillDropsLocked()...)
	impacts := append([]*netproto.ImpactEvent(nil), r.lastImpacts...)
	pickupEvents := append([]*netproto.PickupEvent(nil), r.lastPickupEvents...)

	return &netproto.Snapshot{
		Tick:                  r.tick,
		LastProcessedInputSeq: lastSeq,
		Score:                 r.playerExperienceLocked(playerID),
		Running:               r.getGameStateLocked().Running,
		Entities:              entities,
		Impacts:               impacts,
		PickupEvents:          pickupEvents,
		Horde:                 r.exportHordeStatusLocked(),
		PlayerLevel:           r.playerLevelLocked(playerID),
		PendingSkillChoices:   r.exportPendingSkillChoicesLocked(playerID),
	}
}

func (r *Runtime) GameOver() *netproto.GameOver {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &netproto.GameOver{
		Tick:  r.tick,
		Score: r.totalExperienceLocked(),
	}
}

func (r *Runtime) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getGameStateLocked().Running
}

func (r *Runtime) SkillCatalog() *skillcfg.Catalog {
	return r.skills
}

func (r *Runtime) GrantPlayerSkill(playerID uint32, skillID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entity, ok := r.playerEntities[playerID]
	if !ok {
		return false
	}
	valid, _ := r.tryGrantSkillLocked(entity, skillID)
	return valid
}

func (r *Runtime) ChoosePlayerSkill(playerID uint32, choiceSequence uint32, skillID string) SkillChoiceResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := SkillChoiceResult{
		PlayerID:       playerID,
		ChoiceSequence: choiceSequence,
		Accepted:       false,
		Granted:        false,
		Message:        "player not found",
	}

	entity, ok := r.playerEntities[playerID]
	if !ok {
		return result
	}
	if !r.world.Alive(entity) || !r.world.Has(entity, r.ids.pendingSkillChoices) {
		result.Message = "player progression state missing"
		return result
	}

	pending := (*PendingSkillChoices)(r.world.Get(entity, r.ids.pendingSkillChoices))
	if len(pending.Queue) == 0 {
		result.Message = "no pending skill choices"
		return result
	}

	head := pending.Queue[0]
	result.ChoiceSequence = head.Sequence
	if choiceSequence != head.Sequence {
		result.Message = "skill choice must be resolved in queue order"
		return result
	}
	if !containsString(head.SkillOptions, skillID) {
		result.Message = "skill is not offered by this level-up choice"
		return result
	}

	valid, granted := r.tryGrantSkillLocked(entity, skillID)
	if !valid {
		result.Message = "unknown or invalid skill"
		return result
	}

	pending.Queue[0] = PendingSkillChoice{}
	pending.Queue = pending.Queue[1:]
	r.rerollPendingSkillChoicesLocked(entity, pending)
	result.Accepted = true
	result.Granted = granted
	if granted {
		result.Message = "skill selected"
	} else {
		result.Message = "skill already at max level, selection consumed with no effect"
	}
	return result
}

func (r *Runtime) initComponentIDs() {
	r.ids = componentIDs{
		position:            ecs.ComponentID[Position](&r.world),
		spawnPoint:          ecs.ComponentID[SpawnPoint](&r.world),
		velocity:            ecs.ComponentID[Velocity](&r.world),
		knockback:           ecs.ComponentID[Knockback](&r.world),
		lifeState:           ecs.ComponentID[LifeState](&r.world),
		prevPosition:        ecs.ComponentID[PreviousPosition](&r.world),
		radius:              ecs.ComponentID[Radius](&r.world),
		enemyClass:          ecs.ComponentID[EnemyClass](&r.world),
		enemyTier:           ecs.ComponentID[EnemyTier](&r.world),
		enemyLevel:          ecs.ComponentID[EnemyLevel](&r.world),
		enemySpawnState:     ecs.ComponentID[EnemySpawnState](&r.world),
		aggroTargetPlayerID: ecs.ComponentID[AggroTargetPlayerID](&r.world),
		aggroWatchState:     ecs.ComponentID[AggroWatchState](&r.world),
		enemyMoveState:      ecs.ComponentID[EnemyMoveState](&r.world),
		enemyLifecycle:      ecs.ComponentID[EnemyLifecycle](&r.world),
		rollStats:           ecs.ComponentID[RollStats](&r.world),
		rollState:           ecs.ComponentID[RollState](&r.world),
		rollLock:            ecs.ComponentID[RollLock](&r.world),
		playerLevel:         ecs.ComponentID[PlayerLevel](&r.world),
		skillInventory:      ecs.ComponentID[SkillInventory](&r.world),
		pendingSkillChoices: ecs.ComponentID[PendingSkillChoices](&r.world),
		attackEffects:       ecs.ComponentID[AttackEffects](&r.world),
		activeBuffs:         ecs.ComponentID[ActiveBuffs](&r.world),
		homingProjectile:    ecs.ComponentID[HomingProjectile](&r.world),
		skillDrop:           ecs.ComponentID[SkillDrop](&r.world),
		health:              ecs.ComponentID[Health](&r.world),
		maxHealth:           ecs.ComponentID[MaxHealth](&r.world),
		experience:          ecs.ComponentID[Experience](&r.world),
		damage:              ecs.ComponentID[Damage](&r.world),
		knockbackForce:      ecs.ComponentID[KnockbackForce](&r.world),
		knockbackResistance: ecs.ComponentID[KnockbackResistance](&r.world),
		lifetime:            ecs.ComponentID[Lifetime](&r.world),
		fireCooldown:        ecs.ComponentID[FireCooldown](&r.world),
		networkID:           ecs.ComponentID[NetworkID](&r.world),
		ownerPlayerID:       ecs.ComponentID[OwnerPlayerID](&r.world),
		lastHitByPlayerID:   ecs.ComponentID[LastHitByPlayerID](&r.world),
		pickupValue:         ecs.ComponentID[PickupValue](&r.world),
		dropValue:           ecs.ComponentID[DropValue](&r.world),

		playerTag:       ecs.ComponentID[PlayerTag](&r.world),
		enemyTag:        ecs.ComponentID[EnemyTag](&r.world),
		bulletTag:       ecs.ComponentID[BulletTag](&r.world),
		playerBulletTag: ecs.ComponentID[PlayerBulletTag](&r.world),
		enemyBulletTag:  ecs.ComponentID[EnemyBulletTag](&r.world),
		pickupTag:       ecs.ComponentID[PickupTag](&r.world),
		expOrbTag:       ecs.ComponentID[ExpOrbTag](&r.world),
		skillDropTag:    ecs.ComponentID[SkillDropTag](&r.world),
		autoFireTag:     ecs.ComponentID[AutoFire](&r.world),
	}
}

func (r *Runtime) initResources() {
	r.world.Resources().Add(ecs.ResourceID[SpawnTimer](&r.world), &SpawnTimer{Frames: r.params.SpawnTimerFrames})
	r.world.Resources().Add(ecs.ResourceID[GameState](&r.world), &GameState{Running: true})
	r.world.Resources().Add(ecs.ResourceID[HordeState](&r.world), &HordeState{
		Value:     0,
		Threshold: r.cfg.HordeThreshold,
	})
}

func (r *Runtime) allocNetIDLocked() uint32 {
	id := r.nextNetID
	r.nextNetID++
	return id
}

func (r *Runtime) getSpawnTimerLocked() *SpawnTimer {
	return r.world.Resources().Get(ecs.ResourceID[SpawnTimer](&r.world)).(*SpawnTimer)
}

func (r *Runtime) getGameStateLocked() *GameState {
	return r.world.Resources().Get(ecs.ResourceID[GameState](&r.world)).(*GameState)
}

func (r *Runtime) getHordeStateLocked() *HordeState {
	return r.world.Resources().Get(ecs.ResourceID[HordeState](&r.world)).(*HordeState)
}

func (r *Runtime) playerExperienceLocked(playerID uint32) int32 {
	entity, ok := r.playerEntities[playerID]
	if !ok {
		return 0
	}
	experience := (*Experience)(r.world.Get(entity, r.ids.experience))
	return experience.Value
}

func (r *Runtime) totalExperienceLocked() int32 {
	var total int32
	for playerID := range r.playerEntities {
		total += r.playerExperienceLocked(playerID)
	}
	return total
}

func (r *Runtime) playerLevelLocked(playerID uint32) uint32 {
	entity, ok := r.playerEntities[playerID]
	if !ok || !r.world.Has(entity, r.ids.playerLevel) {
		return r.levels.BaseLevel()
	}
	level := (*PlayerLevel)(r.world.Get(entity, r.ids.playerLevel))
	return level.Value
}
