package skillcfg

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadEmbeddedCatalog(t *testing.T) {
	catalog, err := LoadEmbeddedCatalog()
	if err != nil {
		t.Fatalf("expected embedded catalog to load, got error: %v", err)
	}

	skill, ok := catalog.Skill("skill_venom_frost_rounds")
	if !ok {
		t.Fatal("expected combined skill to exist")
	}
	if got := skill.MaxLevel(); got != 3 {
		t.Fatalf("expected max level 3, got %d", got)
	}

	effects, ok := catalog.ResolveSkillLevel(skill.ID, 2)
	if !ok {
		t.Fatal("expected level 2 combined skill to resolve")
	}
	if len(effects) != 2 {
		t.Fatalf("expected 2 resolved effects at level 2, got %d", len(effects))
	}

	homingEffects, ok := catalog.ResolveSkillLevel("skill_homing_rounds_1", 3)
	if !ok || len(homingEffects) != 1 {
		t.Fatal("expected homing skill level 3 to resolve")
	}
	if homingEffects[0].BonusHomingShotOnFire == nil {
		t.Fatal("expected homing skill effect payload")
	}
	if got := homingEffects[0].BonusHomingShotOnFire.TriggerEveryShots; got != 3 {
		t.Fatalf("expected homing trigger every 3 shots, got %d", got)
	}

	poisonEffect, ok := catalog.Effect("effect_poison_on_hit_l1")
	if !ok {
		t.Fatal("expected poison effect to exist")
	}
	if poisonEffect.UI == nil || poisonEffect.UI.Icon == "" {
		t.Fatal("expected poison effect ui metadata")
	}

	homingSkill, ok := catalog.Skill("skill_homing_rounds_1")
	if !ok {
		t.Fatal("expected homing skill to exist")
	}
	if homingSkill.UI == nil || homingSkill.UI.Title == "" {
		t.Fatal("expected homing skill ui metadata")
	}
	level3, ok := homingSkill.LevelConfig(3)
	if !ok {
		t.Fatal("expected homing skill level 3 config")
	}
	if level3.UI == nil || level3.UI.Description == "" {
		t.Fatal("expected homing skill level ui metadata")
	}

	rollReserve, ok := catalog.Skill("skill_roll_reserve")
	if !ok {
		t.Fatal("expected roll reserve skill to exist")
	}
	if got := rollReserve.MaxLevel(); got != 3 {
		t.Fatalf("expected roll reserve max level 3, got %d", got)
	}

	killHealEffects, ok := catalog.ResolveSkillLevel("skill_kill_heal", 2)
	if !ok || len(killHealEffects) != 1 {
		t.Fatal("expected kill-heal skill level 2 to resolve")
	}
	if killHealEffects[0].RestoreHealthOnKill == nil {
		t.Fatal("expected kill-heal effect payload")
	}
	if got := killHealEffects[0].RestoreHealthOnKill.MaxHealthPercent; got != 0.01 {
		t.Fatalf("expected kill-heal level 2 percent 0.01, got %f", got)
	}

	vitalityEffects, ok := catalog.ResolveSkillLevel("skill_vitality_boost", 3)
	if !ok || len(vitalityEffects) != 1 {
		t.Fatal("expected vitality skill level 3 to resolve")
	}
	if vitalityEffects[0].MaxHealthScale == nil {
		t.Fatal("expected vitality effect payload")
	}
	if got := vitalityEffects[0].MaxHealthScale.Percent; got != 0.5 {
		t.Fatalf("expected vitality level 3 percent 0.5, got %f", got)
	}
}

func TestLoadCatalogRejectsUnknownEffectReference(t *testing.T) {
	fsys := fstest.MapFS{
		defaultEffectsPath: &fstest.MapFile{Data: []byte(`[
			{
				"id":"effect_poison",
				"name":"毒伤",
				"trigger":"on_attack_hit",
				"kind":"apply_status_on_hit",
				"apply_status_on_hit":{
					"status":"poison",
					"category":"dot",
					"stacking_rule":"additive",
					"dot":{
						"damage_per_tick":1,
						"tick_interval_ticks":10,
						"duration_seconds":3,
						"max_stacks":3
					}
				}
			}
		]`)},
		defaultSkillsPath: &fstest.MapFile{Data: []byte(`[
			{
				"id":"skill_bad_ref",
				"name":"坏引用",
				"levels":[
					{"level":1,"effect_ids":["effect_missing"]}
				]
			}
		]`)},
	}

	_, err := LoadCatalogFromFS(fsys)
	if err == nil {
		t.Fatal("expected config loading to fail for unknown effect reference")
	}
	if !strings.Contains(err.Error(), "unknown effect") {
		t.Fatalf("expected unknown effect error, got: %v", err)
	}
}

func TestLoadCatalogRejectsInvalidPoisonPayload(t *testing.T) {
	fsys := fstest.MapFS{
		defaultEffectsPath: &fstest.MapFile{Data: []byte(`[
			{
				"id":"effect_poison",
				"name":"毒伤",
				"trigger":"on_attack_hit",
				"kind":"apply_status_on_hit",
				"apply_status_on_hit":{
					"status":"poison",
					"category":"dot",
					"stacking_rule":"additive",
					"dot":{
						"damage_per_tick":0,
						"tick_interval_ticks":10,
						"duration_seconds":3,
						"max_stacks":3
					}
				}
			}
		]`)},
		defaultSkillsPath: &fstest.MapFile{Data: []byte(`[
			{
				"id":"skill_ok",
				"name":"正常技能",
				"levels":[
					{"level":1,"effect_ids":["effect_poison"]}
				]
			}
		]`)},
	}

	_, err := LoadCatalogFromFS(fsys)
	if err == nil {
		t.Fatal("expected poison validation to fail")
	}
	if !strings.Contains(err.Error(), "damage_per_tick") {
		t.Fatalf("expected poison validation error, got: %v", err)
	}
}
