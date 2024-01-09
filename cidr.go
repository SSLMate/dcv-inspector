// Copyright (C) 2024 Opsmate, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// Except as contained in this notice, the name(s) of the above copyright
// holders shall not be used in advertising or otherwise to promote the
// sale, use or other dealings in this Software without prior written
// authorization.

package main

import (
	"github.com/kentik/patricia"
	"github.com/kentik/patricia/bool_tree"
	"net/netip"
)

type cidrSet struct {
	v4 *bool_tree.TreeV4
	v6 *bool_tree.TreeV6
}

func newCidrSet() *cidrSet {
	return &cidrSet{
		v4: bool_tree.NewTreeV4(),
		v6: bool_tree.NewTreeV6(),
	}
}

func (s *cidrSet) Has(addr netip.Addr) bool {
	found := false
	if addr.Is4() {
		found, _ = s.v4.FindDeepestTag(patricia.NewIPv4AddressFromBytes(addr.AsSlice(), 32))
	} else if addr.Is6() {
		found, _ = s.v6.FindDeepestTag(patricia.NewIPv6Address(addr.AsSlice(), 128))
	}
	return found
}

func (s *cidrSet) Add(prefix netip.Prefix) {
	if prefix.Addr().Is4() {
		s.v4.Set(patricia.NewIPv4AddressFromBytes(prefix.Addr().AsSlice(), uint(prefix.Bits())), true)
	} else if prefix.Addr().Is6() {
		s.v6.Set(patricia.NewIPv6Address(prefix.Addr().AsSlice(), uint(prefix.Bits())), true)
	}
}
