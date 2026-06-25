package relaycore

import "testing"

// Multi-room: peers()/to are scoped to a room so one relay carries many independent calls.
// A peer that never joins stays in the default room "" (back-compat with old single-room clients).
func TestHubRoomScoping(t *testing.T) {
	h := newHub()
	// add(nil): the routing methods under test never touch the data channel (only send() does), so nil is fine.
	a := h.add(nil)
	b := h.add(nil)
	c := h.add(nil)
	h.setRoom(a, "alpha")
	h.setRoom(b, "alpha")
	h.setRoom(c, "beta")

	// Discovery is room-scoped: alpha sees {a,b}, beta sees only {c}.
	if got := h.idsInRoom("alpha"); len(got) != 2 {
		t.Fatalf("alpha room: want 2 ids, got %v", got)
	}
	if got := h.idsInRoom("beta"); len(got) != 1 || got[0] != c.id {
		t.Fatalf("beta room: want [%d], got %v", c.id, got)
	}

	// Routing is isolated: a→b (same room) resolves; a→c (other room) does NOT, even with c's id known.
	if h.dstInRoom(b.id, "alpha") == nil {
		t.Fatal("a should reach b within room alpha")
	}
	if h.dstInRoom(c.id, "alpha") != nil {
		t.Fatal("a must NOT reach c across rooms (isolation)")
	}

	// Back-compat: a peer that never joined stays in the default room "" and is isolated from named rooms.
	d := h.add(nil)
	if got := h.idsInRoom(""); len(got) != 1 || got[0] != d.id {
		t.Fatalf("default room: want [%d], got %v", d.id, got)
	}
	if got := h.idsInRoom("alpha"); len(got) != 2 {
		t.Fatalf("default-room peer must not leak into alpha; alpha=%v", got)
	}
}
