// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package diff

import "testing"

// parentPath walks a change back to the element that contains it, which is how
// a removal is attributed to the right parent. The leading-slash case matters:
// a top-level path has no parent, and returning "/" instead of "" would
// attribute every root change to a node that does not exist.
func TestParentPath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/Document/GrpHdr/MsgId", "/Document/GrpHdr"},
		{"/Document/GrpHdr", "/Document"},
		{"/Document", ""},
		{"Document", ""},
		{"", ""},
	} {
		if got := parentPath(tc.in); got != tc.want {
			t.Errorf("parentPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
