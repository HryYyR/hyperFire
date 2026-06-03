package gameplay

import (
	"math"
	"testing"
	"time"

	"agentDemo/internal/netproto"
	"agentDemo/internal/session"
	"agentDemo/internal/skillcfg"

	"github.com/mlange-42/arche/ecs"
	"github.com/mlange-42/arche/generic"
)

func TestPlayerDeathDropsHalfExperienceAndRespawnClearsRemainder(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	health := (*Health)(runtime.world.Get(entity, runtime.ids.health))

	experience.Value = 10
	health.Value = 0

	runtime.cleanupDeadLocked()

	if got := experience.Value; got != 5 {
		t.Fatalf("expected remaining experience to be 5 after death drop, got %d", got)
	}

	if ok := runtime.RespawnPlayer(playerID); !ok {
		t.Fatal("expected respawn to succeed")
	}

	if got := experience.Value; got != 0 {
		t.Fatalf("expected experience to reset to 0 after respawn, got %d", got)
	}
}

func TestPlayerDeathDropHappensOnlyOnce(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	health := (*Health)(runtime.world.Get(entity, runtime.ids.health))

	experience.Value = 12
	health.Value = 0

	runtime.cleanupDeadLocked()
	firstRemaining := experience.Value

	runtime.cleanupDeadLocked()
	secondRemaining := experience.Value

	if firstRemaining != 6 {
		t.Fatalf("expected first death drop to leave 6 exp, got %d", firstRemaining)
	}
	if secondRemaining != firstRemaining {
		t.Fatalf("expected second cleanup to keep exp at %d, got %d", firstRemaining, secondRemaining)
	}
}

func TestLevelProgressionQueuesPendingChoicesInOrder(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	level3, ok := runtime.levels.Entry(3)
	if !ok {
		t.Fatal("expected level 3 to exist in level table")
	}

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	playerLevel := (*PlayerLevel)(runtime.world.Get(entity, runtime.ids.playerLevel))
	pending := (*PendingSkillChoices)(runtime.world.Get(entity, runtime.ids.pendingSkillChoices))

	experience.Value = level3.RequiredExp
	runtime.updateLevelProgressionLocked()

	if got, want := playerLevel.Value, uint32(3); got != want {
		t.Fatalf("expected player level %d, got %d", want, got)
	}
	if got := len(pending.Queue); got != 2 {
		t.Fatalf("expected 2 pending choices after jumping to level 3, got %d", got)
	}
	if pending.Queue[0].Sequence != 1 || pending.Queue[0].TargetLevel != 2 {
		t.Fatalf("expected first queued choice to target level 2 with sequence 1, got seq=%d level=%d", pending.Queue[0].Sequence, pending.Queue[0].TargetLevel)
	}
	if pending.Queue[1].Sequence != 2 || pending.Queue[1].TargetLevel != 3 {
		t.Fatalf("expected second queued choice to target level 3 with sequence 2, got seq=%d level=%d", pending.Queue[1].Sequence, pending.Queue[1].TargetLevel)
	}
	for i, choice := range pending.Queue {
		if got := len(choice.SkillOptions); got != 3 {
			t.Fatalf("expected queued choice %d to offer 3 random skills, got %d", i, got)
		}
		if hasDuplicateStrings(choice.SkillOptions) {
			t.Fatalf("expected queued choice %d to avoid duplicate skill options, got %v", i, choice.SkillOptions)
		}
	}
}

func TestChoosePlayerSkillConsumesQueueSequentially(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	level3, ok := runtime.levels.Entry(3)
	if !ok {
		t.Fatal("expected level 3 to exist in level table")
	}

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	inventory := (*SkillInventory)(runtime.world.Get(entity, runtime.ids.skillInventory))
	pending := (*PendingSkillChoices)(runtime.world.Get(entity, runtime.ids.pendingSkillChoices))

	experience.Value = level3.RequiredExp
	runtime.updateLevelProgressionLocked()

	outOfOrder := runtime.ChoosePlayerSkill(playerID, 2, pending.Queue[1].SkillOptions[0])
	if outOfOrder.Accepted {
		t.Fatal("expected out-of-order skill selection to be rejected")
	}

	firstChoiceSkill := pending.Queue[0].SkillOptions[0]
	firstResult := runtime.ChoosePlayerSkill(playerID, 1, firstChoiceSkill)
	if !firstResult.Accepted || !firstResult.Granted {
		t.Fatalf("expected first skill choice to be accepted and granted, got accepted=%v granted=%v message=%q", firstResult.Accepted, firstResult.Granted, firstResult.Message)
	}
	if got := len(inventory.Skills); got != 1 {
		t.Fatalf("expected 1 skill after first selection, got %d", got)
	}
	if got := inventory.Skills[0].Level; got != 1 {
		t.Fatalf("expected first selection to grant level 1, got %d", got)
	}

	secondChoiceSkill := pending.Queue[0].SkillOptions[0]
	secondResult := runtime.ChoosePlayerSkill(playerID, 2, secondChoiceSkill)
	if !secondResult.Accepted || !secondResult.Granted {
		t.Fatalf("expected second skill choice to be accepted and granted, got accepted=%v granted=%v message=%q", secondResult.Accepted, secondResult.Granted, secondResult.Message)
	}
	if got := len(pending.Queue); got != 0 {
		t.Fatalf("expected all pending choices to be consumed, got %d", got)
	}
}

func TestChoosePlayerSkillCanUpgradeSameSkillAcrossChoices(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]
	inventory := (*SkillInventory)(runtime.world.Get(entity, runtime.ids.skillInventory))
	pending := (*PendingSkillChoices)(runtime.world.Get(entity, runtime.ids.pendingSkillChoices))

	pending.NextSequence = 3
	pending.Queue = []PendingSkillChoice{{
		Sequence:     1,
		TargetLevel:  2,
		SkillOptions: []string{"skill_toxic_rounds_1"},
	}}

	firstResult := runtime.ChoosePlayerSkill(playerID, 1, "skill_toxic_rounds_1")
	if !firstResult.Accepted || !firstResult.Granted {
		t.Fatalf("expected first upgrade choice to succeed, got accepted=%v granted=%v message=%q", firstResult.Accepted, firstResult.Granted, firstResult.Message)
	}

	pending.Queue = []PendingSkillChoice{{
		Sequence:     2,
		TargetLevel:  3,
		SkillOptions: []string{"skill_toxic_rounds_1"},
	}}

	secondResult := runtime.ChoosePlayerSkill(playerID, 2, "skill_toxic_rounds_1")
	if !secondResult.Accepted || !secondResult.Granted {
		t.Fatalf("expected duplicate upgrade choice to succeed, got accepted=%v granted=%v message=%q", secondResult.Accepted, secondResult.Granted, secondResult.Message)
	}
	if got := len(inventory.Skills); got != 1 {
		t.Fatalf("expected duplicate upgrade to keep one skill entry, got %d", got)
	}
	if got := inventory.Skills[0].Level; got != 2 {
		t.Fatalf("expected duplicate upgrade to raise skill to level 2, got %d", got)
	}
}

func TestGrantPlayerSkillStopsGrantingPastMaxLevel(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]
	inventory := (*SkillInventory)(runtime.world.Get(entity, runtime.ids.skillInventory))

	for i := 0; i < 3; i++ {
		valid, granted := runtime.tryGrantSkillLocked(entity, "skill_toxic_rounds_1")
		if !valid || !granted {
			t.Fatalf("expected grant %d to succeed, valid=%v granted=%v", i+1, valid, granted)
		}
	}

	valid, granted := runtime.tryGrantSkillLocked(entity, "skill_toxic_rounds_1")
	if !valid {
		t.Fatal("expected max-level grant to still be treated as a valid skill choice")
	}
	if granted {
		t.Fatal("expected max-level grant to have no effect")
	}
	if got := inventory.Skills[0].Level; got != 3 {
		t.Fatalf("expected skill level to stay at max level 3, got %d", got)
	}
}

func TestLevelUpChoiceExcludesMaxLevelSkills(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	for i := 0; i < 3; i++ {
		valid, granted := runtime.tryGrantSkillLocked(entity, "skill_toxic_rounds_1")
		if !valid || !granted {
			t.Fatalf("expected toxic skill grant %d to succeed, valid=%v granted=%v", i+1, valid, granted)
		}
	}

	options := runtime.rollLevelUpSkillOptionsLocked(entity, 3)
	if got := len(options); got != 3 {
		t.Fatalf("expected 3 available random options, got %d", got)
	}
	if containsString(options, "skill_toxic_rounds_1") {
		t.Fatalf("expected maxed skill to be excluded from options, got %v", options)
	}
}

func TestPlayerDeathClearsSkillsLevelsAndPendingChoices(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	level2, ok := runtime.levels.Entry(2)
	if !ok {
		t.Fatal("expected level 2 to exist in level table")
	}

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	health := (*Health)(runtime.world.Get(entity, runtime.ids.health))
	playerLevel := (*PlayerLevel)(runtime.world.Get(entity, runtime.ids.playerLevel))
	inventory := (*SkillInventory)(runtime.world.Get(entity, runtime.ids.skillInventory))
	pending := (*PendingSkillChoices)(runtime.world.Get(entity, runtime.ids.pendingSkillChoices))

	experience.Value = level2.RequiredExp
	runtime.updateLevelProgressionLocked()
	if _, granted := runtime.tryGrantSkillLocked(entity, "skill_toxic_rounds_1"); !granted {
		t.Fatal("expected direct skill grant to succeed")
	}

	health.Value = 0
	runtime.cleanupDeadLocked()

	if got, want := playerLevel.Value, runtime.levels.BaseLevel(); got != want {
		t.Fatalf("expected player level reset to %d on death, got %d", want, got)
	}
	if got := len(inventory.Skills); got != 0 {
		t.Fatalf("expected skills to be cleared on death, got %d", got)
	}
	if got := len(pending.Queue); got != 0 {
		t.Fatalf("expected pending choices to clear on death, got %d", got)
	}
}

func TestRollReserveSkillIncreasesPlayerRollCharges(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]
	rollStats := (*RollStats)(runtime.world.Get(entity, runtime.ids.rollStats))
	rollState := (*RollState)(runtime.world.Get(entity, runtime.ids.rollState))

	if got := rollStats.MaxCharges; got != runtime.cfg.PlayerRollMaxCharges {
		t.Fatalf("expected base roll charges %d, got %d", runtime.cfg.PlayerRollMaxCharges, got)
	}

	valid, granted := runtime.tryGrantSkillLocked(entity, "skill_roll_reserve")
	if !valid || !granted {
		t.Fatalf("expected roll reserve grant to succeed, valid=%v granted=%v", valid, granted)
	}
	if got := rollStats.MaxCharges; got != runtime.cfg.PlayerRollMaxCharges+1 {
		t.Fatalf("expected roll charges to increase to %d, got %d", runtime.cfg.PlayerRollMaxCharges+1, got)
	}
	if got := rollState.Charges; got != rollStats.MaxCharges {
		t.Fatalf("expected current roll charges to refill to new max %d, got %d", rollStats.MaxCharges, got)
	}
}

func TestVitalityBoostRaisesPlayerMaxHealth(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]
	health := (*Health)(runtime.world.Get(entity, runtime.ids.health))

	if got := runtime.playerMaxHealthLocked(entity); got != runtime.cfg.PlayerHealth {
		t.Fatalf("expected base max health %d, got %d", runtime.cfg.PlayerHealth, got)
	}

	valid, granted := runtime.tryGrantSkillLocked(entity, "skill_vitality_boost")
	if !valid || !granted {
		t.Fatalf("expected vitality boost grant to succeed, valid=%v granted=%v", valid, granted)
	}
	if got, want := runtime.playerMaxHealthLocked(entity), 110; got != want {
		t.Fatalf("expected vitality boost max health %d, got %d", want, got)
	}
	if got := health.Value; got != 110 {
		t.Fatalf("expected full-health player to remain full at 110 hp, got %d", got)
	}
}

func TestSnapshotExportsPlayerMaxHealth(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	valid, granted := runtime.tryGrantSkillLocked(entity, "skill_vitality_boost")
	if !valid || !granted {
		t.Fatalf("expected vitality boost grant to succeed, valid=%v granted=%v", valid, granted)
	}

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	for _, state := range snapshot.Entities {
		if state.Kind != netproto.EntityKind_ENTITY_KIND_PLAYER || state.OwnerPlayerId != playerID {
			continue
		}
		if got, want := state.HpMax, int32(110); got != want {
			t.Fatalf("expected exported player hp_max %d, got %d", want, got)
		}
		if got, want := state.Hp, int32(110); got != want {
			t.Fatalf("expected exported player hp %d, got %d", want, got)
		}
		return
	}

	t.Fatal("expected snapshot to include player entity")
}

func TestKillHealSkillRestoresHealthFromEnemyKill(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	health := (*Health)(runtime.world.Get(player, runtime.ids.health))
	health.Value = 40

	valid, granted := runtime.tryGrantSkillLocked(player, "skill_kill_heal")
	if !valid || !granted {
		t.Fatalf("expected kill-heal skill grant to succeed, valid=%v granted=%v", valid, granted)
	}

	runtime.spawnEnemyLocked()
	enemy := firstRawEnemyEntity(t, runtime)
	enemyHealth := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	lastHit := (*LastHitByPlayerID)(runtime.world.Get(enemy, runtime.ids.lastHitByPlayerID))
	enemyHealth.Value = 0
	lastHit.Value = playerID

	runtime.cleanupDeadLocked()

	if got := health.Value; got != 41 {
		t.Fatalf("expected kill-heal skill to restore minimum 1 hp, got %d", got)
	}
}

func TestPlayerDeathSnapshotResetsPlayerLevelImmediately(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	level3, ok := runtime.levels.Entry(3)
	if !ok {
		t.Fatal("expected level 3 to exist in level table")
	}

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	health := (*Health)(runtime.world.Get(entity, runtime.ids.health))

	experience.Value = level3.RequiredExp
	runtime.updateLevelProgressionLocked()
	health.Value = 0

	runtime.cleanupDeadLocked()

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if got, want := snapshot.PlayerLevel, runtime.levels.BaseLevel(); got != want {
		t.Fatalf("expected snapshot player level reset to %d after death, got %d", want, got)
	}
	if got := len(snapshot.PendingSkillChoices); got != 0 {
		t.Fatalf("expected pending choices to clear from death snapshot, got %d", got)
	}
}

func TestRespawnSnapshotKeepsBasePlayerLevel(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	level2, ok := runtime.levels.Entry(2)
	if !ok {
		t.Fatal("expected level 2 to exist in level table")
	}

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	health := (*Health)(runtime.world.Get(entity, runtime.ids.health))

	experience.Value = level2.RequiredExp
	runtime.updateLevelProgressionLocked()
	health.Value = 0
	runtime.cleanupDeadLocked()

	if ok := runtime.RespawnPlayer(playerID); !ok {
		t.Fatal("expected respawn to succeed")
	}

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if got, want := snapshot.PlayerLevel, runtime.levels.BaseLevel(); got != want {
		t.Fatalf("expected respawn snapshot player level %d, got %d", want, got)
	}
	if got := snapshot.Score; got != 0 {
		t.Fatalf("expected respawn snapshot score 0 after experience reset, got %d", got)
	}
}

func TestSnapshotExportsPlayerLevelAndPendingChoices(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]

	level2, ok := runtime.levels.Entry(2)
	if !ok {
		t.Fatal("expected level 2 to exist in level table")
	}

	experience := (*Experience)(runtime.world.Get(entity, runtime.ids.experience))
	experience.Value = level2.RequiredExp
	runtime.updateLevelProgressionLocked()

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if got, want := snapshot.PlayerLevel, uint32(2); got != want {
		t.Fatalf("expected snapshot player level %d, got %d", want, got)
	}
	if got := len(snapshot.PendingSkillChoices); got != 1 {
		t.Fatalf("expected 1 pending skill choice in snapshot, got %d", got)
	}
	if snapshot.PendingSkillChoices[0].Sequence != 1 {
		t.Fatalf("expected pending choice sequence 1, got %d", snapshot.PendingSkillChoices[0].Sequence)
	}
	if len(snapshot.PendingSkillChoices[0].Options) == 0 {
		t.Fatal("expected pending choice to include exported skill options")
	}
}

func TestBladeEnemyMeleeAttackDamagesNearbyPlayer(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyLevel := (*EnemyLevel)(runtime.world.Get(enemy, runtime.ids.enemyLevel))
	fireCooldown := (*FireCooldown)(runtime.world.Get(enemy, runtime.ids.fireCooldown))

	enemyPos.X = 21
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassBlade
	enemyLevel.Value = 1
	fireCooldown.Frames = 0
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))

	runtime.updateEnemyMeleeLocked()

	health := (*Health)(runtime.world.Get(player, runtime.ids.health))
	if got, want := health.Value, runtime.cfg.PlayerHealth-runtime.enemyBladeDamageByLevelLocked(enemyTier.Value, enemyLevel.Value); got != want {
		t.Fatalf("expected player hp %d after blade hit, got %d", want, got)
	}
}

func TestEnemySpawnAppliesLevelScaledHealthAndCooldown(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyBaseLevel = 3
	cfg.EnemyMaxLevel = 3
	cfg.EnemyLowHealth = 40
	cfg.EnemyHighHealth = 40
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	level := (*EnemyLevel)(runtime.world.Get(enemy, runtime.ids.enemyLevel))
	class := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	tier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	health := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	fireCooldown := (*FireCooldown)(runtime.world.Get(enemy, runtime.ids.fireCooldown))

	if got, want := health.Value, runtime.enemyHealthByLevelLocked(tier.Value, level.Value, cfg.EnemyLowHealth); got != want {
		t.Fatalf("expected spawned enemy health %d, got %d", want, got)
	}
	if got, want := fireCooldown.Frames, runtime.enemyAttackCooldownFramesLocked(tier.Value, class.Value, level.Value); got != want {
		t.Fatalf("expected spawned enemy cooldown %d, got %d", want, got)
	}
}

func TestEnemyLevelScalingShortensCooldownAtHigherLevel(t *testing.T) {
	runtime := NewRuntime(60)

	bladeLow := runtime.enemyAttackCooldownFramesLocked(EnemyTierMinion, EnemyClassBlade, 1)
	bladeHigh := runtime.enemyAttackCooldownFramesLocked(EnemyTierMinion, EnemyClassBlade, 4)
	gunnerLow := runtime.enemyAttackCooldownFramesLocked(EnemyTierMinion, EnemyClassGunner, 1)
	gunnerHigh := runtime.enemyAttackCooldownFramesLocked(EnemyTierMinion, EnemyClassGunner, 4)

	if bladeHigh >= bladeLow {
		t.Fatalf("expected high-level blade cooldown %d to be lower than level-1 cooldown %d", bladeHigh, bladeLow)
	}
	if gunnerHigh >= gunnerLow {
		t.Fatalf("expected high-level gunner cooldown %d to be lower than level-1 cooldown %d", gunnerHigh, gunnerLow)
	}
}

func TestEnemyBladeDamageScalesWithLevel(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyLevel := (*EnemyLevel)(runtime.world.Get(enemy, runtime.ids.enemyLevel))
	fireCooldown := (*FireCooldown)(runtime.world.Get(enemy, runtime.ids.fireCooldown))
	playerHealth := (*Health)(runtime.world.Get(player, runtime.ids.health))

	enemyPos.X = 21
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassBlade
	enemyLevel.Value = 4
	fireCooldown.Frames = 0
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))

	beforeHP := playerHealth.Value
	runtime.updateEnemyMeleeLocked()

	if got, want := beforeHP-playerHealth.Value, runtime.enemyBladeDamageByLevelLocked(enemyTier.Value, enemyLevel.Value); got != want {
		t.Fatalf("expected blade level-scaled damage %d, got %d", want, got)
	}
}

func TestEnemyFireUsesLevelScaledDamageAndCooldown(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyLevel := (*EnemyLevel)(runtime.world.Get(enemy, runtime.ids.enemyLevel))
	fireCooldown := (*FireCooldown)(runtime.world.Get(enemy, runtime.ids.fireCooldown))

	enemyPos.X = 10
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassGunner
	enemyLevel.Value = 4
	fireCooldown.Frames = 0
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))

	runtime.updateEnemyFireLocked()

	if got := countEnemyBullets(runtime); got != 1 {
		t.Fatalf("expected 1 enemy bullet, got %d", got)
	}
	bullet := firstEnemyBulletEntity(t, runtime)
	damage := (*Damage)(runtime.world.Get(bullet, runtime.ids.damage))

	if got, want := damage.Value, runtime.enemyBulletDamageByLevelLocked(enemyTier.Value, enemyLevel.Value); got != want {
		t.Fatalf("expected enemy bullet damage %d, got %d", want, got)
	}
	if got, want := fireCooldown.Frames, runtime.enemyAttackCooldownFramesLocked(enemyTier.Value, enemyClass.Value, enemyLevel.Value); got != want {
		t.Fatalf("expected enemy fire cooldown %d, got %d", want, got)
	}
}

func TestEnemyTierScalingBoostsEliteAndBossStats(t *testing.T) {
	runtime := NewRuntime(60)
	const level uint32 = 4
	const baseHealth = 50

	minionHealth := runtime.enemyHealthByLevelLocked(EnemyTierMinion, level, baseHealth)
	eliteHealth := runtime.enemyHealthByLevelLocked(EnemyTierElite, level, baseHealth)
	bossHealth := runtime.enemyHealthByLevelLocked(EnemyTierBoss, level, baseHealth)
	minionBladeDamage := runtime.enemyBladeDamageByLevelLocked(EnemyTierMinion, level)
	eliteBladeDamage := runtime.enemyBladeDamageByLevelLocked(EnemyTierElite, level)
	bossBladeDamage := runtime.enemyBladeDamageByLevelLocked(EnemyTierBoss, level)
	minionCooldown := runtime.enemyAttackCooldownFramesLocked(EnemyTierMinion, EnemyClassGunner, level)
	eliteCooldown := runtime.enemyAttackCooldownFramesLocked(EnemyTierElite, EnemyClassGunner, level)
	bossCooldown := runtime.enemyAttackCooldownFramesLocked(EnemyTierBoss, EnemyClassGunner, level)

	if eliteHealth <= minionHealth {
		t.Fatalf("expected elite health %d to exceed minion health %d", eliteHealth, minionHealth)
	}
	if bossHealth <= eliteHealth {
		t.Fatalf("expected boss health %d to exceed elite health %d", bossHealth, eliteHealth)
	}
	if eliteBladeDamage <= minionBladeDamage {
		t.Fatalf("expected elite damage %d to exceed minion damage %d", eliteBladeDamage, minionBladeDamage)
	}
	if bossBladeDamage <= eliteBladeDamage {
		t.Fatalf("expected boss damage %d to exceed elite damage %d", bossBladeDamage, eliteBladeDamage)
	}
	if eliteCooldown >= minionCooldown {
		t.Fatalf("expected elite cooldown %d to be faster than minion cooldown %d", eliteCooldown, minionCooldown)
	}
	if bossCooldown >= eliteCooldown {
		t.Fatalf("expected boss cooldown %d to be faster than elite cooldown %d", bossCooldown, eliteCooldown)
	}
}

func TestExportEnemiesIncludesEnemyClass(t *testing.T) {
	runtime := NewRuntime(60)
	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyClass.Value = EnemyClassGunner

	entities := runtime.exportEnemiesLocked()
	if len(entities) != 1 {
		t.Fatalf("expected 1 exported enemy, got %d", len(entities))
	}
	if got := entities[0].GetEnemyClass(); got != runtime.enemyClassProtoLocked(EnemyClassGunner) {
		t.Fatalf("expected exported enemy class %v, got %v", runtime.enemyClassProtoLocked(EnemyClassGunner), got)
	}
}

func TestSpawnEnemyTierRespectsMinimumLevel(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyBaseLevel = 1
	cfg.EnemyMaxLevel = 6
	cfg.EnemyBonusLevelChance = 0

	minionRuntime := NewRuntimeWithConfig(cfg)
	minionRuntime.cfg.EnemyMinionSpawnWeight = 1
	minionRuntime.cfg.EnemyEliteSpawnWeight = 0
	minionRuntime.cfg.EnemyBossSpawnWeight = 0
	minionRuntime.spawnEnemyLocked()
	minion := firstRawEnemyEntity(t, minionRuntime)
	minionTier := (*EnemyTier)(minionRuntime.world.Get(minion, minionRuntime.ids.enemyTier))
	minionLevel := (*EnemyLevel)(minionRuntime.world.Get(minion, minionRuntime.ids.enemyLevel))
	if got := minionTier.Value; got != EnemyTierMinion {
		t.Fatalf("expected minion tier, got %d", got)
	}
	if got, want := minionLevel.Value, cfg.EnemyMinionMinLevel; got < want {
		t.Fatalf("expected minion level >= %d, got %d", want, got)
	}

	eliteRuntime := NewRuntimeWithConfig(cfg)
	eliteRuntime.cfg.EnemyMinionSpawnWeight = 0
	eliteRuntime.cfg.EnemyEliteSpawnWeight = 1
	eliteRuntime.cfg.EnemyBossSpawnWeight = 0
	eliteRuntime.spawnEnemyLocked()
	elite := firstRawEnemyEntity(t, eliteRuntime)
	eliteTier := (*EnemyTier)(eliteRuntime.world.Get(elite, eliteRuntime.ids.enemyTier))
	eliteLevel := (*EnemyLevel)(eliteRuntime.world.Get(elite, eliteRuntime.ids.enemyLevel))
	if got := eliteTier.Value; got != EnemyTierElite {
		t.Fatalf("expected elite tier, got %d", got)
	}
	if got, want := eliteLevel.Value, cfg.EnemyEliteMinLevel; got < want {
		t.Fatalf("expected elite level >= %d, got %d", want, got)
	}

	bossRuntime := NewRuntimeWithConfig(cfg)
	bossRuntime.cfg.EnemyMinionSpawnWeight = 0
	bossRuntime.cfg.EnemyEliteSpawnWeight = 0
	bossRuntime.cfg.EnemyBossSpawnWeight = 1
	bossRuntime.startHordeLocked(bossRuntime.getHordeStateLocked())
	bossRuntime.spawnEnemyLocked()
	boss := firstRawEnemyEntity(t, bossRuntime)
	bossTier := (*EnemyTier)(bossRuntime.world.Get(boss, bossRuntime.ids.enemyTier))
	bossLevel := (*EnemyLevel)(bossRuntime.world.Get(boss, bossRuntime.ids.enemyLevel))
	if got := bossTier.Value; got != EnemyTierBoss {
		t.Fatalf("expected boss tier, got %d", got)
	}
	if got, want := bossLevel.Value, cfg.EnemyBossMinLevel; got < want {
		t.Fatalf("expected boss level >= %d, got %d", want, got)
	}
}

func TestEnemySpawnDelayBlocksMovementAndAttack(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyMinionSpawnWeight = 1
	cfg.EnemyEliteSpawnWeight = 0
	cfg.EnemyBossSpawnWeight = 0
	cfg.EnemyMinionSpawnDelay = time.Second
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstRawEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemySpawn := (*EnemySpawnState)(runtime.world.Get(enemy, runtime.ids.enemySpawnState))
	fireCooldown := (*FireCooldown)(runtime.world.Get(enemy, runtime.ids.fireCooldown))

	enemyPos.X = 10
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassGunner
	fireCooldown.Frames = 0

	runtime.updateEnemyAggroLocked()
	runtime.updateEnemyMovementLocked()
	runtime.updateEnemyRollLocked()
	runtime.updateEnemyFireLocked()

	if enemySpawn.RemainingFrames <= 0 {
		t.Fatal("expected spawned enemy to still be in spawn delay")
	}
	if math.Abs(enemyVel.X)+math.Abs(enemyVel.Y) > 1e-6 {
		t.Fatalf("expected spawning enemy to stay still, got velocity=(%.6f, %.6f)", enemyVel.X, enemyVel.Y)
	}
	if got := countEnemyBullets(runtime); got != 0 {
		t.Fatalf("expected spawning enemy not to fire, got %d bullets", got)
	}
}

func TestExportEnemiesIncludesTierAndSpawnFrames(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyMinionSpawnWeight = 0
	cfg.EnemyEliteSpawnWeight = 1
	cfg.EnemyBossSpawnWeight = 0
	cfg.EnemyEliteSpawnDelay = time.Second
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstRawEnemyEntity(t, runtime)
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	spawnState := (*EnemySpawnState)(runtime.world.Get(enemy, runtime.ids.enemySpawnState))

	entities := runtime.exportEnemiesLocked()
	if len(entities) != 1 {
		t.Fatalf("expected 1 exported enemy, got %d", len(entities))
	}
	if got := entities[0].GetEnemyTier(); got != runtime.enemyTierProtoLocked(enemyTier.Value) {
		t.Fatalf("expected exported enemy tier %v, got %v", runtime.enemyTierProtoLocked(enemyTier.Value), got)
	}
	if got, want := entities[0].GetSpawnRemainingFrames(), uint32(spawnState.RemainingFrames); got != want {
		t.Fatalf("expected exported spawn remaining frames %d, got %d", want, got)
	}
	if got, want := entities[0].GetSpawnTotalFrames(), uint32(spawnState.TotalFrames); got != want {
		t.Fatalf("expected exported spawn total frames %d, got %d", want, got)
	}
}

func TestSnapshotExportsHordeStatus(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.HordeThreshold = 3
	cfg.HordeDuration = 5 * time.Second
	runtime := NewRuntimeWithConfig(cfg)
	horde := runtime.getHordeStateLocked()
	horde.Value = 2

	snapshot := runtime.BuildSnapshotFor(0)
	if snapshot == nil || snapshot.GetHorde() == nil {
		t.Fatal("expected snapshot horde status")
	}
	if got := snapshot.GetHorde().GetValue(); got != 2 {
		t.Fatalf("expected horde value 2, got %d", got)
	}
	if got := snapshot.GetHorde().GetThreshold(); got != 3 {
		t.Fatalf("expected horde threshold 3, got %d", got)
	}
}

func TestEnemyKillStartsHordeAndNextSpawnIsBoss(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyMinionSpawnWeight = 1
	cfg.EnemyEliteSpawnWeight = 0
	cfg.EnemyBossSpawnWeight = 1
	cfg.HordeBossSpawnWeight = 1
	cfg.HordeThreshold = 1
	cfg.HordeValuePerMinionKill = 1
	cfg.EnemySkillDropChance = 1
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier)).Value
	health := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	health.Value = 0

	runtime.cleanupDeadLocked()

	horde := runtime.getHordeStateLocked()
	if !horde.Active {
		t.Fatal("expected horde to become active after threshold kill")
	}
	if got := horde.Value; got != cfg.HordeThreshold {
		t.Fatalf("expected horde value clamped to threshold %d, got %d", cfg.HordeThreshold, got)
	}
	if enemyTier != EnemyTierMinion {
		t.Fatalf("expected pre-horde enemy to be minion, got %d", enemyTier)
	}

	runtime.spawnEnemyLocked()
	boss := firstRawEnemyEntity(t, runtime)
	bossTier := (*EnemyTier)(runtime.world.Get(boss, runtime.ids.enemyTier))
	if got := bossTier.Value; got != EnemyTierBoss {
		t.Fatalf("expected first horde spawn to be boss, got %d", got)
	}
	if !horde.BossSpawnedThisWave {
		t.Fatal("expected horde state to record boss spawn")
	}
}

func TestHordeScalesEnemyAggroRanges(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.HordeAggroRadiusScale = 2
	cfg.HordeAlertRadiusScale = 1.5
	runtime := NewRuntimeWithConfig(cfg)

	baseAggro := runtime.enemyAggroRadiusLocked(EnemyClassGunner)
	baseAlert := runtime.enemyAlertRadiusLocked(EnemyClassGunner)

	runtime.startHordeLocked(runtime.getHordeStateLocked())

	if got, want := runtime.enemyAggroRadiusLocked(EnemyClassGunner), baseAggro*cfg.HordeAggroRadiusScale; math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected horde aggro radius %.2f, got %.2f", want, got)
	}
	if got, want := runtime.enemyAlertRadiusLocked(EnemyClassGunner), baseAlert*cfg.HordeAlertRadiusScale; math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected horde alert radius %.2f, got %.2f", want, got)
	}
}

func TestHordeSpawnIntervalRampsToPeakByHalfway(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.HordeSpawnInterval = 1200 * time.Millisecond
	cfg.HordePeakSpawnInterval = 400 * time.Millisecond
	cfg.HordeDuration = 20 * time.Second
	runtime := NewRuntimeWithConfig(cfg)

	horde := runtime.getHordeStateLocked()
	runtime.startHordeLocked(horde)

	startInterval := runtime.currentSpawnIntervalFramesLocked()
	if got, want := startInterval, runtime.params.HordeSpawnTimerFrames; got != want {
		t.Fatalf("expected horde start interval %d, got %d", want, got)
	}

	horde.RemainingFrames = horde.TotalFrames * 3 / 4
	midRampInterval := runtime.currentSpawnIntervalFramesLocked()
	if midRampInterval >= startInterval {
		t.Fatalf("expected early horde interval %d to be faster than start interval %d", midRampInterval, startInterval)
	}
	if midRampInterval <= runtime.params.HordePeakSpawnTimerFrames {
		t.Fatalf("expected early horde interval %d to stay above peak interval %d before halfway", midRampInterval, runtime.params.HordePeakSpawnTimerFrames)
	}

	horde.RemainingFrames = horde.TotalFrames / 2
	halfwayInterval := runtime.currentSpawnIntervalFramesLocked()
	if got, want := halfwayInterval, runtime.params.HordePeakSpawnTimerFrames; got != want {
		t.Fatalf("expected halfway horde interval %d, got %d", want, got)
	}

	horde.RemainingFrames = horde.TotalFrames / 4
	lateInterval := runtime.currentSpawnIntervalFramesLocked()
	if got, want := lateInterval, runtime.params.HordePeakSpawnTimerFrames; got != want {
		t.Fatalf("expected late horde interval to stay at peak %d, got %d", want, got)
	}
}

func TestSpawnRespectsEnemyCap(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.MaxEnemies = 2
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	runtime.spawnEnemyLocked()
	if got := countEnemies(runtime); got != 2 {
		t.Fatalf("expected 2 enemies after manual spawns, got %d", got)
	}

	runtime.getSpawnTimerLocked().Frames = 1
	runtime.updateSpawnLocked()

	if got := countEnemies(runtime); got != 2 {
		t.Fatalf("expected enemy cap to block extra spawns, got %d enemies", got)
	}
}

func TestEnemySpawnKeepsSafeDistanceFromPlayers(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemySpawnMinDistance = 25
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 50
	playerPos.Y = 50

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))

	if got := math.Hypot(enemyPos.X-playerPos.X, enemyPos.Y-playerPos.Y); got < cfg.EnemySpawnMinDistance {
		t.Fatalf("expected enemy to spawn at least %.2f units away from player, got %.2f", cfg.EnemySpawnMinDistance, got)
	}
}

func TestEnemyCleanupRemovesUntouchedEnemyAfterLifetime(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyCleanupLifetime = time.Second
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	lifecycle := (*EnemyLifecycle)(runtime.world.Get(enemy, runtime.ids.enemyLifecycle))
	lifecycle.AgeFrames = runtime.params.EnemyCleanupLifetimeFrames

	runtime.updateEnemyCleanupLocked()

	if runtime.world.Alive(enemy) {
		t.Fatal("expected untouched enemy to be removed after cleanup lifetime")
	}
}

func TestEnemyCleanupRemovesFarEnemyAfterLifetime(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyCleanupLifetime = time.Second
	cfg.EnemyCleanupDistance = 20
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 10
	playerPos.Y = 10

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	lifecycle := (*EnemyLifecycle)(runtime.world.Get(enemy, runtime.ids.enemyLifecycle))
	lifecycle.AgeFrames = runtime.params.EnemyCleanupLifetimeFrames
	lifecycle.HasTouchedPlayer = true
	enemyPos.X = 80
	enemyPos.Y = 80

	runtime.updateEnemyCleanupLocked()

	if runtime.world.Alive(enemy) {
		t.Fatal("expected far enemy to be removed after cleanup lifetime")
	}
}

func TestEnemyCleanupKeepsTouchedNearbyEnemy(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyCleanupLifetime = time.Second
	cfg.EnemyCleanupDistance = 20
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 10
	playerPos.Y = 10

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	lifecycle := (*EnemyLifecycle)(runtime.world.Get(enemy, runtime.ids.enemyLifecycle))
	lifecycle.AgeFrames = runtime.params.EnemyCleanupLifetimeFrames
	lifecycle.HasTouchedPlayer = true
	enemyPos.X = 18
	enemyPos.Y = 10

	runtime.updateEnemyCleanupLocked()

	if !runtime.world.Alive(enemy) {
		t.Fatal("expected touched nearby enemy to stay alive after cleanup pass")
	}
}

func TestLevelOneEnemyStartsWithoutSkills(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyBaseLevel = 1
	cfg.EnemyMaxLevel = 1
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	level := (*EnemyLevel)(runtime.world.Get(enemy, runtime.ids.enemyLevel))
	inventory := (*SkillInventory)(runtime.world.Get(enemy, runtime.ids.skillInventory))

	if got := level.Value; got != 1 {
		t.Fatalf("expected enemy level 1, got %d", got)
	}
	if got := len(inventory.Skills); got != 0 {
		t.Fatalf("expected level-1 enemy to start without skills, got %d", got)
	}
}

func TestHighLevelEnemyStartsWithMoreSkills(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyBaseLevel = 1
	cfg.EnemyMaxLevel = 4
	cfg.EnemyBonusLevelChance = 1
	cfg.EnemyMinionSpawnWeight = 0
	cfg.EnemyEliteSpawnWeight = 1
	cfg.EnemyBossSpawnWeight = 0
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	level := (*EnemyLevel)(runtime.world.Get(enemy, runtime.ids.enemyLevel))
	inventory := (*SkillInventory)(runtime.world.Get(enemy, runtime.ids.skillInventory))

	if got := level.Value; got != 4 {
		t.Fatalf("expected enemy level 4, got %d", got)
	}
	if got := len(inventory.Skills); got != 3 {
		t.Fatalf("expected level-4 enemy to start with 3 skills, got %d", got)
	}
}

func TestEnemyDeathDropsSingleOwnedSkill(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemySkillDropChance = 1
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	health := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	inventory := (*SkillInventory)(runtime.world.Get(enemy, runtime.ids.skillInventory))
	tier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	clearSkillInventory(inventory)
	tier.Value = EnemyTierElite
	inventory.Skills = append(inventory.Skills, SkillProgress{SkillID: "skill_toxic_rounds_1", Level: 1})
	health.Value = 0

	runtime.cleanupDeadLocked()

	if got := countSkillDrops(runtime); got != 1 {
		t.Fatalf("expected single-skill enemy to drop exactly one skill pickup, got %d", got)
	}
	drop := firstSkillDropEntity(t, runtime)
	skillDrop := (*SkillDrop)(runtime.world.Get(drop, runtime.ids.skillDrop))
	if skillDrop.SkillID != "skill_toxic_rounds_1" {
		t.Fatalf("expected dropped skill id %q, got %q", "skill_toxic_rounds_1", skillDrop.SkillID)
	}
}

func TestMinionNeverCarriesOrDropsSkills(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyBaseLevel = 1
	cfg.EnemyMaxLevel = 4
	cfg.EnemyBonusLevelChance = 1
	cfg.EnemyMinionSpawnWeight = 1
	cfg.EnemyEliteSpawnWeight = 0
	cfg.EnemyBossSpawnWeight = 0
	cfg.EnemySkillDropChance = 1
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	tier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	inventory := (*SkillInventory)(runtime.world.Get(enemy, runtime.ids.skillInventory))
	health := (*Health)(runtime.world.Get(enemy, runtime.ids.health))

	if got := tier.Value; got != EnemyTierMinion {
		t.Fatalf("expected minion tier, got %d", got)
	}
	if got := len(inventory.Skills); got != 0 {
		t.Fatalf("expected minion not to carry skills, got %d", got)
	}

	runtime.spawnSkillDropLocked(Position{X: (*Position)(runtime.world.Get(enemy, runtime.ids.position)).X, Y: (*Position)(runtime.world.Get(enemy, runtime.ids.position)).Y}, "skill_toxic_rounds_1")
	runtime.applySkillPickupLocked()
	if got := len(inventory.Skills); got != 0 {
		t.Fatalf("expected minion not to absorb skill drops, got %d carried skills", got)
	}
	if got := countSkillDrops(runtime); got != 1 {
		t.Fatalf("expected unabsorbed skill drop to stay on ground, got %d", got)
	}

	health.Value = 0
	runtime.cleanupDeadLocked()

	if got := countSkillDrops(runtime); got != 1 {
		t.Fatalf("expected minion death not to add new skill drops, got total %d", got)
	}
}

func TestBossSkillDropStillGuaranteed(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemySkillDropChance = 0
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	tier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	health := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	inventory := (*SkillInventory)(runtime.world.Get(enemy, runtime.ids.skillInventory))

	clearSkillInventory(inventory)
	tier.Value = EnemyTierBoss
	inventory.Skills = append(inventory.Skills, SkillProgress{SkillID: "skill_toxic_rounds_1", Level: 1})
	health.Value = 0

	runtime.cleanupDeadLocked()

	if got := countSkillDrops(runtime); got != 1 {
		t.Fatalf("expected boss with one skill to still guarantee one drop, got %d", got)
	}
}

func TestPlayerCanAbsorbSkillDrop(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	inventory := (*SkillInventory)(runtime.world.Get(player, runtime.ids.skillInventory))
	clearSkillInventory(inventory)

	runtime.spawnSkillDropLocked(Position{X: playerPos.X, Y: playerPos.Y}, "skill_frost_rounds_1")
	runtime.applySkillPickupLocked()

	if got := len(inventory.Skills); got != 1 {
		t.Fatalf("expected player to absorb one skill pickup, got %d skills", got)
	}
	if inventory.Skills[0].SkillID != "skill_frost_rounds_1" {
		t.Fatalf("expected absorbed skill %q, got %q", "skill_frost_rounds_1", inventory.Skills[0].SkillID)
	}
	if got := countSkillDrops(runtime); got != 0 {
		t.Fatalf("expected consumed skill drop to be removed, got %d", got)
	}
}

func TestEnemyCanAbsorbSkillDrop(t *testing.T) {
	runtime := NewRuntime(60)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	tier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	inventory := (*SkillInventory)(runtime.world.Get(enemy, runtime.ids.skillInventory))
	clearSkillInventory(inventory)
	tier.Value = EnemyTierElite

	runtime.spawnSkillDropLocked(Position{X: enemyPos.X, Y: enemyPos.Y}, "skill_toxic_rounds_1")
	runtime.applySkillPickupLocked()

	if got := len(inventory.Skills); got != 1 {
		t.Fatalf("expected enemy to absorb one skill pickup, got %d skills", got)
	}
	if inventory.Skills[0].SkillID != "skill_toxic_rounds_1" {
		t.Fatalf("expected enemy absorbed skill %q, got %q", "skill_toxic_rounds_1", inventory.Skills[0].SkillID)
	}
}

func TestEnemySeeksNearbySkillDropWhenOutOfCombat(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemySkillPickupSeekRadius = 20
	runtime := NewRuntimeWithConfig(cfg)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	tier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))

	enemyPos.X = 10
	enemyPos.Y = 10
	tier.Value = EnemyTierElite
	runtime.spawnSkillDropLocked(Position{X: 18, Y: 10}, "skill_frost_rounds_1")
	runtime.updateEnemyMovementLocked()

	if enemyVel.X <= 0 || math.Abs(enemyVel.Y) > 1e-6 {
		t.Fatalf("expected idle enemy to move toward nearby skill drop, got velocity=(%.6f, %.6f)", enemyVel.X, enemyVel.Y)
	}
}

func TestEnemyCombatTargetOverridesSkillDropSeeking(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemySkillPickupSeekRadius = 30
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 24
	playerPos.Y = 10

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	moveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))

	enemyPos.X = 10
	enemyPos.Y = 10
	enemyClass.Value = EnemyClassGunner
	enemyTier.Value = EnemyTierElite
	moveState.StrafeSign = 1
	runtime.spawnSkillDropLocked(Position{X: 11, Y: 10}, "skill_frost_rounds_1")
	runtime.updateEnemyMovementLocked()

	if math.Abs(enemyVel.X)+math.Abs(enemyVel.Y) <= 1e-6 {
		t.Fatal("expected enemy with combat target to move for combat instead of idling")
	}
	if math.Abs(enemyVel.Y) <= 1e-6 {
		t.Fatalf("expected gunner combat movement to take priority over nearby skill drop, got velocity=(%.6f, %.6f)", enemyVel.X, enemyVel.Y)
	}
}

func TestEnemyRetaliatesByChasingFarPlayerWithoutFiringOffscreen(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 40
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyCooldown := (*FireCooldown)(runtime.world.Get(enemy, runtime.ids.fireCooldown))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassGunner
	enemyCooldown.Frames = 0

	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position:      Position{X: 0, Y: 0},
		Velocity:      Velocity{X: 1, Y: 0},
		OwnerPlayerID: playerID,
	})
	playerBullet := firstPlayerBulletEntity(t, runtime)

	runtime.applyHitEventsLocked([]HitEvent{{
		Bullet: playerBullet,
		Target: enemy,
		Damage: 0,
	}})

	runtime.updateEnemyMovementLocked()
	runtime.updateEnemyFireLocked()

	if got := countEnemyBullets(runtime); got != 0 {
		t.Fatalf("expected retaliating gunner to stop firing beyond combat range, got %d bullets", got)
	}
	if enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity)); enemyVel.X <= 0 || enemyVel.Y <= 0 {
		t.Fatalf("expected retaliating gunner to chase distant target, got velocity=(%.6f, %.6f)", enemyVel.X, enemyVel.Y)
	}
}

func TestEnemyAggroClearsWhenTargetPlayerDies(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerHealth := (*Health)(runtime.world.Get(player, runtime.ids.health))

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))
	aggroTarget.Value = playerID

	playerHealth.Value = 0
	runtime.cleanupDeadLocked()

	if aggroTarget.Value != 0 {
		t.Fatalf("expected enemy aggro to clear after target death, got %d", aggroTarget.Value)
	}
}

func TestBladeKeepsAggroBeyondBaseAggroRadiusAfterBeingHit(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 40
	playerPos.Y = 40

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassBlade

	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position:      Position{X: 0, Y: 0},
		Velocity:      Velocity{X: 1, Y: 0},
		OwnerPlayerID: playerID,
	})
	playerBullet := firstPlayerBulletEntity(t, runtime)

	runtime.applyHitEventsLocked([]HitEvent{{
		Bullet: playerBullet,
		Target: enemy,
		Damage: 0,
	}})

	runtime.updateEnemyMovementLocked()

	if aggroTarget.Value != playerID {
		t.Fatalf("expected blade to keep aggro on hit player %d, got %d", playerID, aggroTarget.Value)
	}
	if math.Abs(enemyVel.X)+math.Abs(enemyVel.Y) <= 1e-6 {
		t.Fatal("expected blade with aggro to keep chasing distant target")
	}
}

func TestEnemyDropsAggroWhenTargetIsTooFarAway(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 80
	playerPos.Y = 80

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassGunner
	aggroTarget.Value = playerID

	runtime.updateEnemyMovementLocked()
	runtime.updateEnemyFireLocked()

	if aggroTarget.Value != 0 {
		t.Fatalf("expected overly distant aggro target to be cleared, got %d", aggroTarget.Value)
	}
	if got := countEnemyBullets(runtime); got != 0 {
		t.Fatalf("expected no bullets after dropping distant aggro, got %d", got)
	}
}

func TestEnemyLocksTargetAfterStayingInAlertRadius(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyGunnerAggroRadius = 10
	cfg.EnemyGunnerAlertRadius = 20
	cfg.EnemyAlertLockDelay = time.Second / 20
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 15
	playerPos.Y = 0

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))
	aggroWatch := (*AggroWatchState)(runtime.world.Get(enemy, runtime.ids.aggroWatchState))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassGunner

	runtime.updateEnemyAggroLocked()
	if aggroTarget.Value != 0 {
		t.Fatalf("expected no immediate lock from outer alert radius, got %d", aggroTarget.Value)
	}
	if aggroWatch.CandidatePlayerID != playerID || aggroWatch.Frames != 1 {
		t.Fatalf("expected alert watch to start on player %d with 1 frame, got player=%d frames=%d", playerID, aggroWatch.CandidatePlayerID, aggroWatch.Frames)
	}

	runtime.updateEnemyAggroLocked()
	if aggroTarget.Value != 0 {
		t.Fatalf("expected no lock before alert timer completes, got %d", aggroTarget.Value)
	}

	runtime.updateEnemyAggroLocked()
	if aggroTarget.Value != playerID {
		t.Fatalf("expected alert timer to lock player %d, got %d", playerID, aggroTarget.Value)
	}
	if aggroWatch.CandidatePlayerID != 0 || aggroWatch.Frames != 0 {
		t.Fatalf("expected alert watch to clear after locking, got player=%d frames=%d", aggroWatch.CandidatePlayerID, aggroWatch.Frames)
	}
}

func TestEnemyLocksImmediatelyInsideBaseAggroRadius(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyGunnerAggroRadius = 10
	cfg.EnemyGunnerAlertRadius = 20
	cfg.EnemyAlertLockDelay = time.Second
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 8
	playerPos.Y = 0

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))
	aggroWatch := (*AggroWatchState)(runtime.world.Get(enemy, runtime.ids.aggroWatchState))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassGunner

	runtime.updateEnemyAggroLocked()
	if aggroTarget.Value != playerID {
		t.Fatalf("expected immediate lock inside base aggro radius on player %d, got %d", playerID, aggroTarget.Value)
	}
	if aggroWatch.CandidatePlayerID != 0 || aggroWatch.Frames != 0 {
		t.Fatalf("expected no delayed watch after immediate lock, got player=%d frames=%d", aggroWatch.CandidatePlayerID, aggroWatch.Frames)
	}
}

func TestEnemyAlertTimerResetsWhenPlayerLeavesAlertRadius(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyBladeAggroRadius = 8
	cfg.EnemyBladeAlertRadius = 16
	cfg.EnemyAlertLockDelay = time.Second / 20
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))
	aggroWatch := (*AggroWatchState)(runtime.world.Get(enemy, runtime.ids.aggroWatchState))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassBlade

	playerPos.X = 12
	playerPos.Y = 0
	runtime.updateEnemyAggroLocked()
	if aggroWatch.CandidatePlayerID != playerID || aggroWatch.Frames != 1 {
		t.Fatalf("expected alert watch to start before leaving radius, got player=%d frames=%d", aggroWatch.CandidatePlayerID, aggroWatch.Frames)
	}

	playerPos.X = 30
	runtime.updateEnemyAggroLocked()
	if aggroTarget.Value != 0 {
		t.Fatalf("expected no lock after leaving alert radius, got %d", aggroTarget.Value)
	}
	if aggroWatch.CandidatePlayerID != 0 || aggroWatch.Frames != 0 {
		t.Fatalf("expected alert watch to reset after leaving radius, got player=%d frames=%d", aggroWatch.CandidatePlayerID, aggroWatch.Frames)
	}
}

func TestSnapshotExportsEnemyWatchingState(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.EnemyGunnerAggroRadius = 10
	cfg.EnemyGunnerAlertRadius = 20
	cfg.EnemyAlertLockDelay = time.Second
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 15
	playerPos.Y = 0

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyNetID := (*NetworkID)(runtime.world.Get(enemy, runtime.ids.networkID))

	enemyPos.X = 0
	enemyPos.Y = 0
	enemyClass.Value = EnemyClassGunner

	runtime.updateEnemyAggroLocked()

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	enemyState := findEntityStateByNetID(t, snapshot, enemyNetID.Value)
	if got := enemyState.GetEnemyAggroState(); got != netproto.EnemyAggroState_ENEMY_AGGRO_STATE_WATCHING {
		t.Fatalf("expected enemy watching state, got %v", got)
	}
	if got := enemyState.GetAggroTargetPlayerId(); got != playerID {
		t.Fatalf("expected watching target player %d, got %d", playerID, got)
	}
	if got := enemyState.GetAggroWatchFrames(); got != 1 {
		t.Fatalf("expected watch frames 1, got %d", got)
	}
	if got := enemyState.GetAggroWatchTotalFrames(); got == 0 {
		t.Fatal("expected exported total watch frames > 0")
	}
}

func TestSnapshotExportsExpPickupEvent(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerNetID := (*NetworkID)(runtime.world.Get(player, runtime.ids.networkID))
	playerPos.X = 10
	playerPos.Y = 10

	runtime.spawnExpOrbLocked(Position{X: 10, Y: 10}, 7)
	expOrb := firstExpOrbEntity(t, runtime)
	expOrbNetID := (*NetworkID)(runtime.world.Get(expOrb, runtime.ids.networkID)).Value

	runtime.applyExpPickupLocked()

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if got := len(snapshot.GetPickupEvents()); got != 1 {
		t.Fatalf("expected 1 pickup event, got %d", got)
	}

	event := snapshot.GetPickupEvents()[0]
	if got := event.GetCollectorNetId(); got != playerNetID.Value {
		t.Fatalf("expected collector net id %d, got %d", playerNetID.Value, got)
	}
	if got := event.GetCollectorKind(); got != netproto.EntityKind_ENTITY_KIND_PLAYER {
		t.Fatalf("expected collector kind player, got %v", got)
	}
	if got := event.GetCollectorPlayerId(); got != playerID {
		t.Fatalf("expected collector player id %d, got %d", playerID, got)
	}
	if got := event.GetPickupNetId(); got != expOrbNetID {
		t.Fatalf("expected pickup net id %d, got %d", expOrbNetID, got)
	}
	if got := event.GetPickupKind(); got != netproto.EntityKind_ENTITY_KIND_PICKUP_EXP {
		t.Fatalf("expected exp pickup kind, got %v", got)
	}
	if got := event.GetExpValue(); got != 7 {
		t.Fatalf("expected exp value 7, got %d", got)
	}
	if !event.GetGranted() {
		t.Fatal("expected exp pickup event to be granted")
	}
}

func TestSnapshotExportsEnemySkillPickupEvent(t *testing.T) {
	runtime := NewRuntime(60)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyNetID := (*NetworkID)(runtime.world.Get(enemy, runtime.ids.networkID))
	enemyTier := (*EnemyTier)(runtime.world.Get(enemy, runtime.ids.enemyTier))
	enemyPos.X = 10
	enemyPos.Y = 10
	enemyTier.Value = EnemyTierElite

	runtime.spawnSkillDropLocked(Position{X: 10, Y: 10}, "skill_toxic_rounds_1")
	drop := firstSkillDropEntity(t, runtime)
	dropNetID := (*NetworkID)(runtime.world.Get(drop, runtime.ids.networkID)).Value

	runtime.applySkillPickupLocked()

	snapshot := runtime.BuildSnapshotFor(0)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if got := len(snapshot.GetPickupEvents()); got != 1 {
		t.Fatalf("expected 1 pickup event, got %d", got)
	}

	event := snapshot.GetPickupEvents()[0]
	if got := event.GetCollectorNetId(); got != enemyNetID.Value {
		t.Fatalf("expected collector net id %d, got %d", enemyNetID.Value, got)
	}
	if got := event.GetCollectorKind(); got != netproto.EntityKind_ENTITY_KIND_ENEMY {
		t.Fatalf("expected collector kind enemy, got %v", got)
	}
	if got := event.GetCollectorPlayerId(); got != 0 {
		t.Fatalf("expected enemy collector player id 0, got %d", got)
	}
	if got := event.GetPickupNetId(); got != dropNetID {
		t.Fatalf("expected pickup net id %d, got %d", dropNetID, got)
	}
	if got := event.GetPickupKind(); got != netproto.EntityKind_ENTITY_KIND_PICKUP_SKILL {
		t.Fatalf("expected skill pickup kind, got %v", got)
	}
	if got := event.GetSkillId(); got != "skill_toxic_rounds_1" {
		t.Fatalf("expected skill id %q, got %q", "skill_toxic_rounds_1", got)
	}
	if !event.GetGranted() {
		t.Fatal("expected skill pickup event to be granted")
	}
}

func TestEnemyBladeDodgesThreateningPlayerBulletWhenRollReady(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 13
	playerPos.Y = 1

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyRollState := (*RollState)(runtime.world.Get(enemy, runtime.ids.rollState))
	enemyRollLock := (*RollLock)(runtime.world.Get(enemy, runtime.ids.rollLock))
	enemyRollStats := (*RollStats)(runtime.world.Get(enemy, runtime.ids.rollStats))

	enemyPos.X = 10
	enemyPos.Y = 1
	enemyClass.Value = EnemyClassBlade
	*enemyRollStats = runtime.enemyRollStatsLocked(EnemyClassBlade)
	resetRollState(enemyRollState, enemyRollStats)
	enemyRollLock.Frames = 0

	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position:      Position{X: 0, Y: 1},
		Velocity:      Velocity{X: runtime.params.PlayerBulletSpeed, Y: 0},
		OwnerPlayerID: 1,
	})

	runtime.updateEnemyRollLocked()

	if enemyRollState.ActiveFrames <= 0 {
		t.Fatal("expected blade to start a dodge roll against threatening bullet")
	}
	if math.Abs(enemyRollState.DirY) <= 1e-6 {
		t.Fatalf("expected blade dodge roll to move sideways off bullet path, got dir=(%.6f, %.6f)", enemyRollState.DirX, enemyRollState.DirY)
	}
}

func TestEnemyBladeDoesNotDodgeBulletOutsideCombat(t *testing.T) {
	runtime := NewRuntime(60)

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyRollState := (*RollState)(runtime.world.Get(enemy, runtime.ids.rollState))
	enemyRollLock := (*RollLock)(runtime.world.Get(enemy, runtime.ids.rollLock))
	enemyRollStats := (*RollStats)(runtime.world.Get(enemy, runtime.ids.rollStats))

	enemyPos.X = 10
	enemyPos.Y = 1
	enemyClass.Value = EnemyClassBlade
	*enemyRollStats = runtime.enemyRollStatsLocked(EnemyClassBlade)
	resetRollState(enemyRollState, enemyRollStats)
	enemyRollLock.Frames = 0

	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position:      Position{X: 0, Y: 1},
		Velocity:      Velocity{X: runtime.params.PlayerBulletSpeed, Y: 0},
		OwnerPlayerID: 1,
	})

	runtime.updateEnemyRollLocked()

	if enemyRollState.ActiveFrames != 0 {
		t.Fatalf("expected blade to ignore bullet dodge while out of combat, got active frames %d", enemyRollState.ActiveFrames)
	}
}

func TestEnemyKnockbackResistanceReducesAppliedKnockback(t *testing.T) {
	const inputKnockback = 10.0

	bladeRuntime := NewRuntime(60)
	bladeRuntime.spawnEnemyLocked()
	bladeEnemy := firstEnemyEntity(t, bladeRuntime)
	bladeClass := (*EnemyClass)(bladeRuntime.world.Get(bladeEnemy, bladeRuntime.ids.enemyClass))
	bladeResistance := (*KnockbackResistance)(bladeRuntime.world.Get(bladeEnemy, bladeRuntime.ids.knockbackResistance))
	bladeKnockback := (*Knockback)(bladeRuntime.world.Get(bladeEnemy, bladeRuntime.ids.knockback))
	bladeClass.Value = EnemyClassBlade
	bladeResistance.Value = bladeRuntime.cfg.EnemyBladeKnockbackResistance

	bladeRuntime.applyHitEventsLocked([]HitEvent{{
		Target:     bladeEnemy,
		Damage:     0,
		KnockbackX: inputKnockback,
	}})

	wantBlade := inputKnockback * (1 - bladeRuntime.cfg.EnemyBladeKnockbackResistance)
	assertFloatClose(t, bladeKnockback.X, wantBlade)

	gunnerRuntime := NewRuntime(60)
	gunnerRuntime.spawnEnemyLocked()
	gunnerEnemy := firstEnemyEntity(t, gunnerRuntime)
	gunnerClass := (*EnemyClass)(gunnerRuntime.world.Get(gunnerEnemy, gunnerRuntime.ids.enemyClass))
	gunnerResistance := (*KnockbackResistance)(gunnerRuntime.world.Get(gunnerEnemy, gunnerRuntime.ids.knockbackResistance))
	gunnerKnockback := (*Knockback)(gunnerRuntime.world.Get(gunnerEnemy, gunnerRuntime.ids.knockback))
	gunnerClass.Value = EnemyClassGunner
	gunnerResistance.Value = gunnerRuntime.cfg.EnemyGunnerKnockbackResistance

	gunnerRuntime.applyHitEventsLocked([]HitEvent{{
		Target:     gunnerEnemy,
		Damage:     0,
		KnockbackX: inputKnockback,
	}})

	assertFloatClose(t, gunnerKnockback.X, inputKnockback)

	if !(bladeKnockback.X < gunnerKnockback.X) {
		t.Fatalf("expected blade knockback %.2f to be less than gunner knockback %.2f", bladeKnockback.X, gunnerKnockback.X)
	}
}

func TestEnemyPatrolProducesMovementWithoutCombatTarget(t *testing.T) {
	runtime := NewRuntime(60)
	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	moveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))
	velocity := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))

	enemyClass.Value = EnemyClassGunner
	moveState.PatrolWaitFrames = 0
	moveState.PatrolMoveFrames = 0

	runtime.updateEnemyMovementLocked()

	if math.Abs(velocity.X)+math.Abs(velocity.Y) <= 1e-6 {
		t.Fatal("expected patrol to set a non-zero velocity when no combat target exists")
	}
}

func TestEnemyGunnerCombatMovementStrafesInsteadOfCharging(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	moveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))

	enemyPos.X = 10
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassGunner
	moveState.StrafeSign = 1

	runtime.updateEnemyMovementLocked()

	assertFloatClose(t, enemyVel.X, 0)
	if math.Abs(enemyVel.Y) <= 1e-6 {
		t.Fatal("expected gunner strafe movement to produce vertical speed")
	}
}

func TestEnemyGunnerCombatMovementChasesWhenTargetTooFar(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 30
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	moveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))
	aggroTarget := (*AggroTargetPlayerID)(runtime.world.Get(enemy, runtime.ids.aggroTargetPlayerID))

	enemyPos.X = 0
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassGunner
	moveState.StrafeSign = 1
	aggroTarget.Value = playerID

	runtime.updateEnemyMovementLocked()

	if enemyVel.X <= 0 {
		t.Fatalf("expected far gunner target to produce forward chase velocity, got %.6f", enemyVel.X)
	}
	if math.Abs(enemyVel.Y) > 1e-6 {
		t.Fatalf("expected far gunner chase to avoid sideways-only strafe, got %.6f", enemyVel.Y)
	}
}

func TestEnemyBladeCombatMovementUsesCurvedApproach(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	moveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))

	enemyPos.X = 10
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassBlade
	moveState.ArcSign = 1

	runtime.updateEnemyMovementLocked()

	if enemyVel.X <= 0 {
		t.Fatalf("expected blade to still advance toward player on X axis, got %.6f", enemyVel.X)
	}
	if math.Abs(enemyVel.Y) <= 1e-6 {
		t.Fatal("expected blade curved approach to include tangential Y movement")
	}
}

func TestPlayerRollConsumesChargeAndMovesByConfiguredDistance(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.PlayerRollDistance = 6
	cfg.RollDuration = 100 * 1000 * 1000
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	pos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	rollState := (*RollState)(runtime.world.Get(player, runtime.ids.rollState))
	rollStats := (*RollStats)(runtime.world.Get(player, runtime.ids.rollStats))

	startX := pos.X
	runtime.SetInput(playerID, session.InputState{
		Seq:   1,
		MoveX: 1,
		Roll:  true,
	})
	runtime.updatePlayerControlLocked()
	runtime.updatePlayerRollLocked()

	if got, want := rollState.Charges, rollStats.MaxCharges-1; got != want {
		t.Fatalf("expected charges %d after roll, got %d", want, got)
	}

	for i := 0; i < rollStats.DurationFrames; i++ {
		runtime.updateMovementLocked()
	}

	assertFloatClose(t, pos.X-startX, cfg.PlayerRollDistance)
}

func TestPlayerRollChargesRecoverAfterCooldown(t *testing.T) {
	cfg := DefaultConfig(60)
	cfg.PlayerRollCooldown = 100 * 1000 * 1000
	cfg.RollDuration = 50 * 1000 * 1000
	runtime := NewRuntimeWithConfig(cfg)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	rollState := (*RollState)(runtime.world.Get(player, runtime.ids.rollState))
	rollStats := (*RollStats)(runtime.world.Get(player, runtime.ids.rollStats))

	runtime.SetInput(playerID, session.InputState{
		Seq:   1,
		MoveX: 1,
		Roll:  true,
	})
	runtime.updatePlayerControlLocked()
	runtime.updatePlayerRollLocked()

	if rollState.Charges != rollStats.MaxCharges-1 {
		t.Fatalf("expected one charge consumed, got %d/%d", rollState.Charges, rollStats.MaxCharges)
	}

	for i := 0; i < rollStats.CooldownFrames; i++ {
		runtime.updateRollRecoveryLocked()
	}

	if rollState.Charges != rollStats.MaxCharges {
		t.Fatalf("expected charges to recover to %d, got %d", rollStats.MaxCharges, rollState.Charges)
	}
}

func TestHitTemporarilyLocksRoll(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	rollState := (*RollState)(runtime.world.Get(player, runtime.ids.rollState))
	rollLock := (*RollLock)(runtime.world.Get(player, runtime.ids.rollLock))

	runtime.applyHitEventsLocked([]HitEvent{{
		Target:     player,
		Damage:     1,
		KnockbackX: 1,
	}})

	if rollLock.Frames <= 0 {
		t.Fatal("expected hit to apply a roll lock")
	}

	runtime.SetInput(playerID, session.InputState{
		Seq:   1,
		MoveX: 1,
		Roll:  true,
	})
	runtime.updatePlayerControlLocked()
	runtime.updatePlayerRollLocked()

	if rollState.ActiveFrames != 0 {
		t.Fatalf("expected roll to be blocked while locked, got active frames %d", rollState.ActiveFrames)
	}
}

func TestEnemyBladeStartsCombatRoll(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	enemyMoveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))
	enemyRollState := (*RollState)(runtime.world.Get(enemy, runtime.ids.rollState))
	enemyRollStats := (*RollStats)(runtime.world.Get(enemy, runtime.ids.rollStats))

	enemyPos.X = 10
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassBlade
	enemyMoveState.ArcSign = 1
	*enemyRollStats = runtime.enemyRollStatsLocked(EnemyClassBlade)
	resetRollState(enemyRollState, enemyRollStats)

	runtime.updateEnemyMovementLocked()
	runtime.updateEnemyRollLocked()

	if enemyRollState.ActiveFrames <= 0 {
		t.Fatal("expected blade enemy to start a combat roll")
	}
	if enemyRollState.DirX <= 0 {
		t.Fatalf("expected blade roll to generally move toward the target, got dirX=%.6f", enemyRollState.DirX)
	}
}

func TestEnemyBladeRollCooldownIsShorterThanGunner(t *testing.T) {
	runtime := NewRuntime(60)

	blade := runtime.enemyRollStatsLocked(EnemyClassBlade)
	gunner := runtime.enemyRollStatsLocked(EnemyClassGunner)

	if blade.CooldownFrames >= gunner.CooldownFrames {
		t.Fatalf("expected blade cooldown %d to be shorter than gunner cooldown %d", blade.CooldownFrames, gunner.CooldownFrames)
	}
}

func TestRuntimeLoadsSkillCatalog(t *testing.T) {
	runtime := NewRuntime(60)

	if runtime.SkillCatalog() == nil {
		t.Fatal("expected runtime to expose a loaded skill catalog")
	}
	if _, ok := runtime.SkillCatalog().ResolveSkill("skill_toxic_rounds_1"); !ok {
		t.Fatal("expected default toxic skill to resolve from runtime catalog")
	}
}

func TestHomingSkillSpawnsBonusProjectileAtUpgradedInterval(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]
	inventory := (*SkillInventory)(runtime.world.Get(entity, runtime.ids.skillInventory))
	fireCooldown := (*FireCooldown)(runtime.world.Get(entity, runtime.ids.fireCooldown))
	inventory.Skills = append(inventory.Skills, SkillProgress{
		SkillID: "skill_homing_rounds_1",
		Level:   2,
	})

	for i := 0; i < 4; i++ {
		runtime.SetInput(playerID, session.InputState{
			Seq:   uint32(i + 1),
			Fire:  true,
			AimDX: 1,
		})
		fireCooldown.Frames = 0
		runtime.updatePlayerFireLocked()
	}

	if got := countPlayerBullets(runtime); got != 5 {
		t.Fatalf("expected 4 normal bullets plus 1 homing bonus bullet, got %d", got)
	}
	if got := countHomingPlayerBullets(runtime); got != 1 {
		t.Fatalf("expected exactly 1 homing bonus bullet, got %d", got)
	}
}

func TestHomingProjectileLocksEnemyWhenItEntersSearchRadius(t *testing.T) {
	runtime := NewRuntime(60)

	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position: Position{X: 0, Y: 0},
		Velocity: Velocity{X: runtime.params.PlayerBulletSpeed, Y: 0},
		Homing: &HomingProjectile{
			SearchRadius: 3,
			Speed:        runtime.params.PlayerBulletSpeed,
		},
	})
	bullet := firstHomingPlayerBulletEntity(t, runtime)
	homing := (*HomingProjectile)(runtime.world.Get(bullet, runtime.ids.homingProjectile))

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyPos.X = 10
	enemyPos.Y = 0

	runtime.updateHomingProjectilesLocked()
	if !homing.Target.IsZero() {
		t.Fatal("expected homing projectile to wait when no enemy is inside search radius")
	}

	enemyPos.X = 2
	runtime.updateHomingProjectilesLocked()

	if homing.Target != enemy {
		t.Fatalf("expected homing projectile to lock the first enemy entering range, got %+v", homing.Target)
	}

	velocity := (*Velocity)(runtime.world.Get(bullet, runtime.ids.velocity))
	if velocity.X <= 0 {
		t.Fatalf("expected homing projectile to steer toward target, got velocity=(%.6f, %.6f)", velocity.X, velocity.Y)
	}
}

func TestSnapshotExportsProjectileSubtypeForPlayerBullets(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)

	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position:      Position{X: 1, Y: 1},
		Velocity:      Velocity{X: 2, Y: 0},
		OwnerPlayerID: playerID,
	})
	runtime.spawnPlayerBulletLocked(BulletSpawn{
		Position:      Position{X: 2, Y: 2},
		Velocity:      Velocity{X: 2, Y: 0},
		OwnerPlayerID: playerID,
		Homing: &HomingProjectile{
			SearchRadius: 6,
			Speed:        runtime.params.PlayerBulletSpeed,
		},
	})

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	var normalFound bool
	var homingFound bool
	for _, entity := range snapshot.Entities {
		if entity.Kind != netproto.EntityKind_ENTITY_KIND_BULLET_PLAYER {
			continue
		}
		switch entity.ProjectileSubtype {
		case netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_NORMAL:
			normalFound = true
		case netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_HOMING:
			homingFound = true
		}
	}

	if !normalFound {
		t.Fatal("expected snapshot to export at least one normal player bullet subtype")
	}
	if !homingFound {
		t.Fatal("expected snapshot to export at least one homing player bullet subtype")
	}
}

func TestSnapshotExportsNormalProjectileSubtypeForEnemyBullets(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	runtime.spawnEnemyBulletLocked(BulletSpawn{
		Position: Position{X: 3, Y: 3},
		Velocity: Velocity{X: -1, Y: 0},
	})

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	for _, entity := range snapshot.Entities {
		if entity.Kind != netproto.EntityKind_ENTITY_KIND_BULLET_ENEMY {
			continue
		}
		if entity.ProjectileSubtype != netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_NORMAL {
			t.Fatalf("expected enemy bullet subtype %v, got %v", netproto.ProjectileSubtype_PROJECTILE_SUBTYPE_NORMAL, entity.ProjectileSubtype)
		}
		return
	}

	t.Fatal("expected snapshot to include one enemy bullet")
}

func TestSnapshotExportsSkillDropEntity(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	runtime.spawnSkillDropLocked(Position{X: 7, Y: 9}, "skill_homing_rounds_1")

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	for _, entity := range snapshot.Entities {
		if entity.Kind != netproto.EntityKind_ENTITY_KIND_PICKUP_SKILL {
			continue
		}
		if entity.SkillId != "skill_homing_rounds_1" {
			t.Fatalf("expected exported skill pickup id %q, got %q", "skill_homing_rounds_1", entity.SkillId)
		}
		return
	}

	t.Fatal("expected snapshot to include one skill pickup entity")
}

func TestPoisonBuffTicksDamageAfterHit(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	if !runtime.GrantPlayerSkill(playerID, "skill_toxic_rounds_1") {
		t.Fatal("expected granting poison skill to succeed")
	}

	player := runtime.playerEntities[playerID]
	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyHealth := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	enemyBuffs := (*ActiveBuffs)(runtime.world.Get(enemy, runtime.ids.activeBuffs))

	startHP := enemyHealth.Value
	runtime.applyHitEventsLocked([]HitEvent{{
		Target:       enemy,
		OnHitEffects: runtime.collectOnHitEffectsLocked(player),
	}})

	if len(enemyBuffs.Items) != 1 {
		t.Fatalf("expected 1 active buff after poison hit, got %d", len(enemyBuffs.Items))
	}
	if got := enemyBuffs.Items[0].Status; got != skillcfg.StatusKindPoison {
		t.Fatalf("expected poison buff, got %q", got)
	}
	if got := enemyBuffs.Items[0].Category; got != skillcfg.BuffCategoryDot {
		t.Fatalf("expected poison category %q, got %q", skillcfg.BuffCategoryDot, got)
	}
	if got := enemyBuffs.Items[0].Stacks; got != 1 {
		t.Fatalf("expected poison buff to start at 1 stack, got %d", got)
	}

	for i := 0; i < enemyBuffs.Items[0].TickIntervalFrames; i++ {
		runtime.updateBuffsLocked()
	}

	if enemyHealth.Value >= startHP {
		t.Fatalf("expected poison to reduce hp below %d, got %d", startHP, enemyHealth.Value)
	}
}

func TestPoisonBuffStacksIncreaseDotDamage(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	entity := runtime.playerEntities[playerID]
	inventory := (*SkillInventory)(runtime.world.Get(entity, runtime.ids.skillInventory))
	inventory.Skills = append(inventory.Skills, SkillProgress{
		SkillID: "skill_toxic_rounds_1",
		Level:   2,
	})

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyHealth := (*Health)(runtime.world.Get(enemy, runtime.ids.health))
	enemyBuffs := (*ActiveBuffs)(runtime.world.Get(enemy, runtime.ids.activeBuffs))

	startHP := enemyHealth.Value
	onHitEffects := runtime.collectOnHitEffectsLocked(entity)
	runtime.applyHitEventsLocked([]HitEvent{{Target: enemy, OnHitEffects: onHitEffects}})
	runtime.applyHitEventsLocked([]HitEvent{{Target: enemy, OnHitEffects: onHitEffects}})

	if len(enemyBuffs.Items) != 1 {
		t.Fatalf("expected stacked poison to stay in one buff entry, got %d", len(enemyBuffs.Items))
	}
	if got := enemyBuffs.Items[0].Stacks; got != 2 {
		t.Fatalf("expected poison stacks to reach 2, got %d", got)
	}

	for i := 0; i < enemyBuffs.Items[0].TickIntervalFrames; i++ {
		runtime.updateBuffsLocked()
	}

	wantLoss := enemyBuffs.Items[0].DamagePerTick * enemyBuffs.Items[0].Stacks
	if got := startHP - enemyHealth.Value; got != wantLoss {
		t.Fatalf("expected stacked poison to deal %d damage on tick, got %d", wantLoss, got)
	}
}

func TestChillBuffReducesEnemyMoveSpeed(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	if !runtime.GrantPlayerSkill(playerID, "skill_frost_rounds_1") {
		t.Fatal("expected granting chill skill to succeed")
	}

	player := runtime.playerEntities[playerID]
	playerPos := (*Position)(runtime.world.Get(player, runtime.ids.position))
	playerPos.X = 20
	playerPos.Y = 20

	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)
	enemyPos := (*Position)(runtime.world.Get(enemy, runtime.ids.position))
	enemyVel := (*Velocity)(runtime.world.Get(enemy, runtime.ids.velocity))
	enemyClass := (*EnemyClass)(runtime.world.Get(enemy, runtime.ids.enemyClass))
	moveState := (*EnemyMoveState)(runtime.world.Get(enemy, runtime.ids.enemyMoveState))
	enemyBuffs := (*ActiveBuffs)(runtime.world.Get(enemy, runtime.ids.activeBuffs))

	enemyPos.X = 10
	enemyPos.Y = 20
	enemyClass.Value = EnemyClassGunner
	moveState.StrafeSign = 1

	runtime.updateEnemyMovementLocked()
	baseSpeed := math.Hypot(enemyVel.X, enemyVel.Y)
	if baseSpeed <= 0 {
		t.Fatal("expected baseline enemy speed to be positive")
	}

	runtime.applyHitEventsLocked([]HitEvent{{
		Target:       enemy,
		OnHitEffects: runtime.collectOnHitEffectsLocked(player),
	}})
	if len(enemyBuffs.Items) != 1 {
		t.Fatalf("expected 1 active buff after chill hit, got %d", len(enemyBuffs.Items))
	}
	if got := enemyBuffs.Items[0].Stacks; got != 1 {
		t.Fatalf("expected chill to stay at 1 stack, got %d", got)
	}

	runtime.updateEnemyMovementLocked()
	chilledSpeed := math.Hypot(enemyVel.X, enemyVel.Y)

	if chilledSpeed >= baseSpeed {
		t.Fatalf("expected chilled speed %.6f to be below base speed %.6f", chilledSpeed, baseSpeed)
	}
	assertFloatClose(t, chilledSpeed, baseSpeed*enemyBuffs.Items[0].MoveSpeedMultiplier)
}

func TestSnapshotExportsEnemyBuffState(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	if !runtime.GrantPlayerSkill(playerID, "skill_toxic_rounds_1") {
		t.Fatal("expected granting poison skill to succeed")
	}

	player := runtime.playerEntities[playerID]
	runtime.spawnEnemyLocked()
	enemy := firstEnemyEntity(t, runtime)

	runtime.applyHitEventsLocked([]HitEvent{{
		Target:       enemy,
		OnHitEffects: runtime.collectOnHitEffectsLocked(player),
	}})

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	var enemyState *netproto.EntityState
	for _, entity := range snapshot.Entities {
		if entity.Kind == netproto.EntityKind_ENTITY_KIND_ENEMY {
			enemyState = entity
			break
		}
	}
	if enemyState == nil {
		t.Fatal("expected enemy state in snapshot")
	}
	if len(enemyState.Buffs) != 1 {
		t.Fatalf("expected 1 exported buff, got %d", len(enemyState.Buffs))
	}

	buff := enemyState.Buffs[0]
	if buff.Kind != netproto.BuffKind_BUFF_KIND_POISON {
		t.Fatalf("expected poison buff kind, got %v", buff.Kind)
	}
	if buff.Category != netproto.BuffCategory_BUFF_CATEGORY_DOT {
		t.Fatalf("expected poison buff category %v, got %v", netproto.BuffCategory_BUFF_CATEGORY_DOT, buff.Category)
	}
	if buff.RemainingFrames == 0 {
		t.Fatal("expected remaining_frames > 0")
	}
	if buff.TickIntervalFrames == 0 {
		t.Fatal("expected tick_interval_frames > 0")
	}
	if buff.TickFramesRemaining == 0 {
		t.Fatal("expected tick_frames_remaining > 0")
	}
	if buff.DamagePerTick <= 0 {
		t.Fatalf("expected damage_per_tick > 0, got %d", buff.DamagePerTick)
	}
	if buff.Stacks != 1 {
		t.Fatalf("expected exported poison stacks 1, got %d", buff.Stacks)
	}
	if buff.MaxStacks == 0 {
		t.Fatal("expected exported poison max_stacks > 0")
	}
}

func TestSnapshotExportsPlayerBuffState(t *testing.T) {
	runtime := NewRuntime(60)
	const playerID uint32 = 1

	runtime.AddPlayer(playerID)
	player := runtime.playerEntities[playerID]

	chillEffect := skillcfg.EffectConfig{
		Kind: skillcfg.EffectKindApplyStatusOnHit,
		ApplyStatusOnHit: &skillcfg.ApplyStatusOnHitConfig{
			Status:       skillcfg.StatusKindChill,
			Category:     skillcfg.BuffCategoryControl,
			StackingRule: skillcfg.BuffStackingRuleNone,
			Chill: &skillcfg.ChillStatusConfig{
				MoveSpeedMultiplier: 0.5,
				DurationSeconds:     1,
			},
		},
	}

	runtime.applyHitEventsLocked([]HitEvent{{
		Target:       player,
		OnHitEffects: []skillcfg.EffectConfig{chillEffect},
	}})

	snapshot := runtime.BuildSnapshotFor(playerID)
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	var playerState *netproto.EntityState
	for _, entity := range snapshot.Entities {
		if entity.Kind == netproto.EntityKind_ENTITY_KIND_PLAYER && entity.OwnerPlayerId == playerID {
			playerState = entity
			break
		}
	}
	if playerState == nil {
		t.Fatal("expected player state in snapshot")
	}
	if len(playerState.Buffs) != 1 {
		t.Fatalf("expected 1 exported buff, got %d", len(playerState.Buffs))
	}

	buff := playerState.Buffs[0]
	if buff.Kind != netproto.BuffKind_BUFF_KIND_CHILL {
		t.Fatalf("expected chill buff kind, got %v", buff.Kind)
	}
	if buff.Category != netproto.BuffCategory_BUFF_CATEGORY_CONTROL {
		t.Fatalf("expected chill buff category %v, got %v", netproto.BuffCategory_BUFF_CATEGORY_CONTROL, buff.Category)
	}
	if buff.RemainingFrames == 0 {
		t.Fatal("expected remaining_frames > 0")
	}
	if buff.MoveSpeedMultiplier >= 1 {
		t.Fatalf("expected move_speed_multiplier < 1, got %f", buff.MoveSpeedMultiplier)
	}
	if buff.Stacks != 1 || buff.MaxStacks != 1 {
		t.Fatalf("expected chill buff to export one non-stacking layer, got stacks=%d max=%d", buff.Stacks, buff.MaxStacks)
	}
}

func firstEnemyEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[EnemyClass]().With(generic.T[EnemyTag]())
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one enemy entity")
	}
	entity := query.Entity()
	if runtime.world.Has(entity, runtime.ids.enemySpawnState) {
		spawnState := (*EnemySpawnState)(runtime.world.Get(entity, runtime.ids.enemySpawnState))
		spawnState.RemainingFrames = 0
	}
	return entity
}

func firstRawEnemyEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[EnemyClass]().With(generic.T[EnemyTag]())
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one enemy entity")
	}
	return query.Entity()
}

func firstPlayerBulletEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[OwnerPlayerID]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one player bullet entity")
	}
	return query.Entity()
}

func firstEnemyBulletEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[Damage]().With(generic.T2[BulletTag, EnemyBulletTag]()...)
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one enemy bullet entity")
	}
	return query.Entity()
}

func firstHomingPlayerBulletEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[HomingProjectile]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one homing player bullet entity")
	}
	return query.Entity()
}

func firstSkillDropEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[SkillDrop]().With(generic.T2[PickupTag, SkillDropTag]()...)
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one skill drop entity")
	}
	return query.Entity()
}

func firstExpOrbEntity(t *testing.T, runtime *Runtime) ecs.Entity {
	t.Helper()

	filter := generic.NewFilter1[PickupValue]().With(generic.T2[PickupTag, ExpOrbTag]()...)
	query := filter.Query(&runtime.world)
	defer query.Close()
	if !query.Next() {
		t.Fatal("expected at least one exp orb entity")
	}
	return query.Entity()
}

func countPlayerBullets(runtime *Runtime) int {
	filter := generic.NewFilter1[Position]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&runtime.world)

	count := 0
	for query.Next() {
		_ = query.Get()
		count++
	}
	return count
}

func countEnemies(runtime *Runtime) int {
	filter := generic.NewFilter1[Health]().With(generic.T[EnemyTag]())
	query := filter.Query(&runtime.world)

	count := 0
	for query.Next() {
		health := query.Get()
		if health.Value > 0 {
			count++
		}
	}
	return count
}

func countSkillDrops(runtime *Runtime) int {
	filter := generic.NewFilter1[SkillDrop]().With(generic.T2[PickupTag, SkillDropTag]()...)
	query := filter.Query(&runtime.world)

	count := 0
	for query.Next() {
		_ = query.Get()
		count++
	}
	return count
}

func countHomingPlayerBullets(runtime *Runtime) int {
	filter := generic.NewFilter1[HomingProjectile]().With(generic.T2[BulletTag, PlayerBulletTag]()...)
	query := filter.Query(&runtime.world)

	count := 0
	for query.Next() {
		_ = query.Get()
		count++
	}
	return count
}

func countEnemyBullets(runtime *Runtime) int {
	filter := generic.NewFilter1[Position]().With(generic.T2[BulletTag, EnemyBulletTag]()...)
	query := filter.Query(&runtime.world)

	count := 0
	for query.Next() {
		_ = query.Get()
		count++
	}
	return count
}

func hasDuplicateStrings(items []string) bool {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item]; exists {
			return true
		}
		seen[item] = struct{}{}
	}
	return false
}

func assertFloatClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected %.6f, got %.6f", want, got)
	}
}

func findEntityStateByNetID(t *testing.T, snapshot *netproto.Snapshot, netID uint32) *netproto.EntityState {
	t.Helper()
	for _, entity := range snapshot.GetEntities() {
		if entity.GetNetId() == netID {
			return entity
		}
	}
	t.Fatalf("expected entity state with net id %d", netID)
	return nil
}
