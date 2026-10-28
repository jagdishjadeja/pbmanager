package platform

import "fmt"

const MiB int64 = 1 << 20
const DefaultMemoryMiB = 512
const MinMemoryMiB = 128
const MaxMemoryMiB = 32768
const MemoryStepMiB = 64

func MemoryLimit(mib int) (int64, error) {
	if mib < MinMemoryMiB || mib > MaxMemoryMiB || mib%MemoryStepMiB != 0 {
		return 0, fmt.Errorf("choose RAM between 128 and 32768 MiB in steps of 64 MiB")
	}
	return int64(mib) * MiB, nil
}
