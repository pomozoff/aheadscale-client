package net

import "testing"

func TestAheadscaleMarkRangeIsSeparate(t *testing.T) {
	if ControlPlaneMark&0xFFFFFF00 == 0x1BD00 {
		t.Fatal("Aheadscale shares NetBird's firewall mark namespace")
	}
}
