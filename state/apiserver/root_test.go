// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/rpc/rpcreflect"
	"launchpad.net/juju-core/state/apiserver"
)

type rootSuite struct{}

var _ = gc.Suite(&rootSuite{})

var allowedDiscardedMethods = []string{
	"AuthClient",
	"AuthEnvironManager",
	"AuthMachineAgent",
	"AuthOwner",
	"AuthUnitAgent",
	"GetAuthEntity",
	"GetAuthTag",
}

func (*rootSuite) TestDiscardedAPIMethods(c *gc.C) {
	t := rpcreflect.TypeOf(apiserver.RootType)
	// We must have some root-level methods.
	c.Assert(t.MethodNames(), gc.Not(gc.HasLen), 0)
	c.Assert(t.DiscardedMethods(), gc.DeepEquals, allowedDiscardedMethods)

	for _, name := range t.MethodNames() {
		m, err := t.Method(name)
		c.Assert(err, gc.IsNil)
		// We must have some methods on every object returned
		// by a root-level method.
		c.Assert(m.ObjType.MethodNames(), gc.Not(gc.HasLen), 0)
		// We don't allow any methods that don't implement
		// an RPC entry point.
		c.Assert(m.ObjType.DiscardedMethods(), gc.HasLen, 0)
	}
}

type testKiller struct {
	killed time.Time
}

func (k *testKiller) Kill() {
	k.killed = time.Now()
}

func (r *rootSuite) TestPingTimeout(c *gc.C) {
	killer := &testKiller{}
	pinger, err := apiserver.NewSrvPinger(killer, 5*time.Millisecond)
	c.Assert(err, gc.IsNil)
	for i := 0; i < 10; i++ {
		time.Sleep(time.Millisecond)
		pinger.Ping()
	}
	// Expect killer.killed to be set 5ms after last ping.
	broken := time.Now()
	time.Sleep(10 * time.Millisecond)
	killDiff := killer.killed.Sub(broken).Nanoseconds() / 1000000
	c.Assert(killDiff >= 5 && killDiff <= 6, gc.Equals, true)
}
