package gameplay

import "math"

// Enemy stat growth is centralized here so balance changes stay consistent
// across spawn, melee, and projectile systems.
func (r *Runtime) enemyHealthByLevelLocked(tier uint8, level uint32, base int) int {
	scaledBase := int(math.Round(float64(base) * r.enemyHealthBaseScaleLocked(tier)))
	if scaledBase < 1 {
		scaledBase = 1
	}
	return scaleEnemyIntStat(scaledBase, level, r.cfg.EnemyHealthGrowthPerLevel*r.enemyHealthGrowthScaleLocked(tier))
}

func (r *Runtime) enemyBladeDamageByLevelLocked(tier uint8, level uint32) int {
	return scaleEnemyIntStat(r.cfg.EnemyBladeDamage, level, r.cfg.EnemyDamageGrowthPerLevel*r.enemyDamageGrowthScaleLocked(tier))
}

func (r *Runtime) enemyBulletDamageByLevelLocked(tier uint8, level uint32) int {
	return scaleEnemyIntStat(r.cfg.BulletDamage, level, r.cfg.EnemyDamageGrowthPerLevel*r.enemyDamageGrowthScaleLocked(tier))
}

func (r *Runtime) enemyAttackCooldownFramesLocked(tier uint8, class uint8, level uint32) int {
	baseFrames := r.enemyBaseAttackCooldownFramesLocked(class)
	return scaleEnemyCooldownFrames(baseFrames, level, r.cfg.EnemyAttackSpeedGrowthPerLevel*r.enemyAttackSpeedScaleLocked(tier))
}

func (r *Runtime) enemyBaseAttackCooldownFramesLocked(class uint8) int {
	switch class {
	case EnemyClassBlade:
		return r.params.EnemyBladeAttackCooldownFrames
	case EnemyClassGunner:
		return r.params.EnemyGunnerFireCooldownFrames
	default:
		return r.params.EnemyGunnerFireCooldownFrames
	}
}

func (r *Runtime) enemyHealthBaseScaleLocked(tier uint8) float64 {
	switch tier {
	case EnemyTierElite:
		return r.cfg.EnemyEliteBaseHealthScale
	case EnemyTierBoss:
		return r.cfg.EnemyBossBaseHealthScale
	default:
		return 1
	}
}

func (r *Runtime) enemyHealthGrowthScaleLocked(tier uint8) float64 {
	switch tier {
	case EnemyTierElite:
		return r.cfg.EnemyEliteHealthGrowthScale
	case EnemyTierBoss:
		return r.cfg.EnemyBossHealthGrowthScale
	default:
		return 1
	}
}

func (r *Runtime) enemyDamageGrowthScaleLocked(tier uint8) float64 {
	switch tier {
	case EnemyTierElite:
		return r.cfg.EnemyEliteDamageGrowthScale
	case EnemyTierBoss:
		return r.cfg.EnemyBossDamageGrowthScale
	default:
		return 1
	}
}

func (r *Runtime) enemyAttackSpeedScaleLocked(tier uint8) float64 {
	switch tier {
	case EnemyTierElite:
		return r.cfg.EnemyEliteAttackSpeedScale
	case EnemyTierBoss:
		return r.cfg.EnemyBossAttackSpeedScale
	default:
		return 1
	}
}

func scaleEnemyIntStat(base int, level uint32, growthPerLevel float64) int {
	if base <= 0 {
		return 0
	}
	multiplier := enemyLevelMultiplier(level, growthPerLevel)
	value := int(math.Round(float64(base) * multiplier))
	if value < 1 {
		return 1
	}
	return value
}

func scaleEnemyCooldownFrames(baseFrames int, level uint32, growthPerLevel float64) int {
	if baseFrames <= 1 {
		return 1
	}
	multiplier := enemyLevelMultiplier(level, growthPerLevel)
	value := int(math.Round(float64(baseFrames) / multiplier))
	if value < 1 {
		return 1
	}
	return value
}

func enemyLevelMultiplier(level uint32, growthPerLevel float64) float64 {
	if level <= 1 || growthPerLevel <= 0 {
		return 1
	}
	return 1 + float64(level-1)*growthPerLevel
}
