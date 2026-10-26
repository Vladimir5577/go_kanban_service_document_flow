package service

import "testing"

func TestAssigneeDeltaNotifiesOnlyNew(t *testing.T) {
	added, removed := assigneeDelta([]int64{1, 2}, []int64{2, 3})
	if len(added) != 1 || added[0] != 3 {
		t.Fatalf("added = %v, want [3]", added)
	}
	if len(removed) != 1 || removed[0] != 1 {
		t.Fatalf("removed = %v, want [1]", removed)
	}

	added, removed = assigneeDelta(nil, []int64{4, 5})
	if len(added) != 2 || len(removed) != 0 {
		t.Fatalf("add = %v / %v", added, removed)
	}

	added, removed = assigneeDelta([]int64{4}, nil)
	if len(removed) != 1 || len(added) != 0 {
		t.Fatalf("remove = %v / %v", added, removed)
	}

	added, removed = assigneeDelta([]int64{7}, []int64{7})
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("same set = %v / %v", added, removed)
	}
}

func TestPositiveUniqueUserIDs(t *testing.T) {
	got := positiveUniqueUserIDs([]int64{0, 3, 3, -1, 5})
	if len(got) != 2 || got[0] != 3 || got[1] != 5 {
		t.Fatalf("got %v", got)
	}
}
