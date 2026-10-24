package servidor

import "testing"

type fakeConn struct{ closed bool }

func (f *fakeConn) Close() error { f.closed = true; return nil }

// A connection opened while pgocode is closing (after closeAll) is closed at once, not kept where nobody closes it.
func TestSessionConnsOpenAfterClose(t *testing.T) {
	var c sessionConns[*fakeConn]
	a := &fakeConn{}
	if !c.open(a, 1, "s", "luis") {
		t.Fatal("open")
	}
	c.closeAll()
	if !a.closed {
		t.Error("closeAll leaves a connection open")
	}
	b := &fakeConn{}
	if c.open(b, 1, "s", "luis") || !b.closed || c.get(1, "s") != nil {
		t.Error("a connection opened while closing is kept")
	}
}
