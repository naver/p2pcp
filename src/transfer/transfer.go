// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/utils"
)

func SetupTransfer(chunkManager *ChunkManager, storage storage.Storage) error {
	// Create dirs first, then finally create files
	err := storage.Prepare()
	if err != nil {
		return err
	}

	for i := range chunkManager.manifest.Dirs {
		err := storage.PrepareEntry(&chunkManager.manifest.Dirs[i])
		if err != nil {
			return err
		}
	}
	for i := range chunkManager.manifest.Symlinks {
		err := storage.PrepareEntry(&chunkManager.manifest.Symlinks[i])
		if err != nil {
			return err
		}
	}
	for i := range chunkManager.manifest.EmptyFiles {
		err := storage.PrepareEntry(&chunkManager.manifest.EmptyFiles[i])
		if err != nil {
			return err
		}
	}
	for i := range chunkManager.manifest.Files {
		err := storage.PrepareEntry(&chunkManager.manifest.Files[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func CompleteTransfer(chunkManager *ChunkManager, storage storage.Storage) error {
	// Complete files first, then finally complete dirs
	for i := range chunkManager.manifest.Files {
		err := storage.CommitEntry(&chunkManager.manifest.Files[i])
		if err != nil {
			return err
		}
	}
	for i := range chunkManager.manifest.EmptyFiles {
		err := storage.CommitEntry(&chunkManager.manifest.EmptyFiles[i])
		if err != nil {
			return err
		}
	}
	for i := range chunkManager.manifest.Symlinks {
		err := storage.CommitEntry(&chunkManager.manifest.Symlinks[i])
		if err != nil {
			return err
		}
	}
	for i := range chunkManager.manifest.Dirs {
		err := storage.CommitEntry(&chunkManager.manifest.Dirs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func VerifyTransfer(chunkManager *ChunkManager, storage storage.Storage) bool {
	dirMap := map[string]types.DirEntry{}
	symlinkMap := map[string]types.SymlinkEntry{}
	emptyFileMap := map[string]types.EmptyFileEntry{}
	fileMap := map[string]*types.FileEntry{}

	err := storage.Walk("", func(entry types.Entry) error {
		switch e := entry.(type) {
		case *types.DirEntry:
			dirMap[e.Name] = *e
		case *types.SymlinkEntry:
			symlinkMap[e.Name] = *e
		case *types.EmptyFileEntry:
			emptyFileMap[e.Name] = *e
		case *types.FileEntry:
			fileMap[e.Name] = e
		}
		return nil
	})
	if err != nil {
		utils.ErrorPrintf("Verification failed.: %s", err.Error())
		return false
	}

	isValid := true

	for _, i := range chunkManager.manifest.Dirs {
		target, ok := dirMap[i.Name]
		if !ok {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (dir not found)", i.Name)
			continue
		}

		if i.Perm != target.Perm {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (dir not equal)", i.Name)
			continue
		}
	}

	for _, i := range chunkManager.manifest.Symlinks {
		target, ok := symlinkMap[i.Name]
		if !ok {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (symlink not found)", i.Name)
			continue
		}

		if i.SymlinkTo != target.SymlinkTo {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (symlink not equal)", i.Name)
			continue
		}
	}

	for _, i := range chunkManager.manifest.EmptyFiles {
		target, ok := emptyFileMap[i.Name]
		if !ok {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (empty file not found)", i.Name)
			continue
		}

		if i.Perm != target.Perm {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (empty file not equal)", i.Name)
			continue
		}
	}

	for i := range chunkManager.manifest.Files {
		f := &chunkManager.manifest.Files[i]

		target, ok := fileMap[f.Name]
		if !ok {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (file not found)", f.Name)
			continue
		}

		if f.Size != target.Size || f.Perm != target.Perm {
			isValid = false
			utils.ErrorPrintf("Verification failed.: %s (file not equal)", f.Name)
			continue
		}
	}

	return isValid
}
