package server

import (
	"reflect"
	"testing"

	"agentDemo/internal/netproto"
	"agentDemo/internal/session"
)

func TestBuildSnapshotForSessionSendsFullThenDeltaAfterAck(t *testing.T) {
	srv := &Server{cfg: Config{TickHz: 60}}
	sess := session.New(1001, 1, "tester", nil)

	first := &netproto.Snapshot{
		Tick: 1,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
		Score: 1,
	}

	full := srv.buildSnapshotForSession(sess, first)
	if !full.GetIsFull() {
		t.Fatal("expected first sync packet to be full")
	}
	if got := full.GetBaseTick(); got != 0 {
		t.Fatalf("expected full snapshot base tick 0, got %d", got)
	}
	if got := len(full.GetEntities()); got != 1 {
		t.Fatalf("expected full snapshot to include 1 entity, got %d", got)
	}

	sess.SetInput(session.InputState{Seq: 1, AckedSnapshotTick: 1})

	second := &netproto.Snapshot{
		Tick:    2,
		Score:   2,
		Running: true,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
	}

	delta := srv.buildSnapshotForSession(sess, second)
	if delta.GetIsFull() {
		t.Fatal("expected second sync packet to be delta after ack")
	}
	if got := delta.GetBaseTick(); got != 1 {
		t.Fatalf("expected delta base tick 1, got %d", got)
	}
	if got := len(delta.GetEntities()); got != 0 {
		t.Fatalf("expected unchanged entity list to be empty in delta, got %d", got)
	}
	if got := delta.GetScore(); got != 2 {
		t.Fatalf("expected player score to still be included, got %d", got)
	}
}

func TestBuildSnapshotForSessionDiffsUpsertsAndRemovals(t *testing.T) {
	srv := &Server{cfg: Config{TickHz: 60}}
	sess := session.New(1001, 1, "tester", nil)

	full := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 1,
		Entities: []*netproto.EntityState{
			{NetId: 1, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
			{NetId: 2, Kind: netproto.EntityKind_ENTITY_KIND_ENEMY, Hp: 30},
		},
	})
	if !full.GetIsFull() {
		t.Fatal("expected initial full snapshot")
	}
	sess.SetInput(session.InputState{Seq: 1, AckedSnapshotTick: 1})

	delta := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 2,
		Entities: []*netproto.EntityState{
			{NetId: 1, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 95},
			{NetId: 3, Kind: netproto.EntityKind_ENTITY_KIND_BULLET_PLAYER, Radius: 0.5},
		},
	})
	if delta.GetIsFull() {
		t.Fatal("expected diff packet to stay delta")
	}

	gotUpserts := entityNetIDs(delta.GetEntities())
	wantUpserts := []uint32{1, 3}
	if !reflect.DeepEqual(gotUpserts, wantUpserts) {
		t.Fatalf("expected upserts %v, got %v", wantUpserts, gotUpserts)
	}

	gotRemoved := append([]uint32(nil), delta.GetRemovedNetIds()...)
	wantRemoved := []uint32{2}
	if !reflect.DeepEqual(gotRemoved, wantRemoved) {
		t.Fatalf("expected removed ids %v, got %v", wantRemoved, gotRemoved)
	}
}

func TestBuildSnapshotForSessionFallsBackToFullWhenBaseMissing(t *testing.T) {
	srv := &Server{cfg: Config{TickHz: 60}}
	sess := session.New(1001, 1, "tester", nil)

	first := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 1,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
	})
	if !first.GetIsFull() {
		t.Fatal("expected initial full snapshot")
	}

	sess.SetInput(session.InputState{Seq: 1, AckedSnapshotTick: 999})

	update := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 2,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 90},
		},
	})
	if !update.GetIsFull() {
		t.Fatal("expected missing baseline to trigger full snapshot")
	}
	if got := update.GetBaseTick(); got != 0 {
		t.Fatalf("expected fallback full snapshot base tick 0, got %d", got)
	}
}

func TestBuildSnapshotForSessionSendsPeriodicFullSnapshot(t *testing.T) {
	srv := &Server{cfg: Config{TickHz: 60}}
	sess := session.New(1001, 1, "tester", nil)

	first := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 1,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
	})
	if !first.GetIsFull() {
		t.Fatal("expected initial full snapshot")
	}
	sess.SetInput(session.InputState{Seq: 1, AckedSnapshotTick: 1})

	update := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 121,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
	})
	if !update.GetIsFull() {
		t.Fatal("expected periodic full snapshot after full-sync interval")
	}
}

func TestBuildSnapshotForSessionAlwaysIncludesPickupEvents(t *testing.T) {
	srv := &Server{cfg: Config{TickHz: 60}}
	sess := session.New(1001, 1, "tester", nil)

	first := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 1,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
	})
	if !first.GetIsFull() {
		t.Fatal("expected initial full snapshot")
	}

	sess.SetInput(session.InputState{Seq: 1, AckedSnapshotTick: 1})

	update := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 2,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
		PickupEvents: []*netproto.PickupEvent{
			{
				CollectorNetId:    10,
				CollectorKind:     netproto.EntityKind_ENTITY_KIND_PLAYER,
				CollectorPlayerId: 1,
				PickupNetId:       20,
				PickupKind:        netproto.EntityKind_ENTITY_KIND_PICKUP_EXP,
				ExpValue:          5,
				Granted:           true,
			},
		},
	})
	if update.GetIsFull() {
		t.Fatal("expected delta snapshot")
	}
	if got := len(update.GetPickupEvents()); got != 1 {
		t.Fatalf("expected 1 pickup event in delta snapshot, got %d", got)
	}
	if got := update.GetPickupEvents()[0].GetPickupNetId(); got != 20 {
		t.Fatalf("expected pickup event net id 20, got %d", got)
	}
}

func TestBuildSnapshotForSessionAlwaysIncludesHordeStatus(t *testing.T) {
	srv := &Server{cfg: Config{TickHz: 60}}
	sess := session.New(1001, 1, "tester", nil)

	first := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 1,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
	})
	if !first.GetIsFull() {
		t.Fatal("expected initial full snapshot")
	}

	sess.SetInput(session.InputState{Seq: 1, AckedSnapshotTick: 1})

	update := srv.buildSnapshotForSession(sess, &netproto.Snapshot{
		Tick: 2,
		Entities: []*netproto.EntityState{
			{NetId: 10, Kind: netproto.EntityKind_ENTITY_KIND_PLAYER, Hp: 100},
		},
		Horde: &netproto.HordeStatus{
			Value:           5,
			Threshold:       20,
			Active:          true,
			RemainingFrames: 300,
		},
	})
	if update.GetHorde() == nil {
		t.Fatal("expected horde status to be forwarded")
	}
	if got := update.GetHorde().GetValue(); got != 5 {
		t.Fatalf("expected horde value 5, got %d", got)
	}
	if !update.GetHorde().GetActive() {
		t.Fatal("expected horde active state to be forwarded")
	}
}

func entityNetIDs(entities []*netproto.EntityState) []uint32 {
	result := make([]uint32, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		result = append(result, entity.GetNetId())
	}
	return result
}
