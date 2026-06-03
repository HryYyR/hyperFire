package gameplay

import (
	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updateHomingProjectilesLocked() {
	filter := generic.NewFilter3[Position, Velocity, HomingProjectile]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&r.world)
	targets := r.snapshotLiveEnemyTargetsLocked()

	for query.Next() {
		pos, vel, homing := query.Get()
		target, found := r.resolveHomingTargetLocked(pos.X, pos.Y, homing, targets)
		if !found {
			continue
		}

		velX, velY, ok := calcAimVelocity(pos.X, pos.Y, target.X, target.Y, homing.Speed)
		if !ok {
			continue
		}
		vel.X = velX
		vel.Y = velY
	}
}

func (r *Runtime) resolveHomingTargetLocked(fromX, fromY float64, homing *HomingProjectile, targets []TargetSnapshot) (TargetSnapshot, bool) {
	if homing == nil {
		return TargetSnapshot{}, false
	}

	if !homing.Target.IsZero() && r.world.Alive(homing.Target) && r.world.Has(homing.Target, r.ids.enemyTag) {
		health := (*Health)(r.world.Get(homing.Target, r.ids.health))
		if health.Value > 0 {
			pos := (*Position)(r.world.Get(homing.Target, r.ids.position))
			return TargetSnapshot{
				Entity: homing.Target,
				X:      pos.X,
				Y:      pos.Y,
			}, true
		}
	}

	homing.Target = ecs.Entity{}
	target, found := findNearestTargetInRadius(fromX, fromY, targets, homing.SearchRadius)
	if !found {
		return TargetSnapshot{}, false
	}

	homing.Target = target.Entity
	return target, true
}
