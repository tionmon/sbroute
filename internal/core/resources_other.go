//go:build !linux

package core

import "runtime"

func residentMemory() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys - m.HeapReleased
}
func memoryLimit() int64                { return 0 }
func cgroupMemory() (uint64, uint64)    { return 0, 0 }
func fileDescriptors() (uint64, uint64) { return 0, 0 }
func processCPUSeconds() float64        { return 0 }
