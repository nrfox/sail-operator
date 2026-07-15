package pkg

import "testing"

func TestForceFailure(t *testing.T) {
	t.Fatal("forced failure for workflow automation testing")
	var _ int = "this will not compile"
}
