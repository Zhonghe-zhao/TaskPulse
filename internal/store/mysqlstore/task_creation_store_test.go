package mysqlstore

import "testing"

func TestNewTaskCreationStoreRejectsNilDatabase(t *testing.T) {
	if _, err := NewTaskCreationStore(nil); err == nil {
		t.Fatal("expected nil database to be rejected")
	}
}
