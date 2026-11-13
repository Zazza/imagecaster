package sd

import (
	"fmt"
	"time"
)

type MemoryPool struct {
	Free  float64 `json:"free"`
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
}

type CUDAMemory struct {
	System MemoryPool `json:"system"`
}

type MemoryStats struct {
	RAM  MemoryPool  `json:"ram"`
	CUDA *CUDAMemory `json:"cuda,omitempty"`
}

func (c *Client) MemoryInfo() (*MemoryStats, error) {
	var stats MemoryStats
	if err := c.get("/memory", 3*time.Second, &stats); err != nil {
		return nil, fmt.Errorf("memory info: %w", err)
	}
	return &stats, nil
}
