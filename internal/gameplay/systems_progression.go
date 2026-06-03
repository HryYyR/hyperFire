package gameplay

import (
	"fmt"
	"slices"
	"strings"

	"agentDemo/internal/netproto"
	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func (r *Runtime) updateLevelProgressionLocked() {
	filter := generic.NewFilter4[Experience, Health, PlayerLevel, PendingSkillChoices]().With(generic.T[PlayerTag]())
	query := filter.Query(&r.world)
	for query.Next() {
		experience, health, playerLevel, pendingSkillChoices := query.Get()
		entity := query.Entity()
		if health.Value <= 0 {
			continue
		}

		targetLevel := r.levels.LevelForExp(experience.Value)
		if targetLevel.Level <= playerLevel.Value {
			continue
		}

		for _, entry := range r.levels.EntriesBetweenLevels(playerLevel.Value, targetLevel.Level) {
			enqueuePendingSkillChoice(pendingSkillChoices, entry.Level, r.rollLevelUpSkillOptionsLocked(entity, 3))
		}
		playerLevel.Value = targetLevel.Level
	}
}

func enqueuePendingSkillChoice(pending *PendingSkillChoices, targetLevel uint32, skillOptions []string) {
	if pending == nil || len(skillOptions) == 0 {
		return
	}

	choice := PendingSkillChoice{
		Sequence:     pending.NextSequence,
		TargetLevel:  targetLevel,
		SkillOptions: append([]string(nil), skillOptions...),
	}
	pending.NextSequence++
	pending.Queue = append(pending.Queue, choice)
}

func resetPendingSkillChoices(pending *PendingSkillChoices) {
	if pending == nil {
		return
	}

	for i := range pending.Queue {
		pending.Queue[i] = PendingSkillChoice{}
	}
	pending.Queue = pending.Queue[:0]
	pending.NextSequence = 1
}

func (r *Runtime) rollLevelUpSkillOptionsLocked(entity ecs.Entity, count int) []string {
	if entity.IsZero() || !r.world.Alive(entity) || !r.world.Has(entity, r.ids.skillInventory) || count <= 0 {
		return nil
	}

	inventory := (*SkillInventory)(r.world.Get(entity, r.ids.skillInventory))
	available := r.availableLevelUpSkillsLocked(inventory)
	if len(available) == 0 {
		return nil
	}
	if count > len(available) {
		count = len(available)
	}

	perm := r.rng.Perm(len(available))
	options := make([]string, 0, count)
	for _, index := range perm[:count] {
		options = append(options, available[index])
	}
	slices.Sort(options)
	return options
}

func (r *Runtime) availableLevelUpSkillsLocked(inventory *SkillInventory) []string {
	allSkills := r.skills.Skills()
	if len(allSkills) == 0 {
		return nil
	}

	options := make([]string, 0, len(allSkills))
	for _, skill := range allSkills {
		if skill.ID == "" {
			continue
		}
		if skillLevelInInventory(inventory, skill.ID) >= skill.MaxLevel() {
			continue
		}
		options = append(options, skill.ID)
	}
	return options
}

func (r *Runtime) rerollPendingSkillChoicesLocked(entity ecs.Entity, pending *PendingSkillChoices) {
	if pending == nil || len(pending.Queue) == 0 {
		return
	}

	next := pending.Queue[:0]
	for _, choice := range pending.Queue {
		choice.SkillOptions = r.rollLevelUpSkillOptionsLocked(entity, 3)
		if len(choice.SkillOptions) == 0 {
			continue
		}
		next = append(next, choice)
	}
	pending.Queue = next
}

func (r *Runtime) exportPendingSkillChoicesLocked(playerID uint32) []*netproto.PendingSkillChoice {
	entity, ok := r.playerEntities[playerID]
	if !ok || !r.world.Has(entity, r.ids.pendingSkillChoices) {
		return nil
	}

	pending := (*PendingSkillChoices)(r.world.Get(entity, r.ids.pendingSkillChoices))
	if pending == nil || len(pending.Queue) == 0 {
		return nil
	}
	inventory := (*SkillInventory)(r.world.Get(entity, r.ids.skillInventory))

	result := make([]*netproto.PendingSkillChoice, 0, len(pending.Queue))
	for _, queued := range pending.Queue {
		options := make([]*netproto.SkillOption, 0, len(queued.SkillOptions))
		for _, skillID := range queued.SkillOptions {
			skill, ok := r.skills.Skill(skillID)
			if ok {
				currentLevel := skillLevelInInventory(inventory, skillID)
				options = append(options, &netproto.SkillOption{
					SkillId:     skill.ID,
					Name:        skill.Name,
					Description: describeSkillOption(skill, currentLevel, r.skillLevelEffectDescriptionLocked(skill.ID, currentLevel+1)),
				})
				continue
			}
			options = append(options, &netproto.SkillOption{
				SkillId: skillID,
				Name:    skillID,
			})
		}
		result = append(result, &netproto.PendingSkillChoice{
			Sequence:    queued.Sequence,
			TargetLevel: queued.TargetLevel,
			Options:     options,
		})
	}
	return result
}

func (r *Runtime) skillLevelEffectDescriptionLocked(skillID string, level int) string {
	effects, ok := r.skills.ResolveSkillLevel(skillID, level)
	if !ok {
		return ""
	}

	parts := make([]string, 0, len(effects))
	for _, effect := range effects {
		if effect.Description != "" {
			parts = append(parts, effect.Description)
		}
	}
	return strings.Join(parts, " ")
}

func describeSkillOption(skill skillcfg.SkillConfig, currentLevel int, nextLevelDescription string) string {
	base := skill.Description
	if nextLevelDescription != "" {
		base = nextLevelDescription
	}

	maxLevel := skill.MaxLevel()
	switch {
	case currentLevel <= 0:
		return fmt.Sprintf("%s（新技能，1/%d）", base, maxLevel)
	case currentLevel >= maxLevel:
		return fmt.Sprintf("%s（已满级 %d/%d，再次选择不会生效）", base, maxLevel, maxLevel)
	default:
		return fmt.Sprintf("%s（升级到 %d/%d）", base, currentLevel+1, maxLevel)
	}
}
