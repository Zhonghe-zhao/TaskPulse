package mysqlstore

import "testing"

func TestNewEventStoreRejectsNilDatabase(t *testing.T) {
	if _, err := NewEventStore(nil); err == nil {
		t.Fatal("expected nil database to be rejected")
	}
}
