package main

import (
	"testing"
)

// Test_apiConfig_Initialization verifies that apiConfig can be created
// and fileserverHits starts at 0
func Test_apiConfig_Initialization(t *testing.T) {
	cfg := apiConfig{}

	got := cfg.fileserverHits.Load()
	want := int32(0)

	if got != want {
		t.Errorf("fileserverHits initial value = %d, want %d", got, want)
	}
}
