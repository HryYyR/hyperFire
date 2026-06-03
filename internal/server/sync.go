package server

import (
	"sort"

	"agentDemo/internal/netproto"
	"agentDemo/internal/session"

	"google.golang.org/protobuf/proto"
)

const (
	fullSnapshotIntervalSeconds = 2
	snapshotHistorySeconds      = 6
)

func (s *Server) buildSnapshotForSession(sess *session.Session, full *netproto.Snapshot) *netproto.Snapshot {
	if full == nil {
		return nil
	}

	currentEntities := indexEntityStates(full.Entities)
	ackedTick, lastFullTick, forceFull, baseState, hasBase := sess.SyncBaseline()

	sendFull := forceFull || !hasBase
	if !sendFull {
		interval := s.fullSnapshotIntervalTicks()
		if interval > 0 && (lastFullTick == 0 || full.GetTick() >= lastFullTick+interval) {
			sendFull = true
		}
	}

	entities := append([]*netproto.EntityState(nil), full.Entities...)
	var removedNetIDs []uint32
	baseTick := uint32(0)
	if !sendFull {
		entities, removedNetIDs = diffEntityStates(full.Entities, baseState.Entities)
		baseTick = ackedTick
	}

	update := &netproto.Snapshot{
		Tick:                  full.GetTick(),
		LastProcessedInputSeq: full.GetLastProcessedInputSeq(),
		Score:                 full.GetScore(),
		Running:               full.GetRunning(),
		Entities:              entities,
		Impacts:               append([]*netproto.ImpactEvent(nil), full.GetImpacts()...),
		PickupEvents:          append([]*netproto.PickupEvent(nil), full.GetPickupEvents()...),
		Horde:                 full.GetHorde(),
		PlayerLevel:           full.GetPlayerLevel(),
		PendingSkillChoices:   append([]*netproto.PendingSkillChoice(nil), full.GetPendingSkillChoices()...),
		BaseTick:              baseTick,
		IsFull:                sendFull,
		RemovedNetIds:         removedNetIDs,
	}

	sess.RecordSentSnapshot(full.GetTick(), currentEntities, sendFull, s.snapshotHistoryTicks())
	return update
}

func (s *Server) fullSnapshotIntervalTicks() uint32 {
	if s.cfg.TickHz == 0 {
		return 0
	}
	return s.cfg.TickHz * fullSnapshotIntervalSeconds
}

func (s *Server) snapshotHistoryTicks() uint32 {
	if s.cfg.TickHz == 0 {
		return 0
	}
	return s.cfg.TickHz * snapshotHistorySeconds
}

func indexEntityStates(entities []*netproto.EntityState) map[uint32]*netproto.EntityState {
	if len(entities) == 0 {
		return nil
	}

	result := make(map[uint32]*netproto.EntityState, len(entities))
	for _, entity := range entities {
		if entity == nil || entity.GetNetId() == 0 {
			continue
		}
		result[entity.GetNetId()] = entity
	}
	return result
}

func diffEntityStates(current []*netproto.EntityState, base map[uint32]*netproto.EntityState) ([]*netproto.EntityState, []uint32) {
	if len(current) == 0 && len(base) == 0 {
		return nil, nil
	}

	upserts := make([]*netproto.EntityState, 0, len(current))
	currentIndex := make(map[uint32]struct{}, len(current))
	for _, entity := range current {
		if entity == nil || entity.GetNetId() == 0 {
			continue
		}
		currentIndex[entity.GetNetId()] = struct{}{}
		baseEntity, ok := base[entity.GetNetId()]
		if !ok || !proto.Equal(entity, baseEntity) {
			upserts = append(upserts, entity)
		}
	}

	removed := make([]uint32, 0)
	for netID := range base {
		if _, ok := currentIndex[netID]; ok {
			continue
		}
		removed = append(removed, netID)
	}
	sort.Slice(removed, func(i, j int) bool {
		return removed[i] < removed[j]
	})
	return upserts, removed
}
