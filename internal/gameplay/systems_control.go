package gameplay

import "github.com/mlange-42/arche/generic"

func (r *Runtime) updatePlayerControlLocked() {
	filter := generic.NewFilter2[Velocity, OwnerPlayerID]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		vel, owner := query.Get()
		input := r.latestInputs[owner.Value]
		vel.X = float64(clampMoveAxis(input.MoveX)) * r.params.PlayerMoveSpeed
		vel.Y = float64(clampMoveAxis(input.MoveY)) * r.params.PlayerMoveSpeed
	}
}

func (r *Runtime) updateEnemyChaseLocked() {
	targets := r.snapshotPlayersLocked()
	if len(targets) == 0 {
		return
	}

	filter := generic.NewFilter2[Position, Velocity]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		pos, vel := query.Get()
		target, found := findNearestTargetInRadius(pos.X, pos.Y, targets, r.cfg.EnemyAggroRadius)
		if !found {
			vel.X = 0
			vel.Y = 0
			continue
		}

		velX, velY, ok := calcAimVelocity(pos.X, pos.Y, target.X, target.Y, r.params.EnemyMoveSpeed)
		if !ok {
			continue
		}
		vel.X = velX
		vel.Y = velY
	}
}

func (r *Runtime) updatePlayerFireLocked() {
	filter := generic.NewFilter3[Position, FireCooldown, OwnerPlayerID]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	var toSpawn []BulletSpawn

	for query.Next() {
		pos, fireCooldown, owner := query.Get()
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

		toSpawn = append(toSpawn, BulletSpawn{
			Position:      Position{X: pos.X, Y: pos.Y},
			Velocity:      Velocity{X: velX, Y: velY},
			OwnerPlayerID: owner.Value,
		})
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

	filter := generic.NewFilter2[Position, FireCooldown]().With(generic.T2[EnemyTag, AutoFire]()...)
	query := filter.Query(&r.world)
	var toSpawn []BulletSpawn

	for query.Next() {
		pos, fireCooldown := query.Get()
		if fireCooldown.Frames > 0 {
			fireCooldown.Frames--
		}
		if fireCooldown.Frames > 0 {
			continue
		}

		target, found := findNearestTargetInRadius(pos.X, pos.Y, targets, r.cfg.EnemyAggroRadius)
		if !found {
			continue
		}

		velX, velY, ok := calcAimVelocity(pos.X, pos.Y, target.X, target.Y, r.params.EnemyBulletSpeed)
		if !ok {
			continue
		}

		toSpawn = append(toSpawn, BulletSpawn{
			Position: Position{X: pos.X, Y: pos.Y},
			Velocity: Velocity{X: velX, Y: velY},
		})
		fireCooldown.Frames = r.params.EnemyFireCooldownFrames
	}

	for _, bullet := range toSpawn {
		r.spawnEnemyBulletLocked(bullet)
	}
}

func (r *Runtime) spawnPlayerBulletLocked(spawn BulletSpawn) {
	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.prevPosition,
		r.ids.lifetime,
		r.ids.damage,
		r.ids.radius,
		r.ids.networkID,
		r.ids.ownerPlayerID,
		r.ids.bulletTag,
		r.ids.playerBulletTag,
	)

	pos := (*Position)(r.world.Get(entity, r.ids.position))
	vel := (*Velocity)(r.world.Get(entity, r.ids.velocity))
	prevPos := (*PreviousPosition)(r.world.Get(entity, r.ids.prevPosition))
	lifeTime := (*Lifetime)(r.world.Get(entity, r.ids.lifetime))
	damage := (*Damage)(r.world.Get(entity, r.ids.damage))
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))
	ownerPlayerID := (*OwnerPlayerID)(r.world.Get(entity, r.ids.ownerPlayerID))

	*pos = spawn.Position
	*vel = spawn.Velocity
	prevPos.X = spawn.Position.X
	prevPos.Y = spawn.Position.Y
	lifeTime.Frames = r.params.InitialBulletLifeFrames
	damage.Value = r.cfg.BulletDamage
	radius.Value = r.cfg.BulletRadius
	networkID.Value = r.allocNetIDLocked()
	ownerPlayerID.Value = spawn.OwnerPlayerID
}

func (r *Runtime) spawnEnemyBulletLocked(spawn BulletSpawn) {
	entity := r.world.NewEntity(
		r.ids.position,
		r.ids.velocity,
		r.ids.prevPosition,
		r.ids.lifetime,
		r.ids.damage,
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
	radius := (*Radius)(r.world.Get(entity, r.ids.radius))
	networkID := (*NetworkID)(r.world.Get(entity, r.ids.networkID))

	*pos = spawn.Position
	*vel = spawn.Velocity
	prevPos.X = spawn.Position.X
	prevPos.Y = spawn.Position.Y
	lifeTime.Frames = r.params.InitialBulletLifeFrames
	damage.Value = r.cfg.BulletDamage
	radius.Value = r.cfg.BulletRadius
	networkID.Value = r.allocNetIDLocked()
}
