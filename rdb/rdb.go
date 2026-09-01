package rdb

import (
	"bufio"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gosuda/nedis/db"
)

// Manager manages RDB persistence operations (SAVE, BGSAVE, LOAD).
type Manager struct {
	filePath  string
	bgsaveMu  sync.Mutex
	isBgsaving int32
}

// NewManager creates a new RDB Manager.
func NewManager(filePath string) *Manager {
	if filePath == "" {
		filePath = "dump.rdb"
	}
	return &Manager{filePath: filePath}
}

// Save synchronously dumps all shards to the RDB file.
func (m *Manager) Save(targetDB *db.ShardedDB) error {
	m.bgsaveMu.Lock()
	defer m.bgsaveMu.Unlock()

	tempPath := m.filePath + ".temp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		_ = os.Remove(tempPath)
	}()

	bw := bufio.NewWriterSize(file, 64*1024)
	enc := NewEncoder(bw)

	// 1. Header & Aux fields
	if err := enc.WriteHeader(); err != nil {
		return err
	}
	_ = enc.WriteStandardAuxFields()
	_ = enc.WriteSelectDB(0)

	// 2. Dump all shard entries
	err = targetDB.ForEachShardSnapshot(func(entries []db.DBEntry) error {
		for _, entry := range entries {
			if err := enc.WriteEntry(entry); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 3. Footer & Checksum
	if err := enc.WriteFooter(); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	_ = file.Close()

	// 4. Atomic file replacement
	return os.Rename(tempPath, m.filePath)
}

// BGSave launches an asynchronous background dump without blocking client commands.
func (m *Manager) BGSave(targetDB *db.ShardedDB, onComplete func(err error)) error {
	if !atomic.CompareAndSwapInt32(&m.isBgsaving, 0, 1) {
		return os.ErrExist // Background save already in progress
	}

	go func() {
		defer atomic.StoreInt32(&m.isBgsaving, 0)
		err := m.Save(targetDB)
		if onComplete != nil {
			onComplete(err)
		}
	}()

	return nil
}

// Load loads the RDB file into targetDB.
func (m *Manager) Load(targetDB *db.ShardedDB) error {
	file, err := os.Open(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	br := bufio.NewReaderSize(file, 64*1024)
	dec := NewDecoder(br)
	return dec.Load(targetDB)
}
