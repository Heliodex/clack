// SPDX-License-Identifier: Unlicense OR MIT

package op

import "gioui.org/internal/ops"

// Whole is every operation added to o so far, as a CallOp (Clack patch
// 31, CLACK_PATCHES.md): a finished frame drawn once more under a
// transform, as Clack's inspect tool renders the element it picked at a
// higher density than the window. Like a recorded macro, it is invalid
// once o is Reset.
func (o *Ops) Whole() CallOp {
	return CallOp{ops: &o.Internal, end: ops.PCFor(&o.Internal)}
}
