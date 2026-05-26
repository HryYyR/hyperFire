package gameplay

import (
	"math/rand"
	"sync"

	"agentDemo/internal/netproto"
	"agentDemo/internal/session"

	"github.com/mlange-42/arche/ecs"
)

type Runtime struct {
	mu sync.Mutex

	world ecs.World
	rng   *rand.Rand
	ids   componentIDs

	cfg    Config
	params runtimeParams

	tick      uint32
	nextNetID uint32

	playerEntities map[uint32]ecs.Entity
	latestInputs   map[uint32]session.InputState
	// lastImpacts only lives for the current tick and is copied into the snapshot.
	lastImpacts []*netproto.ImpactEvent
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
		r.ids.velocity,
		r.ids.health,
		r.ids.radius,
		r.ids.fireCooldown,
		r.ids.networkID,
		r.ids.ownerPlayerID,
		r.ids.playerTag,
	)

	index := float64(len(r.playerEntities))
	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	health := (*Health)(r.world.Get(entity, r.ids.health))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	fireCooldown := (*FireCooldown)(r.world.Get(entity, r.ids.fireCooldown))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))
	ownerPlayerID := (*OwnerPlayerID)(r.world.Get(entity, r.ids.ownerPlayerID))

	pos.X = r.cfg.SpawnBaseX + index*r.cfg.SpawnSpacingX
	pos.Y = r.cfg.SpawnBaseY
	vel.X = 0
	vel.Y = 0
	health.Value = r.cfg.PlayerHealth
	radius.Value = r.cfg.PlayerRadius
	fireCooldown.Frames = 0
	networkID.Value = r.allocNetIDLocked()
	ownerPlayerID.Value = playerID

	r.playerEntities[playerID] = entity
	r.latestInputs[playerID] = session.InputState{}
	r.getGameStateLocked().Running = true
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
	r.updateSpawnLocked()
	r.updatePlayerControlLocked()
	r.updateEnemyChaseLocked()
	r.updatePlayerFireLocked()
	r.updateEnemyFireLocked()
	r.updateMovementLocked()
	r.updateLifetimeLocked()
	r.applyPlayerBulletDamageLocked()
	r.applyEnemyBulletDamageLocked()
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
	impacts := append([]*netproto.ImpactEvent(nil), r.lastImpacts...)

	return &netproto.Snapshot{
		Tick:                  r.tick,
		LastProcessedInputSeq: lastSeq,
		Score:                 r.getScoreLocked().Value,
		Running:               r.getGameStateLocked().Running,
		Entities:              entities,
		Impacts:               impacts,
	}
}

func (r *Runtime) GameOver() *netproto.GameOver {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &netproto.GameOver{
		Tick:  r.tick,
		Score: r.getScoreLocked().Value,
	}
}

func (r *Runtime) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getGameStateLocked().Running
}

func (r *Runtime) initComponentIDs() {
	r.ids = componentIDs{
		position:      ecs.ComponentID[Position](&r.world),
		velocity:      ecs.ComponentID[Velocity](&r.world),
		prevPosition:  ecs.ComponentID[PreviousPosition](&r.world),
		radius:        ecs.ComponentID[Radius](&r.world),
		health:        ecs.ComponentID[Health](&r.world),
		damage:        ecs.ComponentID[Damage](&r.world),
		lifetime:      ecs.ComponentID[Lifetime](&r.world),
		fireCooldown:  ecs.ComponentID[FireCooldown](&r.world),
		networkID:     ecs.ComponentID[NetworkID](&r.world),
		ownerPlayerID: ecs.ComponentID[OwnerPlayerID](&r.world),

		playerTag:       ecs.ComponentID[PlayerTag](&r.world),
		enemyTag:        ecs.ComponentID[EnemyTag](&r.world),
		bulletTag:       ecs.ComponentID[BulletTag](&r.world),
		playerBulletTag: ecs.ComponentID[PlayerBulletTag](&r.world),
		enemyBulletTag:  ecs.ComponentID[EnemyBulletTag](&r.world),
		autoFireTag:     ecs.ComponentID[AutoFire](&r.world),
	}
}

func (r *Runtime) initResources() {
	r.world.Resources().Add(ecs.ResourceID[SpawnTimer](&r.world), &SpawnTimer{Frames: r.params.SpawnTimerFrames})
	r.world.Resources().Add(ecs.ResourceID[Score](&r.world), &Score{Value: 0})
	r.world.Resources().Add(ecs.ResourceID[GameState](&r.world), &GameState{Running: true})
}

func (r *Runtime) allocNetIDLocked() uint32 {
	id := r.nextNetID
	r.nextNetID++
	return id
}

func (r *Runtime) getSpawnTimerLocked() *SpawnTimer {
	return r.world.Resources().Get(ecs.ResourceID[SpawnTimer](&r.world)).(*SpawnTimer)
}

func (r *Runtime) getScoreLocked() *Score {
	return r.world.Resources().Get(ecs.ResourceID[Score](&r.world)).(*Score)
}

func (r *Runtime) getGameStateLocked() *GameState {
	return r.world.Resources().Get(ecs.ResourceID[GameState](&r.world)).(*GameState)
}
