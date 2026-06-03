package gameplay

import (
	"math"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

type dodgeBulletSnapshot struct {
	X      float64
	Y      float64
	VelX   float64
	VelY   float64
	Radius float64
}

func (r *Runtime) updateRollRecoveryLocked() {
	filter := generic.NewFilter3[RollState, RollLock, RollStats]()
	query := filter.Query(&r.world)
	for query.Next() {
		rollState, rollLock, rollStats := query.Get()
		advanceRollRecovery(rollState, rollLock, rollStats)
	}
}

func (r *Runtime) updatePlayerRollLocked() {
	filter := generic.NewFilter5[RollState, RollLock, RollStats, OwnerPlayerID, Health]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		rollState, rollLock, rollStats, owner, health := query.Get()
		input := r.latestInputs[owner.Value]

		triggered := input.Roll && !rollState.InputHeld
		rollState.InputHeld = input.Roll
		if !triggered || health.Value <= 0 {
			continue
		}

		dirX, dirY, ok := playerRollDirection(
			input.MoveX,
			input.MoveY,
			input.AimDX,
			input.AimDY,
		)
		if !ok {
			continue
		}
		startRoll(rollState, rollLock, rollStats, dirX, dirY)
	}
}

func (r *Runtime) updateEnemyRollLocked() {
	targets := r.snapshotPlayersLocked()
	bullets := r.snapshotPlayerBulletDodgesLocked()
	if len(targets) == 0 && len(bullets) == 0 {
		return
	}

	filter := generic.NewFilter8[Position, Velocity, EnemyClass, AggroTargetPlayerID, EnemyMoveState, RollState, RollLock, RollStats]().With(generic.T[EnemyTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		pos, vel, class, aggroTarget, moveState, rollState, rollLock, rollStats := query.Get()
		if r.enemyIsSpawningLocked(query.Entity()) {
			continue
		}
		if rollState.ActiveFrames > 0 {
			continue
		}

		target, found := r.selectEnemyChaseTargetLocked(query.Entity(), pos.X, pos.Y, class.Value, aggroTarget, targets)
		if !found {
			continue
		}

		if class.Value == EnemyClassBlade {
			radius := (*Radius)(r.world.Get(query.Entity(), r.ids.radius))
			if r.tryStartEnemyBladeDodgeRollLocked(pos.X, pos.Y, radius.Value, bullets, rollState, rollLock, rollStats) {
				continue
			}
		}
		r.tryStartEnemyRollLocked(class.Value, pos.X, pos.Y, target.X, target.Y, vel.X, vel.Y, moveState, rollState, rollLock, rollStats)
	}
}

func (r *Runtime) snapshotPlayerBulletDodgesLocked() []dodgeBulletSnapshot {
	filter := generic.NewFilter3[Position, Velocity, Radius]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&r.world)
	var bullets []dodgeBulletSnapshot
	for query.Next() {
		pos, vel, radius := query.Get()
		bullets = append(bullets, dodgeBulletSnapshot{
			X:      pos.X,
			Y:      pos.Y,
			VelX:   vel.X,
			VelY:   vel.Y,
			Radius: radius.Value,
		})
	}
	return bullets
}

func (r *Runtime) tryStartEnemyBladeDodgeRollLocked(enemyX, enemyY, enemyRadius float64, bullets []dodgeBulletSnapshot, rollState *RollState, rollLock *RollLock, rollStats *RollStats) bool {
	if len(bullets) == 0 || rollState == nil || rollStats == nil || rollStats.DurationFrames <= 0 {
		return false
	}

	lookaheadFrames := float64(rollStats.DurationFrames)
	bestDist2 := math.MaxFloat64
	var threat dodgeBulletSnapshot
	found := false

	for _, bullet := range bullets {
		endX := bullet.X + bullet.VelX*lookaheadFrames
		endY := bullet.Y + bullet.VelY*lookaheadFrames
		if !judgeSweepHit(bullet.X, bullet.Y, endX, endY, enemyX, enemyY, bullet.Radius, enemyRadius) {
			continue
		}

		dx := bullet.X - enemyX
		dy := bullet.Y - enemyY
		dist2 := dx*dx + dy*dy
		if dist2 < bestDist2 {
			bestDist2 = dist2
			threat = bullet
			found = true
		}
	}
	if !found {
		return false
	}

	bulletDirX, bulletDirY, ok := normalizeVector(threat.VelX, threat.VelY)
	if !ok {
		return false
	}

	relativeX := enemyX - threat.X
	relativeY := enemyY - threat.Y
	cross := bulletDirX*relativeY - bulletDirY*relativeX
	perpX, perpY := perpendicularLeft(bulletDirX, bulletDirY)
	sign := 1.0
	if cross < -1e-6 {
		sign = -1
	} else if math.Abs(cross) <= 1e-6 {
		sign = r.randomSteerSignLocked()
	}

	return startRoll(rollState, rollLock, rollStats, perpX*sign, perpY*sign)
}

func (r *Runtime) tryStartEnemyRollLocked(class uint8, fromX, fromY, toX, toY, moveX, moveY float64, moveState *EnemyMoveState, rollState *RollState, rollLock *RollLock, rollStats *RollStats) {
	switch class {
	case EnemyClassBlade:
		distance := math.Hypot(toX-fromX, toY-fromY)
		if distance <= r.cfg.EnemyBladeAttackRadius*1.2 {
			return
		}
		dirX, dirY, ok := normalizeVector(moveX, moveY)
		if !ok {
			dirX, dirY, ok = normalizeVector(toX-fromX, toY-fromY)
			if !ok {
				return
			}
		}
		startRoll(rollState, rollLock, rollStats, dirX, dirY)
	case EnemyClassGunner:
		distance := math.Hypot(toX-fromX, toY-fromY)
		if distance > r.cfg.EnemyGunnerAggroRadius*0.6 {
			return
		}
		dirX, dirY, ok := normalizeVector(moveX, moveY)
		if !ok {
			sign := r.ensureSteerSignLocked(moveState, true)
			toDirX, toDirY, ok := normalizeVector(toX-fromX, toY-fromY)
			if !ok {
				return
			}
			perpX, perpY := perpendicularLeft(toDirX, toDirY)
			dirX = perpX * sign
			dirY = perpY * sign
		}
		startRoll(rollState, rollLock, rollStats, dirX, dirY)
	}
}

func (r *Runtime) playerRollStatsLocked(entity ecs.Entity) RollStats {
	return RollStats{
		DurationFrames: r.params.RollDurationFrames,
		CooldownFrames: r.params.PlayerRollCooldownFrames,
		MaxCharges:     r.cfg.PlayerRollMaxCharges + r.playerRollBonusChargesLocked(entity),
		Distance:       r.cfg.PlayerRollDistance,
	}
}

func (r *Runtime) enemyRollStatsLocked(class uint8) RollStats {
	switch class {
	case EnemyClassBlade:
		return RollStats{
			DurationFrames: r.params.RollDurationFrames,
			CooldownFrames: r.params.EnemyBladeRollCooldownFrames,
			MaxCharges:     r.cfg.EnemyBladeRollMaxCharges,
			Distance:       r.cfg.EnemyBladeRollDistance,
		}
	case EnemyClassGunner:
		fallthrough
	default:
		return RollStats{
			DurationFrames: r.params.RollDurationFrames,
			CooldownFrames: r.params.EnemyGunnerRollCooldownFrames,
			MaxCharges:     r.cfg.EnemyGunnerRollMaxCharges,
			Distance:       r.cfg.EnemyGunnerRollDistance,
		}
	}
}

func lockRoll(rollLock *RollLock, frames int) {
	if rollLock == nil || frames <= 0 {
		return
	}
	if rollLock.Frames < frames {
		rollLock.Frames = frames
	}
}

func startRoll(rollState *RollState, rollLock *RollLock, rollStats *RollStats, dirX, dirY float64) bool {
	if rollState == nil || rollStats == nil {
		return false
	}
	if rollLock != nil && rollLock.Frames > 0 {
		return false
	}
	if rollState.Charges <= 0 || rollStats.DurationFrames <= 0 || rollStats.Distance <= 0 {
		return false
	}

	dirX, dirY, ok := normalizeVector(dirX, dirY)
	if !ok {
		return false
	}

	rollState.ActiveFrames = rollStats.DurationFrames
	rollState.DirX = dirX
	rollState.DirY = dirY
	rollState.Speed = rollStats.Distance / float64(rollStats.DurationFrames)
	rollState.Charges--
	if rollState.CooldownFrames <= 0 {
		rollState.CooldownFrames = rollStats.CooldownFrames
	}
	return true
}

func resetRollState(rollState *RollState, rollStats *RollStats) {
	if rollState == nil {
		return
	}

	rollState.ActiveFrames = 0
	rollState.CooldownFrames = 0
	rollState.DirX = 0
	rollState.DirY = 0
	rollState.Speed = 0
	rollState.InputHeld = false
	if rollStats != nil {
		rollState.Charges = rollStats.MaxCharges
	}
}

func advanceRollRecovery(rollState *RollState, rollLock *RollLock, rollStats *RollStats) {
	if rollState == nil || rollStats == nil {
		return
	}
	if rollLock != nil && rollLock.Frames > 0 {
		rollLock.Frames--
	}
	if rollState.Charges >= rollStats.MaxCharges {
		rollState.Charges = rollStats.MaxCharges
		rollState.CooldownFrames = 0
		return
	}
	if rollState.CooldownFrames > 0 {
		rollState.CooldownFrames--
	}
	if rollState.CooldownFrames > 0 {
		return
	}

	rollState.Charges++
	if rollState.Charges < rollStats.MaxCharges {
		rollState.CooldownFrames = rollStats.CooldownFrames
	}
}

func advanceRoll(rollState *RollState) {
	if rollState == nil || rollState.ActiveFrames <= 0 {
		return
	}

	rollState.ActiveFrames--
	if rollState.ActiveFrames > 0 {
		return
	}

	rollState.DirX = 0
	rollState.DirY = 0
	rollState.Speed = 0
}

func playerRollDirection(moveX, moveY int32, aimDx, aimDy float32) (float64, float64, bool) {
	moveX = clampMoveAxis(moveX)
	moveY = clampMoveAxis(moveY)
	if dirX, dirY, ok := normalizeVector(float64(moveX), float64(moveY)); ok {
		return dirX, dirY, true
	}
	return normalizeVector(float64(aimDx), float64(aimDy))
}

func (r *Runtime) disableRollForHitLocked(target ecs.Entity) {
	if !r.world.Has(target, r.ids.rollLock) {
		return
	}
	rollLock := (*RollLock)(r.world.Get(target, r.ids.rollLock))
	lockRoll(rollLock, r.params.RollLockOnHitFrames)
}
