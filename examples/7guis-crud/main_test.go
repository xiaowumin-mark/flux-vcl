package main

import "testing"

func TestFilteredPeopleKeepsStableBusinessIDs(t *testing.T) {
	people := []person{
		{ID: 41, Name: "Ada", Surname: "Lovelace"},
		{ID: 73, Name: "Grace", Surname: "Hopper"},
		{ID: 105, Name: "Edsger", Surname: "Dijkstra"},
	}

	filtered := filteredPeople(people, "h")
	if len(filtered) != 1 || filtered[0].ID != 73 {
		t.Fatalf("filtered IDs = %+v, want stable ID 73", filtered)
	}
	if index := personIndexByID(people, filtered[0].ID); index != 1 {
		t.Fatalf("stable ID resolved to index %d, want 1", index)
	}
}

func TestDirectoryCloneOwnsPeopleSlice(t *testing.T) {
	original := directoryState{People: []person{{ID: 1, Name: "Ada", Surname: "Lovelace"}}}
	copy := original.clone()
	copy.People[0].Name = "Changed"
	if original.People[0].Name != "Ada" {
		t.Fatalf("clone mutated original: %+v", original.People[0])
	}
}
