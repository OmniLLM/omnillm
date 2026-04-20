package vmodelrouting

import (
	"omnillm/internal/database"
	"testing"
)

func resetRoundRobinState() {
	rrMu.Lock()
	defer rrMu.Unlock()
	rrState = make(map[string]int)
}

func TestSelectUpstreamReturnsNilForEmptyInput(t *testing.T) {
	if got := SelectUpstream(nil, database.LbStrategyRoundRobin, "vm-1"); got != nil {
		t.Fatalf("expected nil for empty upstreams, got %#v", got)
	}
}

func TestOrderUpstreamsRoundRobinRotatesByVirtualModel(t *testing.T) {
	resetRoundRobinState()

	upstreams := []database.VirtualModelUpstreamRecord{
		{ProviderID: "p1", ModelID: "m1"},
		{ProviderID: "p2", ModelID: "m2"},
		{ProviderID: "p3", ModelID: "m3"},
	}

	first := OrderUpstreams(upstreams, database.LbStrategyRoundRobin, "vm-a")
	second := OrderUpstreams(upstreams, database.LbStrategyRoundRobin, "vm-a")
	third := OrderUpstreams(upstreams, database.LbStrategyRoundRobin, "vm-a")
	other := OrderUpstreams(upstreams, database.LbStrategyRoundRobin, "vm-b")

	if first[0].ProviderID != "p1" || second[0].ProviderID != "p2" || third[0].ProviderID != "p3" {
		t.Fatalf("unexpected round-robin order: first=%s second=%s third=%s", first[0].ProviderID, second[0].ProviderID, third[0].ProviderID)
	}
	if other[0].ProviderID != "p1" {
		t.Fatalf("expected separate round-robin state per virtual model, got %s", other[0].ProviderID)
	}
}

func TestOrderUpstreamsPriorityPreservesOrder(t *testing.T) {
	upstreams := []database.VirtualModelUpstreamRecord{
		{ProviderID: "p1", ModelID: "m1"},
		{ProviderID: "p2", ModelID: "m2"},
	}

	ordered := OrderUpstreams(upstreams, database.LbStrategyPriority, "vm")
	if len(ordered) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(ordered))
	}
	if ordered[0].ProviderID != "p1" || ordered[1].ProviderID != "p2" {
		t.Fatalf("expected priority order to be preserved, got %#v", ordered)
	}
}

func TestOrderUpstreamsReturnsCopyForUnknownStrategy(t *testing.T) {
	upstreams := []database.VirtualModelUpstreamRecord{{ProviderID: "p1", ModelID: "m1"}}
	ordered := OrderUpstreams(upstreams, database.LbStrategy("unknown"), "vm")
	if len(ordered) != 1 || ordered[0].ProviderID != "p1" {
		t.Fatalf("unexpected ordered result: %#v", ordered)
	}

	ordered[0].ProviderID = "changed"
	if upstreams[0].ProviderID != "p1" {
		t.Fatal("expected returned slice to be a copy")
	}
}

func TestMoveToFrontHandlesBounds(t *testing.T) {
	upstreams := []database.VirtualModelUpstreamRecord{
		{ProviderID: "p1"},
		{ProviderID: "p2"},
		{ProviderID: "p3"},
	}

	if got := moveToFront(upstreams, 0); got[0].ProviderID != "p1" {
		t.Fatalf("expected idx 0 to keep order, got %#v", got)
	}
	if got := moveToFront(upstreams, 10); got[0].ProviderID != "p1" {
		t.Fatalf("expected out-of-range idx to keep order, got %#v", got)
	}
	got := moveToFront(upstreams, 2)
	if got[0].ProviderID != "p3" || got[1].ProviderID != "p1" || got[2].ProviderID != "p2" {
		t.Fatalf("unexpected moveToFront result: %#v", got)
	}
}
