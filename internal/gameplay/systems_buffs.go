package gameplay

import (
	"math"
	"slices"

	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updateBuffsLocked() {
	filter := generic.NewFilter2[Health, ActiveBuffs]()
	query := filter.Query(&r.world)
	for query.Next() {
		health, activeBuffs := query.Get()
		if health.Value <= 0 {
			clearActiveBuffs(activeBuffs)
			continue
		}
		updateBuffList(health, activeBuffs)
	}
}

func updateBuffList(health *Health, activeBuffs *ActiveBuffs) {
	if activeBuffs == nil || len(activeBuffs.Items) == 0 {
		return
	}

	items := activeBuffs.Items[:0]
	for _, buff := range activeBuffs.Items {
		updated, alive := updateBuffInstance(health, buff)
		if !alive {
			continue
		}
		items = append(items, updated)
	}
	activeBuffs.Items = items
}

func clearActiveBuffs(activeBuffs *ActiveBuffs) {
	if activeBuffs == nil {
		return
	}
	for i := range activeBuffs.Items {
		activeBuffs.Items[i] = BuffInstance{}
	}
	activeBuffs.Items = activeBuffs.Items[:0]
}

func cloneEffectConfigs(effects []skillcfg.EffectConfig) []skillcfg.EffectConfig {
	if len(effects) == 0 {
		return nil
	}
	cloned := make([]skillcfg.EffectConfig, len(effects))
	copy(cloned, effects)
	return cloned
}

func (r *Runtime) collectSkillEffectsByTriggerLocked(source ecs.Entity, trigger skillcfg.EffectTrigger) []skillcfg.EffectConfig {
	if source.IsZero() || !r.world.Alive(source) || !r.world.Has(source, r.ids.skillInventory) {
		return nil
	}

	inventory := (*SkillInventory)(r.world.Get(source, r.ids.skillInventory))
	if len(inventory.Skills) == 0 {
		return nil
	}

	var effects []skillcfg.EffectConfig
	for _, progress := range inventory.Skills {
		skillEffects, ok := r.skills.ResolveSkillLevel(progress.SkillID, progress.Level)
		if !ok {
			continue
		}
		for _, effect := range skillEffects {
			if effect.Trigger != trigger {
				continue
			}
			effects = append(effects, effect)
		}
	}
	return effects
}

func (r *Runtime) collectOnHitEffectsLocked(source ecs.Entity) []skillcfg.EffectConfig {
	return r.collectSkillEffectsByTriggerLocked(source, skillcfg.EffectTriggerOnAttackHit)
}

func (r *Runtime) collectEnemyKillTriggeredEffectsLocked(source ecs.Entity) []skillcfg.EffectConfig {
	return r.collectSkillEffectsByTriggerLocked(source, skillcfg.EffectTriggerOnEnemyKill)
}

func (r *Runtime) collectPassiveEffectsLocked(source ecs.Entity) []skillcfg.EffectConfig {
	return r.collectSkillEffectsByTriggerLocked(source, skillcfg.EffectTriggerPassive)
}

func (r *Runtime) collectShotTriggeredEffectsLocked(source ecs.Entity) []skillcfg.EffectConfig {
	if source.IsZero() || !r.world.Alive(source) || !r.world.Has(source, r.ids.skillInventory) {
		return nil
	}

	inventory := (*SkillInventory)(r.world.Get(source, r.ids.skillInventory))
	if len(inventory.Skills) == 0 {
		return nil
	}

	var effects []skillcfg.EffectConfig
	for i := range inventory.Skills {
		progress := &inventory.Skills[i]
		skillEffects, ok := r.skills.ResolveSkillLevel(progress.SkillID, progress.Level)
		if !ok {
			continue
		}

		hasShotTrigger := false
		for _, effect := range skillEffects {
			if effect.Trigger == skillcfg.EffectTriggerOnShotFired {
				hasShotTrigger = true
				break
			}
		}
		if !hasShotTrigger {
			continue
		}

		progress.ShotsFired++
		for _, effect := range skillEffects {
			if effect.Trigger != skillcfg.EffectTriggerOnShotFired {
				continue
			}
			if shotTriggerSatisfied(progress.ShotsFired, effect) {
				effects = append(effects, effect)
			}
		}
	}
	return effects
}

func (r *Runtime) applyOnHitEffectsLocked(target ecs.Entity, effects []skillcfg.EffectConfig) {
	if len(effects) == 0 || !r.world.Has(target, r.ids.activeBuffs) {
		return
	}

	activeBuffs := (*ActiveBuffs)(r.world.Get(target, r.ids.activeBuffs))
	for _, effect := range effects {
		if effect.Kind != skillcfg.EffectKindApplyStatusOnHit || effect.ApplyStatusOnHit == nil {
			continue
		}
		r.applyStatusBuffLocked(activeBuffs, *effect.ApplyStatusOnHit)
	}
}

func (r *Runtime) applyStatusBuffLocked(activeBuffs *ActiveBuffs, cfg skillcfg.ApplyStatusOnHitConfig) {
	switch cfg.Status {
	case skillcfg.StatusKindPoison:
		if cfg.Dot == nil {
			return
		}
		applyDotBuff(activeBuffs, BuffInstance{
			Category:            cfg.Category,
			StackingRule:        cfg.StackingRule,
			Status:              skillcfg.StatusKindPoison,
			RemainingFrames:     framesFromDurationSeconds(cfg.Dot.DurationSeconds, r.cfg.TickHz),
			TickIntervalFrames:  cfg.Dot.TickIntervalTicks,
			TickFramesRemaining: cfg.Dot.TickIntervalTicks,
			DamagePerTick:       cfg.Dot.DamagePerTick,
			MoveSpeedMultiplier: 1,
			Stacks:              1,
			MaxStacks:           cfg.Dot.MaxStacks,
		})
	case skillcfg.StatusKindChill:
		if cfg.Chill == nil {
			return
		}
		applyUniqueBuff(activeBuffs, BuffInstance{
			Category:            cfg.Category,
			StackingRule:        cfg.StackingRule,
			Status:              skillcfg.StatusKindChill,
			RemainingFrames:     framesFromDurationSeconds(cfg.Chill.DurationSeconds, r.cfg.TickHz),
			MoveSpeedMultiplier: cfg.Chill.MoveSpeedMultiplier,
			TickIntervalFrames:  0,
			TickFramesRemaining: 0,
			Stacks:              1,
			MaxStacks:           1,
		})
	}
}

func updateBuffInstance(health *Health, buff BuffInstance) (BuffInstance, bool) {
	buff.RemainingFrames--
	if buff.RemainingFrames <= 0 {
		return buff, false
	}

	switch buff.Category {
	case skillcfg.BuffCategoryDot:
		updateDotBuff(health, &buff)
	}

	return buff, true
}

func updateDotBuff(health *Health, buff *BuffInstance) {
	if health == nil || buff == nil || buff.TickIntervalFrames <= 0 || buff.DamagePerTick <= 0 {
		return
	}

	buff.TickFramesRemaining--
	if buff.TickFramesRemaining > 0 {
		return
	}

	stacks := positiveOrOne(buff.Stacks)
	health.Value -= buff.DamagePerTick * stacks
	buff.TickFramesRemaining = buff.TickIntervalFrames
}

func applyDotBuff(activeBuffs *ActiveBuffs, incoming BuffInstance) {
	if activeBuffs == nil || incoming.RemainingFrames <= 0 {
		return
	}
	incoming.Stacks = positiveOrOne(incoming.Stacks)
	incoming.MaxStacks = maxInt(incoming.MaxStacks, incoming.Stacks)

	for i := range activeBuffs.Items {
		existing := &activeBuffs.Items[i]
		if !sameBuffKey(*existing, incoming) {
			continue
		}
		applyDotStacking(existing, incoming)
		return
	}

	activeBuffs.Items = append(activeBuffs.Items, incoming)
}

func applyDotStacking(existing *BuffInstance, incoming BuffInstance) {
	if existing == nil {
		return
	}

	existing.Category = incoming.Category
	existing.StackingRule = incoming.StackingRule
	existing.RemainingFrames = maxInt(existing.RemainingFrames, incoming.RemainingFrames)
	if incoming.DamagePerTick > existing.DamagePerTick {
		existing.DamagePerTick = incoming.DamagePerTick
	}
	if incoming.TickIntervalFrames > 0 && (existing.TickIntervalFrames == 0 || incoming.TickIntervalFrames < existing.TickIntervalFrames) {
		existing.TickIntervalFrames = incoming.TickIntervalFrames
	}
	if existing.TickFramesRemaining <= 0 || (incoming.TickIntervalFrames > 0 && existing.TickFramesRemaining > incoming.TickIntervalFrames) {
		existing.TickFramesRemaining = incoming.TickIntervalFrames
	}
	existing.MaxStacks = maxInt(existing.MaxStacks, incoming.MaxStacks)
	existing.Stacks = minInt(existing.MaxStacks, positiveOrOne(existing.Stacks)+positiveOrOne(incoming.Stacks))
}

func applyUniqueBuff(activeBuffs *ActiveBuffs, incoming BuffInstance) {
	if activeBuffs == nil || incoming.RemainingFrames <= 0 {
		return
	}
	incoming.Stacks = positiveOrOne(incoming.Stacks)
	incoming.MaxStacks = maxInt(incoming.MaxStacks, incoming.Stacks)

	for i := range activeBuffs.Items {
		existing := &activeBuffs.Items[i]
		if !sameBuffKey(*existing, incoming) {
			continue
		}
		existing.Category = incoming.Category
		existing.StackingRule = incoming.StackingRule
		existing.RemainingFrames = maxInt(existing.RemainingFrames, incoming.RemainingFrames)
		existing.TickIntervalFrames = incoming.TickIntervalFrames
		existing.TickFramesRemaining = incoming.TickFramesRemaining
		existing.DamagePerTick = maxInt(existing.DamagePerTick, incoming.DamagePerTick)
		if incoming.MoveSpeedMultiplier > 0 {
			if existing.MoveSpeedMultiplier <= 0 {
				existing.MoveSpeedMultiplier = incoming.MoveSpeedMultiplier
			} else {
				existing.MoveSpeedMultiplier = math.Min(existing.MoveSpeedMultiplier, incoming.MoveSpeedMultiplier)
			}
		}
		existing.Stacks = 1
		existing.MaxStacks = maxInt(existing.MaxStacks, incoming.MaxStacks)
		return
	}

	activeBuffs.Items = append(activeBuffs.Items, incoming)
}

func sameBuffKey(left, right BuffInstance) bool {
	return left.Status == right.Status
}

func (r *Runtime) entityMoveSpeedMultiplierLocked(entity ecs.Entity) float64 {
	if !r.world.Has(entity, r.ids.activeBuffs) {
		return 1
	}

	activeBuffs := (*ActiveBuffs)(r.world.Get(entity, r.ids.activeBuffs))
	multiplier := 1.0
	for _, buff := range activeBuffs.Items {
		if buff.Status != skillcfg.StatusKindChill || buff.RemainingFrames <= 0 {
			continue
		}
		if buff.MoveSpeedMultiplier > 0 {
			multiplier = math.Min(multiplier, buff.MoveSpeedMultiplier)
		}
	}
	return multiplier
}

func (r *Runtime) grantSkillLocked(entity ecs.Entity, skillID string) bool {
	valid, _ := r.tryGrantSkillLocked(entity, skillID)
	return valid
}

func (r *Runtime) tryGrantSkillLocked(entity ecs.Entity, skillID string) (bool, bool) {
	if !r.world.Alive(entity) || !r.world.Has(entity, r.ids.skillInventory) {
		return false, false
	}
	skill, ok := r.skills.Skill(skillID)
	if !ok {
		return false, false
	}

	inventory := (*SkillInventory)(r.world.Get(entity, r.ids.skillInventory))
	oldMaxHealth := 0
	oldRollCharges := 0
	if r.world.Has(entity, r.ids.playerTag) {
		oldMaxHealth = r.playerMaxHealthLocked(entity)
		if r.world.Has(entity, r.ids.rollStats) {
			rollStats := (*RollStats)(r.world.Get(entity, r.ids.rollStats))
			oldRollCharges = rollStats.MaxCharges
		}
	}
	for i := range inventory.Skills {
		progress := &inventory.Skills[i]
		if progress.SkillID != skillID {
			continue
		}
		if progress.Level >= skill.MaxLevel() {
			return true, false
		}
		progress.Level++
		r.refreshGrantedSkillStateLocked(entity, oldMaxHealth, oldRollCharges)
		return true, true
	}

	inventory.Skills = append(inventory.Skills, SkillProgress{
		SkillID: skillID,
		Level:   1,
	})
	r.refreshGrantedSkillStateLocked(entity, oldMaxHealth, oldRollCharges)
	return true, true
}

func (r *Runtime) refreshGrantedSkillStateLocked(entity ecs.Entity, oldMaxHealth int, oldRollCharges int) {
	if !r.world.Has(entity, r.ids.playerTag) {
		return
	}
	r.refreshPlayerDerivedStatsLocked(entity, oldMaxHealth, oldRollCharges)
}

func (r *Runtime) refreshPlayerDerivedStatsLocked(entity ecs.Entity, oldMaxHealth int, oldRollCharges int) {
	if entity.IsZero() || !r.world.Alive(entity) || !r.world.Has(entity, r.ids.playerTag) {
		return
	}

	newRollStats := r.playerRollStatsLocked(entity)
	if r.world.Has(entity, r.ids.rollStats) {
		rollStats := (*RollStats)(r.world.Get(entity, r.ids.rollStats))
		rollState := (*RollState)(r.world.Get(entity, r.ids.rollState))
		newMaxCharges := newRollStats.MaxCharges
		chargeDelta := newMaxCharges - oldRollCharges
		*rollStats = newRollStats
		if rollState != nil {
			if chargeDelta > 0 {
				rollState.Charges = minInt(newMaxCharges, rollState.Charges+chargeDelta)
			} else if rollState.Charges > newMaxCharges {
				rollState.Charges = newMaxCharges
			}
		}
	}

	if !r.world.Has(entity, r.ids.health) {
		return
	}
	if oldMaxHealth <= 0 {
		oldMaxHealth = r.cfg.PlayerHealth
	}

	health := (*Health)(r.world.Get(entity, r.ids.health))
	newMaxHealth := r.playerMaxHealthLocked(entity)
	if r.world.Has(entity, r.ids.maxHealth) {
		maxHealth := (*MaxHealth)(r.world.Get(entity, r.ids.maxHealth))
		maxHealth.Value = newMaxHealth
	}
	switch {
	case health.Value > newMaxHealth:
		health.Value = newMaxHealth
	case health.Value == oldMaxHealth:
		health.Value = newMaxHealth
	}
}

func (r *Runtime) playerRollBonusChargesLocked(entity ecs.Entity) int {
	effects := r.collectPassiveEffectsLocked(entity)
	if len(effects) == 0 {
		return 0
	}

	total := 0
	for _, effect := range effects {
		if effect.Kind != skillcfg.EffectKindBonusRollCharges || effect.BonusRollCharges == nil {
			continue
		}
		total += effect.BonusRollCharges.ExtraCharges
	}
	return total
}

func (r *Runtime) playerMaxHealthLocked(entity ecs.Entity) int {
	scale := 1.0
	for _, effect := range r.collectPassiveEffectsLocked(entity) {
		if effect.Kind != skillcfg.EffectKindMaxHealthScale || effect.MaxHealthScale == nil {
			continue
		}
		scale += effect.MaxHealthScale.Percent
	}
	value := int(math.Round(float64(r.cfg.PlayerHealth) * scale))
	if value < 1 {
		return 1
	}
	return value
}

func (r *Runtime) applyPlayerKillHealLocked(entity ecs.Entity) {
	if entity.IsZero() || !r.world.Alive(entity) || !r.world.Has(entity, r.ids.health) {
		return
	}

	health := (*Health)(r.world.Get(entity, r.ids.health))
	if health.Value <= 0 {
		return
	}

	maxHealth := r.playerMaxHealthLocked(entity)
	if maxHealth <= 0 {
		return
	}

	healTotal := 0
	for _, effect := range r.collectEnemyKillTriggeredEffectsLocked(entity) {
		if effect.Kind != skillcfg.EffectKindRestoreHealthOnKill || effect.RestoreHealthOnKill == nil {
			continue
		}
		heal := int(math.Floor(float64(maxHealth) * effect.RestoreHealthOnKill.MaxHealthPercent))
		if heal < effect.RestoreHealthOnKill.MinHeal {
			heal = effect.RestoreHealthOnKill.MinHeal
		}
		healTotal += heal
	}
	if healTotal <= 0 {
		return
	}

	health.Value = minInt(maxHealth, health.Value+healTotal)
}

func clearSkillInventory(inventory *SkillInventory) {
	if inventory == nil {
		return
	}
	for i := range inventory.Skills {
		inventory.Skills[i] = SkillProgress{}
	}
	inventory.Skills = inventory.Skills[:0]
}

func skillLevelInInventory(inventory *SkillInventory, skillID string) int {
	if inventory == nil {
		return 0
	}
	for _, progress := range inventory.Skills {
		if progress.SkillID == skillID {
			return progress.Level
		}
	}
	return 0
}

func inventorySkillIDs(inventory *SkillInventory) []string {
	if inventory == nil || len(inventory.Skills) == 0 {
		return nil
	}
	result := make([]string, 0, len(inventory.Skills))
	for _, progress := range inventory.Skills {
		if progress.SkillID == "" {
			continue
		}
		result = append(result, progress.SkillID)
	}
	return result
}

func containsString(items []string, target string) bool {
	return slices.Contains(items, target)
}

func containsUint32(items []uint32, target uint32) bool {
	return slices.Contains(items, target)
}

func framesFromDurationSeconds(seconds float64, tickHz uint32) int {
	if seconds <= 0 {
		return 0
	}
	frames := int(math.Round(seconds * float64(tickHz)))
	if frames < 1 {
		return 1
	}
	return frames
}

func shotTriggerSatisfied(totalShots int, effect skillcfg.EffectConfig) bool {
	if totalShots <= 0 {
		return false
	}

	switch effect.Kind {
	case skillcfg.EffectKindBonusHomingShotOnFire:
		if effect.BonusHomingShotOnFire == nil || effect.BonusHomingShotOnFire.TriggerEveryShots <= 0 {
			return false
		}
		return totalShots%effect.BonusHomingShotOnFire.TriggerEveryShots == 0
	default:
		return false
	}
}

func positiveOrOne(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
