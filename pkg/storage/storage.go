package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LinkBatch struct {
	BatchID   int64    `json:"batch_id"`
	URLs      []string `json:"urls"`
	Results   []any    `json:"results"`
	CreatedAt string   `json:"created_at"`
	Status    string   `json:"status"`
	Error     string   `json:"error,omitempty"`
}

type Storage struct {
	dataDir string
	mu      sync.RWMutex
	batches map[int64]*LinkBatch
	nextID  int64
}

func NewStorage(dataDir string) (*Storage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	s := &Storage{
		dataDir: dataDir,
		batches: make(map[int64]*LinkBatch),
		nextID:  1,
	}

	if err := s.loadBatches(); err != nil {
		return nil, fmt.Errorf("failed to load batches: %w", err)
	}

	return s, nil
}

func (s *Storage) SaveBatch(urls []string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch := &LinkBatch{
		BatchID:   s.nextID,
		URLs:      urls,
		CreatedAt: time.Now().Format(time.RFC3339),
		Status:    "pending",
		Results:   make([]interface{}, 0),
	}

	s.batches[s.nextID] = batch
	batchID := s.nextID
	s.nextID++

	s.persistBatch(batch)
	s.persistNextID()

	return batchID
}

func (s *Storage) GetBatch(batchID int64) (*LinkBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch, exists := s.batches[batchID]
	if !exists {
		return nil, fmt.Errorf("batch %d not found", batchID)
	}

	return batch, nil
}

func (s *Storage) UpdateBatch(batchID int64, results []interface{}, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	batch, exists := s.batches[batchID]
	if !exists {
		return fmt.Errorf("batch %d not found", batchID)
	}

	batch.Results = results
	batch.Status = status

	s.persistBatch(batch)

	return nil
}

func (s *Storage) GetBatches(batchIDs []int64) ([]*LinkBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var batches []*LinkBatch
	for _, id := range batchIDs {
		if batch, exists := s.batches[id]; exists {
			batches = append(batches, batch)
		}
	}

	return batches, nil
}

func (s *Storage) ListPendingBatches() []*LinkBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var pending []*LinkBatch
	for _, batch := range s.batches {
		if batch.Status == "pending" || batch.Status == "processing" {
			pending = append(pending, batch)
		}
	}

	return pending
}

func (s *Storage) ListAllBatches() []*LinkBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []*LinkBatch
	for _, batch := range s.batches {
		all = append(all, batch)
	}

	return all
}

func (s *Storage) loadBatches() error {
	nextIDFile := filepath.Join(s.dataDir, "next_id.json")
	if data, err := os.ReadFile(nextIDFile); err == nil {
		var nextID int64
		if err := json.Unmarshal(data, &nextID); err == nil {
			s.nextID = nextID
		}
	}

	files, err := os.ReadDir(s.dataDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" || file.Name() == "next_id.json" {
			continue
		}

		filePath := filepath.Join(s.dataDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var batch LinkBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			continue
		}

		s.batches[batch.BatchID] = &batch
	}

	return nil
}

func (s *Storage) persistBatch(batch *LinkBatch) error {
	filePath := filepath.Join(s.dataDir, fmt.Sprintf("batch_%d.json", batch.BatchID))
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func (s *Storage) persistNextID() error {
	filePath := filepath.Join(s.dataDir, "next_id.json")
	data, err := json.MarshalIndent(s.nextID, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func (s *Storage) WaitForCompletion(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.mu.RLock()
			pendingCount := 0
			for _, batch := range s.batches {
				if batch.Status == "pending" || batch.Status == "processing" {
					pendingCount++
				}
			}
			s.mu.RUnlock()

			if pendingCount > 0 {
				fmt.Printf("Timeout: %d batches still processing\n", pendingCount)
			}
			return
		case <-ticker.C:
			s.mu.RLock()
			allDone := true
			for _, batch := range s.batches {
				if batch.Status == "pending" || batch.Status == "processing" {
					allDone = false
					break
				}
			}
			s.mu.RUnlock()

			if allDone {
				fmt.Println("All pending batches completed")
				return
			}
		}
	}
}
