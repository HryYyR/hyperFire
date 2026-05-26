package gameplay

import (
	"math"

	"github.com/mlange-42/arche/ecs"
)

func findNearestTarget(fromX, fromY float64, targets []TargetSnapshot) (TargetSnapshot, bool) {
	bestDist2 := math.MaxFloat64
	var best TargetSnapshot
	found := false

	for _, target := range targets {
		dx := target.X - fromX
		dy := target.Y - fromY
		dist2 := dx*dx + dy*dy
		if dist2 < bestDist2 {
			bestDist2 = dist2
			best = target
			found = true
		}
	}

	return best, found
}

func findNearestTargetInRadius(fromX, fromY float64, targets []TargetSnapshot, radius float64) (TargetSnapshot, bool) {
	bestDist2 := radius * radius
	var best TargetSnapshot
	found := false

	for _, target := range targets {
		dx := target.X - fromX
		dy := target.Y - fromY
		dist2 := dx*dx + dy*dy
		if dist2 > bestDist2 {
			continue
		}
		if !found || dist2 < bestDist2 {
			bestDist2 = dist2
			best = target
			found = true
		}
	}

	return best, found
}

func calcAimVelocity(fromX, fromY, toX, toY, speed float64) (float64, float64, bool) {
	dx := toX - fromX
	dy := toY - fromY
	dist2 := dx*dx + dy*dy
	if dist2 <= 0.0001 {
		return 0, 0, false
	}

	dist := math.Sqrt(dist2)
	return dx / dist * speed, dy / dist * speed, true
}

func collectSweepHitEvents(bullets []BulletSnapshot, targets []HitTargetSnapshot) ([]HitEvent, map[uint32]ecs.Entity) {
	var hitEvents []HitEvent
	bulletsToRemove := make(map[uint32]ecs.Entity)

	for _, bullet := range bullets {
		for _, target := range targets {
			if judgeSweepHit(bullet.PrevX, bullet.PrevY, bullet.X, bullet.Y, target.X, target.Y, bullet.Radius, target.Radius) {
				hitEvents = append(hitEvents, HitEvent{
					Bullet: bullet.Entity,
					Target: target.Entity,
					Damage: bullet.Damage,
				})
				bulletsToRemove[bullet.Entity.ID()] = bullet.Entity
			}
		}
	}

	return hitEvents, bulletsToRemove
}

func collectDiscreteHitEvents(bullets []BulletSnapshot, targets []HitTargetSnapshot) ([]HitEvent, map[uint32]ecs.Entity) {
	var hitEvents []HitEvent
	bulletsToRemove := make(map[uint32]ecs.Entity)

	for _, bullet := range bullets {
		for _, target := range targets {
			if judgeHit(bullet.X, bullet.Y, target.X, target.Y, bullet.Radius, target.Radius) {
				hitEvents = append(hitEvents, HitEvent{
					Bullet: bullet.Entity,
					Target: target.Entity,
					Damage: bullet.Damage,
				})
				bulletsToRemove[bullet.Entity.ID()] = bullet.Entity
			}
		}
	}

	return hitEvents, bulletsToRemove
}

func collectSampledHitEvents(bullets []BulletSnapshot, targets []HitTargetSnapshot, maxStep float64) ([]HitEvent, map[uint32]ecs.Entity) {
	if maxStep <= 0 {
		return collectDiscreteHitEvents(bullets, targets)
	}

	var hitEvents []HitEvent
	bulletsToRemove := make(map[uint32]ecs.Entity)

	for _, bullet := range bullets {
		dx := bullet.X - bullet.PrevX
		dy := bullet.Y - bullet.PrevY
		distance := math.Sqrt(dx*dx + dy*dy)
		steps := int(math.Ceil(distance / maxStep))
		if steps < 1 {
			steps = 1
		}

		hit := false
		for step := 1; step <= steps && !hit; step++ {
			t := float64(step) / float64(steps)
			sampleX := bullet.PrevX + dx*t
			sampleY := bullet.PrevY + dy*t
			for _, target := range targets {
				if judgeHit(sampleX, sampleY, target.X, target.Y, bullet.Radius, target.Radius) {
					hitEvents = append(hitEvents, HitEvent{
						Bullet: bullet.Entity,
						Target: target.Entity,
						Damage: bullet.Damage,
					})
					bulletsToRemove[bullet.Entity.ID()] = bullet.Entity
					hit = true
					break
				}
			}
		}
	}

	return hitEvents, bulletsToRemove
}

func judgeSweepHit(fromX, fromY, toX, toY, targetX, targetY, bulletRadiusValue, targetRadiusValue float64) bool {
	if judgeHit(toX, toY, targetX, targetY, bulletRadiusValue, targetRadiusValue) {
		return true
	}

	segX := toX - fromX
	segY := toY - fromY
	segLenSquared := segX*segX + segY*segY
	if segLenSquared <= 0.0001 {
		return judgeHit(fromX, fromY, targetX, targetY, bulletRadiusValue, targetRadiusValue)
	}

	toTargetX := targetX - fromX
	toTargetY := targetY - fromY
	projection := (toTargetX*segX + toTargetY*segY) / segLenSquared
	switch {
	case projection < 0:
		projection = 0
	case projection > 1:
		projection = 1
	}

	closestX := fromX + segX*projection
	closestY := fromY + segY*projection
	return judgeHit(closestX, closestY, targetX, targetY, bulletRadiusValue, targetRadiusValue)
}

func judgeHit(bulletPosX, bulletPosY, targetPosX, targetPosY, bulletRadiusValue, targetRadiusValue float64) bool {
	dx := bulletPosX - targetPosX
	dy := bulletPosY - targetPosY
	distanceSquared := dx*dx + dy*dy
	radiusSum := bulletRadiusValue + targetRadiusValue
	return distanceSquared <= radiusSum*radiusSum
}

func clampMoveAxis(value int32) int32 {
	switch {
	case value > 1:
		return 1
	case value < -1:
		return -1
	default:
		return value
	}
}
