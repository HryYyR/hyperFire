package gameplay

func (r *Runtime) updateSpawnLocked() {
	timer := r.getSpawnTimerLocked()
	timer.Frames--
	if timer.Frames > 0 {
		return
	}

	r.spawnEnemyLocked()
	timer.Frames = r.params.SpawnTimerFrames
}

func (r *Runtime) spawnEnemyLocked() {
	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.health,
		r.ids.radius,
		r.ids.fireCooldown,
		r.ids.networkID,
		r.ids.enemyTag,
		r.ids.autoFireTag,
	)

	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	health := (*Health)(r.world.Get(entity, r.ids.health))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	fireCooldown := (*FireCooldown)(r.world.Get(entity, r.ids.fireCooldown))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))

	pos.X = r.rng.Float64() * r.cfg.SpawnAreaWidth
	pos.Y = r.rng.Float64() * r.cfg.SpawnAreaHeight
	vel.X = 0
	vel.Y = 0
	if r.rng.Intn(2) == 0 {
		health.Value = r.cfg.EnemyHighHealth
	} else {
		health.Value = r.cfg.EnemyLowHealth
	}
	radius.Value = r.cfg.EnemyRadius
	fireCooldown.Frames = r.params.EnemyFireCooldownFrames
	networkID.Value = r.allocNetIDLocked()
}
