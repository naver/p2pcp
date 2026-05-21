// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/utils"
)

type LocalStorage struct {
	BaseDir string
}

func newLocalStorage(path string) (*LocalStorage, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	storage := LocalStorage{
		BaseDir: path,
	}

	return &storage, nil
}

func (storage *LocalStorage) GetBaseDirectory() string {
	return storage.BaseDir
}

func (storage *LocalStorage) Walk(path string, fn func(types.Entry) error) error {
	return scanLocalDirectory(filepath.Join(storage.BaseDir, path), "", fn)
}

func (storage *LocalStorage) Prepare() error {
	info, err := os.Stat(storage.BaseDir)
	if err == nil {
		// Path exists - use as-is if it's a writable directory
		if !info.IsDir() {
			return fmt.Errorf("destination path is not a directory: %s", storage.BaseDir)
		}
		if info.Mode().Perm()&0700 != 0700 {
			return fmt.Errorf("destination path requires read, write and execute permissions: %s", storage.BaseDir)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	// Path does not exist - create it
	return os.MkdirAll(storage.BaseDir, os.ModePerm)
}

func (storage *LocalStorage) PrepareEntry(entry types.Entry) error {
	fullPath := filepath.Join(storage.BaseDir, entry.GetName())

	switch e := entry.(type) {
	case *types.DirEntry:
		// Set directory permissions if the path already exists.
		os.Chmod(fullPath, 0700)

		err := os.MkdirAll(fullPath, os.ModePerm)
		if err != nil {
			utils.ErrorPrintf("Failed to create directory.: (%s) %s", e.Name, err.Error())
			return err
		}
		utils.DebugPrintf("Directory created.: %s", e.Name)

	case *types.SymlinkEntry:
		// Delete the symbolic link if it already exists.
		os.Remove(fullPath)

		err := os.Symlink(e.SymlinkTo, fullPath)
		if err != nil {
			utils.ErrorPrintf("Failed to create symbolic link.: (%s) %s", e.Name, err.Error())
			return err
		}
		utils.DebugPrintf("Symbolic link created.: %s -> %s", e.Name, e.SymlinkTo)

	case *types.EmptyFileEntry:
		err := func() error {
			// Set file permissions if the file already exists.
			os.Chmod(fullPath, 0600)

			// Files with a length of 0 do not require downloading; they only need to be created.
			localFile, err := os.OpenFile(fullPath, os.O_CREATE|os.O_TRUNC, os.ModePerm)
			if err != nil {
				return err
			}
			return localFile.Close()
		}()
		if err != nil {
			utils.ErrorPrintf("Failed to create empty file.: (%s) %s", e.Name, err.Error())
			return err
		}
		utils.DebugPrintf("Empty file created.: %s", e.Name)

	case *types.FileEntry:
		err := func() (err error) {
			// Set file permissions if the file already exists.
			os.Chmod(fullPath, 0600)

			var localFile *os.File
			localFile, err = os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, os.ModePerm)
			if err != nil {
				return err
			}
			defer func() {
				cerr := localFile.Close()
				if err == nil && cerr != nil {
					err = fmt.Errorf("close error: %w", cerr)
				}
			}()

			// Pre-create and resize the file to allocate space before transferring.
			return syscall.Ftruncate(int(localFile.Fd()), e.Size)
		}()
		if err != nil {
			utils.ErrorPrintf("Failed to create file.: (%s) %s", e.Name, err.Error())
			return err
		}

	default:
		return fmt.Errorf("unsupported entry type: %T", e)
	}

	return nil
}

func (storage *LocalStorage) CommitEntry(entry types.Entry) error {
	fullPath := filepath.Join(storage.BaseDir, entry.GetName())

	// Set permissions after transfer completion.
	// Directory permissions are applied at the end by iterating in reverse order.
	switch e := entry.(type) {
	case *types.DirEntry:
		err := os.Chmod(fullPath, e.Perm)
		if err != nil {
			return fmt.Errorf("failed to change directory permissions (%s): %s", e.Name, err.Error())
		}

	case *types.EmptyFileEntry:
		err := os.Chmod(fullPath, e.Perm)
		if err != nil {
			return fmt.Errorf("failed to change empty file permissions (%s): %s", e.Name, err.Error())
		}

	case *types.FileEntry:
		err := os.Chmod(fullPath, e.Perm)
		if err != nil {
			return fmt.Errorf("failed to change file permissions (%s): %s", e.Name, err.Error())
		}
	}

	return nil
}

func (storage *LocalStorage) Write(name string, offset int64, size int64, reader io.Reader) (n int64, err error) {
	var f *os.File
	f, err = os.OpenFile(filepath.Join(storage.BaseDir, name), os.O_WRONLY, os.ModePerm)
	if err != nil {
		return 0, err
	}

	defer func() {
		cerr := f.Close()
		if err == nil && cerr != nil {
			err = fmt.Errorf("close error: %w", cerr)
		}
	}()

	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		return 0, err
	}

	return io.CopyN(f, reader, size)
}

func (storage *LocalStorage) GetReader(name string, offset int64, size int64) (io.ReadCloser, error) {
	f, err := os.Open(filepath.Join(storage.BaseDir, name))
	if err != nil {
		return nil, err
	}

	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		f.Close()
		return nil, err
	}

	type limitReadCloser struct {
		io.Reader
		io.Closer
	}

	return &limitReadCloser{
		Reader: io.LimitReader(f, size),
		Closer: f,
	}, nil
}

func scanLocalDirectory(dirSrc string, subDir string, fn func(types.Entry) error) error {
	target := filepath.Join(dirSrc, subDir)
	localFiles, err := os.ReadDir(target)
	if err != nil {
		return err
	}

	for _, localFile := range localFiles {
		name := filepath.Join(subDir, localFile.Name())
		info, err := localFile.Info()
		if err != nil {
			utils.ErrorPrintf("Failed to get file information.: (%s) %s", name, err.Error())
			return err
		}

		mode := info.Mode()
		if mode.IsDir() {
			err := scanLocalDirectory(dirSrc, name, fn)
			if err != nil {
				return err
			}
			err = fn(&types.DirEntry{Name: name, Perm: mode.Perm()})
			if err != nil {
				return err
			}
		} else if mode&os.ModeSymlink != 0 {
			symlinkTo, err := os.Readlink(filepath.Join(dirSrc, name))
			if err != nil {
				utils.ErrorPrintf("Failed to get symbolic link information.: (%s) %s", name, err.Error())
				return err
			}
			err = fn(&types.SymlinkEntry{Name: name, SymlinkTo: symlinkTo})
			if err != nil {
				return err
			}
		} else if mode.IsRegular() {
			size := info.Size()
			if size == 0 {
				err = fn(&types.EmptyFileEntry{Name: name, Perm: mode.Perm()})
				if err != nil {
					return err
				}
			} else {
				err = fn(&types.FileEntry{Name: name, Size: size, Perm: mode.Perm()})
				if err != nil {
					return err
				}
			}
		} else {
			utils.Printf("Unsupported file mode. Ignoring.: (%s) %d", name, mode)
			continue
		}
	}

	return nil
}
