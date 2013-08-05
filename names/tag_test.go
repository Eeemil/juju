// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type tagSuite struct{}

var _ = gc.Suite(&tagSuite{})

var tagKindTests = []struct {
	tag  string
	kind string
	err  string
}{
	{tag: "unit-wordpress-42", kind: names.UnitTagKind},
	{tag: "machine-42", kind: names.MachineTagKind},
	{tag: "service-foo", kind: names.ServiceTagKind},
	{tag: "environment-42", kind: names.EnvironTagKind},
	{tag: "user-admin", kind: names.UserTagKind},
	{tag: "foo", err: `"foo" is not a valid tag`},
	{tag: "unit", err: `"unit" is not a valid tag`},
}

func (*tagSuite) TestTagKind(c *gc.C) {
	for i, test := range tagKindTests {
		c.Logf("test %d: %q -> %q", i, test.tag, test.kind)
		kind, err := names.TagKind(test.tag)
		if test.err == "" {
			c.Assert(test.kind, gc.Equals, kind)
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(kind, gc.Equals, "")
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

var parseTagTests = []struct {
	tag string
	expectKind string
	resultId string
	resultErr string
}{{
	tag: "machine-10",
	expectKind: names.MachineTagKind,
	resultId: "10",
}, {
	tag: "machine-10-lxc-1",
	expectKind: names.MachineTagKind,
	resultId: "10/lxc/1",
}, {
	tag: "foo",
	expectKind: names.MachineTagKind,
	resultErr:  `"foo" is not a valid machine tag`,
}, {
	tag: "machine-#",
	expectKind: names.MachineTagKind,
	resultErr: `"machine-#" is not a valid machine tag`,
},
}

var makeTag = map[string]func(id string) string {
	names.MachineTagKind: names.MachineTag,
	names.UnitTagKind: names.UnitTag,
	names.ServiceTagKind: names.ServiceTag,
	// TODO environment and user, when they have Tag functions.
}

func (*tagSuite) TestParseTag(c *gc.C) {
	for i, test := range parseTagTests {
		c.Logf("test %d. %q expectKind %q", i, test.tag, test.expectKind)
		kind, id, err := names.ParseTag(test.tag, test.expectKind)
		if test.resultErr != "" {
			c.Assert(err, gc.ErrorMatches, test.resultErr)
			c.Assert(kind, gc.Equals, "")
			c.Assert(id, gc.Equals, "")
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(id, gc.Equals, test.resultId)
			if test.expectKind != "" {
				c.Assert(kind, gc.Equals, test.expectKind)
			} else {
				expectKind, err := names.TagKind(test.tag)
				c.Assert(err, gc.IsNil)
				c.Assert(kind, gc.Equals, expectKind)
			}
			// Check that it's reversible.
			if f := makeTag[kind]; f != nil {
				reversed := f(id)
				c.Assert(reversed, gc.Equals, test.tag)
			}
		}
	}
}
