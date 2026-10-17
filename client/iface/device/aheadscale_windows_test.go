package device

import "testing"

func TestAheadscaleWindowsAdapterDoesNotAdoptNetBirdGUID(t *testing.T) {
	guid, err := getGUID()
	if err != nil {
		t.Fatal(err)
	}
	if guid.String() == "{F2F29E61-D91F-4D76-8151-119B20C4BDEB}" {
		t.Fatal("Aheadscale would reuse NetBird's adapter identity")
	}
}
