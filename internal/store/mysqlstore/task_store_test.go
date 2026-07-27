package mysqlstore

import "testing"

func TestNewTaskStoreRejectsNilDatabase(t *testing.T) {
	if _, err := NewTaskStore(nil); err == nil {
		t.Fatal("expected nil database to be rejected")
	}
}
