package capability

import (
	"context"
	"slices"
	"testing"
)

func TestPageForAndFinish(t *testing.T) {
	run := func(context.Context, Exec, Params) (Result, error) { return Result{}, nil }
	reg := NewRegistry().Register(
		&Capability{Name: "channels.test", Module: "channels", Kind: Write, Permission: "tenant.channels.manage", Apply: run},
		&Capability{Name: "channels.connect", Module: "channels", Kind: Write, Permission: "tenant.channels.manage", Apply: run},
		&Capability{Name: "channels.list", Module: "channels", Kind: Read, Permission: "tenant.channels.read", Run: run},
	)
	state := func(context.Context, Exec) (PageState, error) { return PageState{}, nil }
	reg.RegisterPages(
		&Page{Module: "channels", Routes: []string{"/channels"}, Permission: "tenant.channels.read", State: state},
		&Page{Module: "connect", Routes: []string{"/channels/connect"}, State: state},
	)
	has := func(perms ...string) func(string) bool {
		return func(p string) bool { return slices.Contains(perms, p) }
	}
	if p := reg.PageFor("/channels/0b6f", has("tenant.channels.read")); p == nil || p.Module != "channels" {
		t.Fatal("prefix")
	}
	if p := reg.PageFor("/channels/connect/email", has("tenant.channels.read")); p == nil || p.Module != "connect" {
		t.Fatal("longest prefix")
	}
	if p := reg.PageFor("/channels", has()); p != nil {
		t.Fatal("page shown without its permission")
	}
	if reg.PageFor("/channelsx", has("tenant.channels.read")) != nil {
		t.Fatal("not a path prefix")
	}
	st := PageState{Module: "channels",
		Missing: []Step{{Key: "connect", Capability: "channels.connect"}},
		Tests:   []Test{{Capability: "channels.test"}, {Capability: "unknown.test"}}}
	viewer := reg.Finish(st, Filter{Surface: SurfaceWeb, Has: has("tenant.channels.read")})
	if len(viewer.Tests) != 0 || viewer.Missing[0].Capability != "" || len(viewer.Actions) != 0 {
		t.Fatalf("viewer %+v", viewer)
	}
	manager := reg.Finish(st, Filter{Surface: SurfaceWeb, Has: has("tenant.channels.read", "tenant.channels.manage")})
	if len(manager.Tests) != 1 || manager.Missing[0].Capability != "channels.connect" || !slices.Equal(manager.Actions, []string{"channels.connect", "channels.test"}) {
		t.Fatalf("manager %+v", manager)
	}
}
