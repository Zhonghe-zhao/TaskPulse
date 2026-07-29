package mysqlstore

import "testing"

func TestNewTaskTransitionStoreRejectsNilDatabase(t *testing.T) {
	if _, err := NewTaskTransitionStore(nil); err == nil {
		t.Fatal("expected nil database to be rejected")
	}
}
