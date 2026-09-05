package mapreduce

import "kvschool/internal/kv/memmap"

func newInMemoryOutput() *memmap.Store { return memmap.New() }
