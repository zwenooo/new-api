package common

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type BodyStorage interface {
	io.ReadSeeker
	io.Closer
	Bytes() ([]byte, error)
	OpenReader() (io.ReadCloser, error)
	Size() int64
	IsDisk() bool
}

type ReusableBodyReader struct {
	storage BodyStorage
}

func NewReusableBodyReader(storage BodyStorage) *ReusableBodyReader {
	return &ReusableBodyReader{storage: storage}
}

func (r *ReusableBodyReader) Read(p []byte) (int, error) {
	if r == nil || r.storage == nil {
		return 0, io.EOF
	}
	return r.storage.Read(p)
}

func (r *ReusableBodyReader) ContentLength() int64 {
	if r == nil || r.storage == nil {
		return 0
	}
	return r.storage.Size()
}

func (r *ReusableBodyReader) GetBody() (io.ReadCloser, error) {
	if r == nil || r.storage == nil {
		return http.NoBody, nil
	}
	return r.storage.OpenReader()
}

var ErrStorageClosed = fmt.Errorf("body storage is closed")

type memoryStorage struct {
	data   []byte
	reader *bytes.Reader
	size   int64
	closed int32
	mu     sync.Mutex
}

func newMemoryStorage(data []byte) *memoryStorage {
	size := int64(len(data))
	IncrementMemoryBuffers(size)
	return &memoryStorage{
		data:   data,
		reader: bytes.NewReader(data),
		size:   size,
	}
}

func (m *memoryStorage) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Read(p)
}

func (m *memoryStorage) Seek(offset int64, whence int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Seek(offset, whence)
}

func (m *memoryStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		DecrementMemoryBuffers(m.size)
	}
	return nil
}

func (m *memoryStorage) Bytes() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, ErrStorageClosed
	}
	return m.data, nil
}

func (m *memoryStorage) OpenReader() (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, ErrStorageClosed
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

func (m *memoryStorage) Size() int64 {
	return m.size
}

func (m *memoryStorage) IsDisk() bool {
	return false
}

type diskStorage struct {
	file     *os.File
	filePath string
	size     int64
	closed   int32
	mu       sync.Mutex
}

func newDiskStorage(data []byte, cachePath string) (*diskStorage, error) {
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	n, err := file.Write(data)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	MarkDiskCacheFileActive(filePath)
	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     int64(n),
	}, nil
}

func createMemoryStorageFromReader(reader io.Reader, maxBytes int64, prefix []byte) (BodyStorage, error) {
	if int64(len(prefix)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}
	data := make([]byte, len(prefix))
	copy(data, prefix)

	remainingLimit := maxBytes - int64(len(data))
	if reader != nil {
		tail, err := io.ReadAll(io.LimitReader(reader, remainingLimit+1))
		if err != nil {
			return nil, err
		}
		if int64(len(tail)) > remainingLimit {
			return nil, ErrRequestBodyTooLarge
		}
		data = append(data, tail...)
	}
	return newMemoryStorage(data), nil
}

func fallbackDiskStorageToMemory(
	file *os.File,
	filePath string,
	written int64,
	unwritten []byte,
	reader io.Reader,
	maxBytes int64,
	diskErr error,
) (BodyStorage, error) {
	prefix := make([]byte, 0, int(written)+len(unwritten))
	if written > 0 {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close()
			_ = os.Remove(filePath)
			return nil, fmt.Errorf("failed to recover body after disk error: %w", err)
		}
		persisted := make([]byte, int(written))
		if _, err := io.ReadFull(file, persisted); err != nil {
			_ = file.Close()
			_ = os.Remove(filePath)
			return nil, fmt.Errorf("failed to recover body after disk error: %w", err)
		}
		prefix = append(prefix, persisted...)
	}
	if len(unwritten) > 0 {
		prefix = append(prefix, unwritten...)
	}
	_ = file.Close()
	_ = os.Remove(filePath)
	SysError(fmt.Sprintf("failed to create disk body storage, falling back to memory: %v", diskErr))
	return createMemoryStorageFromReader(reader, maxBytes, prefix)
}

func newDiskStorageFromReader(reader io.Reader, maxBytes int64, cachePath string) (BodyStorage, error) {
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		SysError(fmt.Sprintf("failed to create disk body storage, falling back to memory: %v", err))
		return createMemoryStorageFromReader(reader, maxBytes, nil)
	}

	written := int64(0)
	buf := make([]byte, 32*1024)
	for {
		nr, readErr := reader.Read(buf)
		if nr > 0 {
			if written+int64(nr) > maxBytes {
				_ = file.Close()
				_ = os.Remove(filePath)
				return nil, ErrRequestBodyTooLarge
			}
			chunk := buf[:nr]
			offset := 0
			for offset < len(chunk) {
				nw, writeErr := file.Write(chunk[offset:])
				if nw > 0 {
					written += int64(nw)
					offset += nw
				}
				if writeErr != nil {
					return fallbackDiskStorageToMemory(file, filePath, written, chunk[offset:], reader, maxBytes, writeErr)
				}
				if nw == 0 {
					return fallbackDiskStorageToMemory(file, filePath, written, chunk[offset:], reader, maxBytes, io.ErrShortWrite)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = file.Close()
			_ = os.Remove(filePath)
			return nil, readErr
		}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fallbackDiskStorageToMemory(file, filePath, written, nil, nil, maxBytes, err)
	}

	MarkDiskCacheFileActive(filePath)
	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     written,
	}, nil
}

func (d *diskStorage) Read(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Read(p)
}

func (d *diskStorage) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Seek(offset, whence)
}

func (d *diskStorage) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.CompareAndSwapInt32(&d.closed, 0, 1) {
		_ = d.file.Close()
		_ = os.Remove(d.filePath)
		DecrementDiskFiles(d.size)
		UnmarkDiskCacheFileActive(d.filePath)
	}
	return nil
}

func (d *diskStorage) Bytes() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return nil, ErrStorageClosed
	}

	currentPos, err := d.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if _, err := d.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	data := make([]byte, d.size)
	_, err = io.ReadFull(d.file, data)
	if err != nil {
		return nil, err
	}
	if _, err := d.file.Seek(currentPos, io.SeekStart); err != nil {
		return nil, err
	}
	return data, nil
}

func (d *diskStorage) OpenReader() (io.ReadCloser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return nil, ErrStorageClosed
	}
	return os.Open(d.filePath)
}

func (d *diskStorage) Size() int64 {
	return d.size
}

func (d *diskStorage) IsDisk() bool {
	return true
}

func CreateBodyStorage(data []byte) (BodyStorage, error) {
	size := int64(len(data))
	threshold := GetDiskCacheThresholdBytes()
	if IsDiskCacheEnabled() && size >= threshold && TryReserveDiskCache(size) {
		storage, err := newDiskStorage(data, GetDiskCachePath())
		if err != nil {
			ReleaseDiskCacheReservation(size)
			SysError(fmt.Sprintf("failed to create disk storage, falling back to memory: %v", err))
			return newMemoryStorage(data), nil
		}
		CommitDiskCacheReservation(size, storage.Size())
		return storage, nil
	}
	return newMemoryStorage(data), nil
}

func CreateBodyStorageFromReader(reader io.Reader, contentLength int64, maxBytes int64) (BodyStorage, error) {
	threshold := GetDiskCacheThresholdBytes()
	if IsDiskCacheEnabled() &&
		contentLength > 0 &&
		contentLength >= threshold &&
		TryReserveDiskCache(contentLength) {
		storage, err := newDiskStorageFromReader(reader, maxBytes, GetDiskCachePath())
		if err != nil {
			ReleaseDiskCacheReservation(contentLength)
			return nil, err
		}
		if storage.IsDisk() {
			CommitDiskCacheReservation(contentLength, storage.Size())
			IncrementDiskCacheHits()
		} else {
			ReleaseDiskCacheReservation(contentLength)
			IncrementMemoryCacheHits()
		}
		return storage, nil
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}

	storage, err := CreateBodyStorage(data)
	if err != nil {
		return nil, err
	}
	if storage.IsDisk() {
		IncrementDiskCacheHits()
	} else {
		IncrementMemoryCacheHits()
	}
	return storage, nil
}

func ReaderOnly(r io.Reader) io.Reader {
	return struct{ io.Reader }{r}
}

func CleanupOldCacheFiles() {
	_ = CleanupOldDiskCacheFiles(5 * time.Minute)
}
