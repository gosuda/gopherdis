package aof

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosuda/gopherdis/commands"
	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/parser"
)

// FsyncPolicy defines the disk synchronization strategy.
type FsyncPolicy int

const (
	FsyncNo       FsyncPolicy = 0 // Let OS decide when to flush
	FsyncEverySec FsyncPolicy = 1 // Flush every second in background
	FsyncAlways   FsyncPolicy = 2 // Flush on every write command
)

// AOF manages append-only file persistence, double-buffering, and background rewriting.
type AOF struct {
	filePath    string
	file        *os.File
	fsyncPolicy FsyncPolicy
	currentSize int64

	doubleBuf *DoubleBuffer

	rewriteMu   sync.Mutex
	isRewriting bool
	rewriteBuf  [][]byte

	cronStopCh  chan struct{}
	cronRunning bool
	cronMu      sync.Mutex
}

// OpenAOF initializes or opens an AOF file with the specified fsync policy.
func OpenAOF(filePath string, policy FsyncPolicy) (*AOF, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	a := &AOF{
		filePath:    filePath,
		file:        file,
		fsyncPolicy: policy,
		currentSize: info.Size(),
		doubleBuf:   NewDoubleBuffer(file, DefaultBufferSize),
	}

	if policy == FsyncEverySec {
		a.StartFsyncCron(time.Second)
	}

	return a, nil
}

// CurrentSize returns the current AOF file size in bytes.
func (a *AOF) CurrentSize() int64 {
	return atomic.LoadInt64(&a.currentSize)
}

// Feed receives a write command and writes it into the lock-free double buffer.
func (a *AOF) Feed(argv [][]byte) error {
	if len(argv) == 0 {
		return nil
	}

	encoded := encodeCommand(argv)

	// 1. Lock-free append via DoubleBuffer
	n, err := a.doubleBuf.Write(encoded)
	if err != nil {
		return err
	}
	atomic.AddInt64(&a.currentSize, int64(n))

	// 2. If rewrite is active, append to rewrite buffer
	a.rewriteMu.Lock()
	if a.isRewriting {
		clonedArgv := make([][]byte, len(argv))
		for i, arg := range argv {
			clonedArgv[i] = bytes.Clone(arg)
		}
		a.rewriteBuf = append(a.rewriteBuf, encodeCommand(clonedArgv))
	}
	a.rewriteMu.Unlock()

	// 3. If policy is FsyncAlways, flush double buffer and sync immediately
	if a.fsyncPolicy == FsyncAlways {
		if err := a.Flush(); err != nil {
			return err
		}
		return a.Sync()
	}

	return nil
}

// Flush forces swapping and flushing of the double buffer.
func (a *AOF) Flush() error {
	if a.doubleBuf == nil {
		return nil
	}
	return a.doubleBuf.Flush()
}

// Sync forces physical disk sync (fsync).
func (a *AOF) Sync() error {
	if err := a.Flush(); err != nil {
		return err
	}
	if a.file != nil {
		return a.file.Sync()
	}
	return nil
}

// StartFsyncCron starts background cron for FsyncEverySec.
func (a *AOF) StartFsyncCron(interval time.Duration) {
	a.cronMu.Lock()
	defer a.cronMu.Unlock()

	if a.cronRunning {
		return
	}
	a.cronStopCh = make(chan struct{})
	a.cronRunning = true

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = a.Sync()
			case <-a.cronStopCh:
				return
			}
		}
	}()
}

// StopFsyncCron stops background cron.
func (a *AOF) StopFsyncCron() {
	a.cronMu.Lock()
	defer a.cronMu.Unlock()

	if !a.cronRunning {
		return
	}
	close(a.cronStopCh)
	a.cronRunning = false
}

// Rewrite performs BGREWRITEAOF by taking lightweight shard snapshots (minimizing lock hold time),
// streaming data to a temporary file, merging incoming write buffers, and atomically replacing the AOF.
func (a *AOF) Rewrite(targetDB *db.ShardedDB) error {
	tempPath := a.filePath + ".temp"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	// 1. Enable rewrite buffering for incoming writes
	a.rewriteMu.Lock()
	a.isRewriting = true
	a.rewriteBuf = make([][]byte, 0, 1024)
	a.rewriteMu.Unlock()

	bufWriter := bufio.NewWriterSize(tempFile, 64*1024)

	// 2. Iterate shards with minimal lock hold time
	err = targetDB.ForEachShardSnapshot(func(entries []db.DBEntry) error {
		for _, entry := range entries {
			if err := writeEntryToRESP(bufWriter, entry); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		a.rewriteMu.Lock()
		a.isRewriting = false
		a.rewriteBuf = nil
		a.rewriteMu.Unlock()
		return err
	}

	// 3. Drain and flush rewrite buffer
	a.rewriteMu.Lock()
	diffCmds := a.rewriteBuf
	a.rewriteBuf = nil
	a.isRewriting = false
	a.rewriteMu.Unlock()

	for _, cmdBytes := range diffCmds {
		if _, err := bufWriter.Write(cmdBytes); err != nil {
			return err
		}
	}

	if err := bufWriter.Flush(); err != nil {
		return err
	}

	if err := tempFile.Sync(); err != nil {
		return err
	}
	_ = tempFile.Close()

	// 4. Flush active double buffer before swapping file descriptor
	_ = a.Flush()

	_ = a.doubleBuf.Close()
	_ = a.file.Close()

	if err := os.Rename(tempPath, a.filePath); err != nil {
		return err
	}

	newFile, err := os.OpenFile(a.filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	info, err := newFile.Stat()
	if err != nil {
		newFile.Close()
		return err
	}

	a.file = newFile
	a.doubleBuf = NewDoubleBuffer(newFile, DefaultBufferSize)
	atomic.StoreInt64(&a.currentSize, info.Size())

	return nil
}

// Load replays commands from the AOF file into the target database.
func (a *AOF) Load(targetDB *db.ShardedDB) error {
	_ = a.Flush()

	file, err := os.Open(a.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	ctx := &commands.Context{DB: targetDB}

	for {
		argv, err := parser.ParseRequest(reader, nil)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return fmt.Errorf("aof parse error: %w", err)
		}
		if len(argv) == 0 {
			continue
		}

		commands.DefaultTable.Execute(ctx, argv)
	}

	return nil
}

// Close closes the AOF file and terminates background cron jobs.
func (a *AOF) Close() error {
	a.StopFsyncCron()
	if a.doubleBuf != nil {
		_ = a.doubleBuf.Close()
	}
	if a.file != nil {
		_ = a.file.Sync()
		return a.file.Close()
	}
	return nil
}
