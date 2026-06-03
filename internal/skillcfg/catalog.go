package skillcfg

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"sync"
)

const (
	defaultEffectsPath = "data/effects.json"
	defaultSkillsPath  = "data/skills.json"
)

//go:embed data/*.json
var embeddedData embed.FS

type EffectTrigger string

const (
	EffectTriggerOnAttackHit EffectTrigger = "on_attack_hit"
	EffectTriggerOnShotFired EffectTrigger = "on_shot_fired"
	EffectTriggerPassive     EffectTrigger = "passive"
	EffectTriggerOnEnemyKill EffectTrigger = "on_enemy_kill"
)

type EffectKind string

const (
	EffectKindApplyStatusOnHit      EffectKind = "apply_status_on_hit"
	EffectKindBonusHomingShotOnFire EffectKind = "bonus_homing_shot_on_fire"
	EffectKindBonusRollCharges      EffectKind = "bonus_roll_charges"
	EffectKindRestoreHealthOnKill   EffectKind = "restore_health_on_kill"
	EffectKindMaxHealthScale        EffectKind = "max_health_scale"
)

type BuffCategory string

const (
	BuffCategoryUnknown BuffCategory = "unknown"
	BuffCategoryDot     BuffCategory = "dot"
	BuffCategoryControl BuffCategory = "control"
)

type BuffStackingRule string

const (
	BuffStackingRuleNone     BuffStackingRule = "none"
	BuffStackingRuleAdditive BuffStackingRule = "additive"
)

type StatusKind string

const (
	StatusKindPoison StatusKind = "poison"
	StatusKindChill  StatusKind = "chill"
)

type DotStatusConfig struct {
	DamagePerTick     int     `json:"damage_per_tick"`
	TickIntervalTicks int     `json:"tick_interval_ticks"`
	DurationSeconds   float64 `json:"duration_seconds"`
	MaxStacks         int     `json:"max_stacks"`
}

type ChillStatusConfig struct {
	MoveSpeedMultiplier float64 `json:"move_speed_multiplier"`
	DurationSeconds     float64 `json:"duration_seconds"`
}

type ApplyStatusOnHitConfig struct {
	Status       StatusKind         `json:"status"`
	Category     BuffCategory       `json:"category"`
	StackingRule BuffStackingRule   `json:"stacking_rule"`
	Dot          *DotStatusConfig   `json:"dot,omitempty"`
	Chill        *ChillStatusConfig `json:"chill,omitempty"`
}

type BonusHomingShotOnFireConfig struct {
	TriggerEveryShots int     `json:"trigger_every_shots"`
	SearchRadius      float64 `json:"search_radius"`
	BulletSpeedScale  float64 `json:"bullet_speed_scale"`
}

type BonusRollChargesConfig struct {
	ExtraCharges int `json:"extra_charges"`
}

type RestoreHealthOnKillConfig struct {
	MaxHealthPercent float64 `json:"max_health_percent"`
	MinHeal          int     `json:"min_heal"`
}

type MaxHealthScaleConfig struct {
	Percent float64 `json:"percent"`
}

// DisplayConfig is client-facing presentation metadata carried by the config
// table. Gameplay logic must not depend on these fields.
type DisplayConfig struct {
	Title       string   `json:"title,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Rarity      string   `json:"rarity,omitempty"`
	SortOrder   int      `json:"sort_order,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// LevelDisplayConfig stores per-level UI text for the client.
type LevelDisplayConfig struct {
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type EffectConfig struct {
	ID                    string                       `json:"id"`
	Name                  string                       `json:"name"`
	Description           string                       `json:"description,omitempty"`
	UI                    *DisplayConfig               `json:"ui,omitempty"`
	Trigger               EffectTrigger                `json:"trigger"`
	Kind                  EffectKind                   `json:"kind"`
	ApplyStatusOnHit      *ApplyStatusOnHitConfig      `json:"apply_status_on_hit,omitempty"`
	BonusHomingShotOnFire *BonusHomingShotOnFireConfig `json:"bonus_homing_shot_on_fire,omitempty"`
	BonusRollCharges      *BonusRollChargesConfig      `json:"bonus_roll_charges,omitempty"`
	RestoreHealthOnKill   *RestoreHealthOnKillConfig   `json:"restore_health_on_kill,omitempty"`
	MaxHealthScale        *MaxHealthScaleConfig        `json:"max_health_scale,omitempty"`
}

type SkillLevelConfig struct {
	Level     int                 `json:"level"`
	EffectIDs []string            `json:"effect_ids"`
	UI        *LevelDisplayConfig `json:"ui,omitempty"`
}

type SkillConfig struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	UI          *DisplayConfig     `json:"ui,omitempty"`
	Levels      []SkillLevelConfig `json:"levels"`
}

// Catalog is the static skill/effect definition set. Runtime buff state and
// runtime skill levels stay on entities so config data remains immutable.
type Catalog struct {
	effectsByID map[string]EffectConfig
	skillsByID  map[string]SkillConfig
}

func LoadCatalogFromFS(fsys fs.FS) (*Catalog, error) {
	return LoadCatalogFromPaths(fsys, defaultEffectsPath, defaultSkillsPath)
}

func LoadCatalogFromPaths(fsys fs.FS, effectsPath, skillsPath string) (*Catalog, error) {
	effectsRaw, err := fs.ReadFile(fsys, effectsPath)
	if err != nil {
		return nil, fmt.Errorf("read effects config: %w", err)
	}
	skillsRaw, err := fs.ReadFile(fsys, skillsPath)
	if err != nil {
		return nil, fmt.Errorf("read skills config: %w", err)
	}

	var effects []EffectConfig
	if err := json.Unmarshal(effectsRaw, &effects); err != nil {
		return nil, fmt.Errorf("decode effects config: %w", err)
	}
	var skills []SkillConfig
	if err := json.Unmarshal(skillsRaw, &skills); err != nil {
		return nil, fmt.Errorf("decode skills config: %w", err)
	}

	catalog := &Catalog{
		effectsByID: make(map[string]EffectConfig, len(effects)),
		skillsByID:  make(map[string]SkillConfig, len(skills)),
	}
	if err := catalog.validateAndBuild(effects, skills); err != nil {
		return nil, err
	}
	return catalog, nil
}

var (
	embeddedCatalogOnce sync.Once
	embeddedCatalog     *Catalog
	embeddedCatalogErr  error
)

func LoadEmbeddedCatalog() (*Catalog, error) {
	embeddedCatalogOnce.Do(func() {
		embeddedCatalog, embeddedCatalogErr = LoadCatalogFromFS(embeddedData)
	})
	return embeddedCatalog, embeddedCatalogErr
}

func MustLoadEmbeddedCatalog() *Catalog {
	catalog, err := LoadEmbeddedCatalog()
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c *Catalog) Effect(id string) (EffectConfig, bool) {
	effect, ok := c.effectsByID[id]
	return effect, ok
}

func (c *Catalog) Skill(id string) (SkillConfig, bool) {
	skill, ok := c.skillsByID[id]
	return skill, ok
}

func (c *Catalog) Skills() []SkillConfig {
	if c == nil || len(c.skillsByID) == 0 {
		return nil
	}

	result := make([]SkillConfig, 0, len(c.skillsByID))
	for _, skill := range c.skillsByID {
		result = append(result, skill)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ResolveSkill keeps backward compatibility for old call sites and returns the
// level-1 effect set of the skill.
func (c *Catalog) ResolveSkill(id string) ([]EffectConfig, bool) {
	return c.ResolveSkillLevel(id, 1)
}

func (c *Catalog) ResolveSkillLevel(id string, level int) ([]EffectConfig, bool) {
	skill, ok := c.skillsByID[id]
	if !ok {
		return nil, false
	}

	levelCfg, ok := skill.LevelConfig(level)
	if !ok {
		return nil, false
	}

	effects := make([]EffectConfig, 0, len(levelCfg.EffectIDs))
	for _, effectID := range levelCfg.EffectIDs {
		effect, ok := c.effectsByID[effectID]
		if !ok {
			return nil, false
		}
		effects = append(effects, effect)
	}
	return effects, true
}

func (s SkillConfig) MaxLevel() int {
	return len(s.Levels)
}

func (s SkillConfig) LevelConfig(level int) (SkillLevelConfig, bool) {
	if len(s.Levels) == 0 {
		return SkillLevelConfig{}, false
	}
	if level < 1 {
		level = 1
	}
	if level > len(s.Levels) {
		level = len(s.Levels)
	}
	for _, levelCfg := range s.Levels {
		if levelCfg.Level == level {
			return levelCfg, true
		}
	}
	return SkillLevelConfig{}, false
}

func (c *Catalog) validateAndBuild(effects []EffectConfig, skills []SkillConfig) error {
	for _, effect := range effects {
		if err := validateEffect(effect); err != nil {
			return fmt.Errorf("effect %q: %w", effect.ID, err)
		}
		if _, exists := c.effectsByID[effect.ID]; exists {
			return fmt.Errorf("duplicate effect id %q", effect.ID)
		}
		c.effectsByID[effect.ID] = effect
	}

	for _, skill := range skills {
		if err := validateSkill(skill, c.effectsByID); err != nil {
			return fmt.Errorf("skill %q: %w", skill.ID, err)
		}
		if _, exists := c.skillsByID[skill.ID]; exists {
			return fmt.Errorf("duplicate skill id %q", skill.ID)
		}
		c.skillsByID[skill.ID] = skill
	}
	return nil
}

func validateEffect(effect EffectConfig) error {
	if effect.ID == "" {
		return fmt.Errorf("id is required")
	}
	if effect.Name == "" {
		return fmt.Errorf("name is required")
	}

	switch effect.Kind {
	case EffectKindApplyStatusOnHit:
		if effect.Trigger != EffectTriggerOnAttackHit {
			return fmt.Errorf("apply_status_on_hit must use trigger %q", EffectTriggerOnAttackHit)
		}
		if effect.ApplyStatusOnHit == nil {
			return fmt.Errorf("apply_status_on_hit is required")
		}
		if err := validateExclusiveEffectPayloads(effect, "apply_status_on_hit"); err != nil {
			return err
		}
		return validateApplyStatusOnHit(*effect.ApplyStatusOnHit)
	case EffectKindBonusHomingShotOnFire:
		if effect.Trigger != EffectTriggerOnShotFired {
			return fmt.Errorf("bonus_homing_shot_on_fire must use trigger %q", EffectTriggerOnShotFired)
		}
		if effect.BonusHomingShotOnFire == nil {
			return fmt.Errorf("bonus_homing_shot_on_fire is required")
		}
		if err := validateExclusiveEffectPayloads(effect, "bonus_homing_shot_on_fire"); err != nil {
			return err
		}
		return validateBonusHomingShotOnFire(*effect.BonusHomingShotOnFire)
	case EffectKindBonusRollCharges:
		if effect.Trigger != EffectTriggerPassive {
			return fmt.Errorf("bonus_roll_charges must use trigger %q", EffectTriggerPassive)
		}
		if effect.BonusRollCharges == nil {
			return fmt.Errorf("bonus_roll_charges is required")
		}
		if err := validateExclusiveEffectPayloads(effect, "bonus_roll_charges"); err != nil {
			return err
		}
		return validateBonusRollCharges(*effect.BonusRollCharges)
	case EffectKindRestoreHealthOnKill:
		if effect.Trigger != EffectTriggerOnEnemyKill {
			return fmt.Errorf("restore_health_on_kill must use trigger %q", EffectTriggerOnEnemyKill)
		}
		if effect.RestoreHealthOnKill == nil {
			return fmt.Errorf("restore_health_on_kill is required")
		}
		if err := validateExclusiveEffectPayloads(effect, "restore_health_on_kill"); err != nil {
			return err
		}
		return validateRestoreHealthOnKill(*effect.RestoreHealthOnKill)
	case EffectKindMaxHealthScale:
		if effect.Trigger != EffectTriggerPassive {
			return fmt.Errorf("max_health_scale must use trigger %q", EffectTriggerPassive)
		}
		if effect.MaxHealthScale == nil {
			return fmt.Errorf("max_health_scale is required")
		}
		if err := validateExclusiveEffectPayloads(effect, "max_health_scale"); err != nil {
			return err
		}
		return validateMaxHealthScale(*effect.MaxHealthScale)
	default:
		return fmt.Errorf("unsupported kind %q", effect.Kind)
	}
}

func validateExclusiveEffectPayloads(effect EffectConfig, active string) error {
	if active != "apply_status_on_hit" && effect.ApplyStatusOnHit != nil {
		return fmt.Errorf("apply_status_on_hit must be empty for %s", active)
	}
	if active != "bonus_homing_shot_on_fire" && effect.BonusHomingShotOnFire != nil {
		return fmt.Errorf("bonus_homing_shot_on_fire must be empty for %s", active)
	}
	if active != "bonus_roll_charges" && effect.BonusRollCharges != nil {
		return fmt.Errorf("bonus_roll_charges must be empty for %s", active)
	}
	if active != "restore_health_on_kill" && effect.RestoreHealthOnKill != nil {
		return fmt.Errorf("restore_health_on_kill must be empty for %s", active)
	}
	if active != "max_health_scale" && effect.MaxHealthScale != nil {
		return fmt.Errorf("max_health_scale must be empty for %s", active)
	}
	return nil
}

func validateApplyStatusOnHit(cfg ApplyStatusOnHitConfig) error {
	switch cfg.Status {
	case StatusKindPoison:
		if cfg.Dot == nil {
			return fmt.Errorf("dot payload is required")
		}
		if cfg.Chill != nil {
			return fmt.Errorf("chill payload must be empty for poison status")
		}
		if cfg.Category != BuffCategoryDot {
			return fmt.Errorf("poison status must use category %q", BuffCategoryDot)
		}
		if cfg.StackingRule != BuffStackingRuleAdditive {
			return fmt.Errorf("poison status must use stacking rule %q", BuffStackingRuleAdditive)
		}
		if cfg.Dot.DamagePerTick <= 0 {
			return fmt.Errorf("damage_per_tick must be > 0")
		}
		if cfg.Dot.TickIntervalTicks <= 0 {
			return fmt.Errorf("tick_interval_ticks must be > 0")
		}
		if cfg.Dot.DurationSeconds <= 0 {
			return fmt.Errorf("duration_seconds must be > 0")
		}
		if cfg.Dot.MaxStacks <= 0 {
			return fmt.Errorf("max_stacks must be > 0")
		}
	case StatusKindChill:
		if cfg.Chill == nil {
			return fmt.Errorf("chill payload is required")
		}
		if cfg.Dot != nil {
			return fmt.Errorf("dot payload must be empty for chill status")
		}
		if cfg.Category != BuffCategoryControl {
			return fmt.Errorf("chill status must use category %q", BuffCategoryControl)
		}
		if cfg.StackingRule != BuffStackingRuleNone {
			return fmt.Errorf("chill status must use stacking rule %q", BuffStackingRuleNone)
		}
		if cfg.Chill.DurationSeconds <= 0 {
			return fmt.Errorf("duration_seconds must be > 0")
		}
		if cfg.Chill.MoveSpeedMultiplier < 0 || cfg.Chill.MoveSpeedMultiplier >= 1 {
			return fmt.Errorf("move_speed_multiplier must be in [0,1)")
		}
	default:
		return fmt.Errorf("unsupported status %q", cfg.Status)
	}
	return nil
}

func validateBonusHomingShotOnFire(cfg BonusHomingShotOnFireConfig) error {
	if cfg.TriggerEveryShots <= 0 {
		return fmt.Errorf("trigger_every_shots must be > 0")
	}
	if cfg.SearchRadius <= 0 {
		return fmt.Errorf("search_radius must be > 0")
	}
	if cfg.BulletSpeedScale <= 0 {
		return fmt.Errorf("bullet_speed_scale must be > 0")
	}
	return nil
}

func validateBonusRollCharges(cfg BonusRollChargesConfig) error {
	if cfg.ExtraCharges <= 0 {
		return fmt.Errorf("extra_charges must be > 0")
	}
	return nil
}

func validateRestoreHealthOnKill(cfg RestoreHealthOnKillConfig) error {
	if cfg.MaxHealthPercent <= 0 {
		return fmt.Errorf("max_health_percent must be > 0")
	}
	if cfg.MinHeal <= 0 {
		return fmt.Errorf("min_heal must be > 0")
	}
	return nil
}

func validateMaxHealthScale(cfg MaxHealthScaleConfig) error {
	if cfg.Percent <= 0 {
		return fmt.Errorf("percent must be > 0")
	}
	return nil
}

func validateSkill(skill SkillConfig, effectsByID map[string]EffectConfig) error {
	if skill.ID == "" {
		return fmt.Errorf("id is required")
	}
	if skill.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(skill.Levels) == 0 {
		return fmt.Errorf("levels must not be empty")
	}

	seenLevels := make(map[int]struct{}, len(skill.Levels))
	for i, levelCfg := range skill.Levels {
		if levelCfg.Level != i+1 {
			return fmt.Errorf("levels must be contiguous and start at 1")
		}
		if _, exists := seenLevels[levelCfg.Level]; exists {
			return fmt.Errorf("duplicate level %d", levelCfg.Level)
		}
		seenLevels[levelCfg.Level] = struct{}{}
		if len(levelCfg.EffectIDs) == 0 {
			return fmt.Errorf("level %d effect_ids must not be empty", levelCfg.Level)
		}

		seenEffects := make(map[string]struct{}, len(levelCfg.EffectIDs))
		for _, effectID := range levelCfg.EffectIDs {
			if effectID == "" {
				return fmt.Errorf("level %d effect_ids contains empty id", levelCfg.Level)
			}
			if _, exists := effectsByID[effectID]; !exists {
				return fmt.Errorf("level %d references unknown effect %q", levelCfg.Level, effectID)
			}
			if _, duplicate := seenEffects[effectID]; duplicate {
				return fmt.Errorf("level %d duplicate effect reference %q", levelCfg.Level, effectID)
			}
			seenEffects[effectID] = struct{}{}
		}
	}
	return nil
}
