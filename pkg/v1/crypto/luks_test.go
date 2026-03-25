package crypto

import "testing"

func TestLUKSRepositoryStruct(t *testing.T) {
	repo := &LUKSRepository{
		DevicePath: "/dev/test",
		MountPath:  "/mnt/test",
		DeviceName: "test-continuity",
	}

	if repo.DevicePath != "/dev/test" {
		t.Errorf("Expected DevicePath=/dev/test, got %s", repo.DevicePath)
	}
}
