package vcs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"code.google.com/p/go.tools/godoc/vfs"
)

type LocalGitCmdRepository struct {
	Dir string
}

func (r *LocalGitCmdRepository) ResolveRevision(spec string) (CommitID, error) {
	cmd := exec.Command("git", "rev-parse", spec)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("exec `git rev-parse` failed: %s. Output was:\n\n%s", err, out)
	}
	return CommitID(bytes.TrimSpace(out)), nil
}

func (r *LocalGitCmdRepository) ResolveBranch(name string) (CommitID, error) {
	return r.ResolveRevision(name)
}

func (r *LocalGitCmdRepository) ResolveTag(name string) (CommitID, error) {
	return r.ResolveRevision(name)
}

func (r *LocalGitCmdRepository) FileSystem(at CommitID) (vfs.FileSystem, error) {
	return &localGitCmdFS{
		dir: r.Dir,
		at:  at,
	}, nil
}

type localGitCmdFS struct {
	dir string
	at  CommitID
}

func (fs *localGitCmdFS) Open(name string) (vfs.ReadSeekCloser, error) {
	cmd := exec.Command("git", "show", string(fs.at)+":"+name)
	cmd.Dir = fs.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if bytes.Contains(out, []byte("exists on disk, but not in")) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("exec `git show` failed: %s. Output was:\n\n%s", err, out)
	}
	return nopCloser{bytes.NewReader(out)}, nil
}

func (fs *localGitCmdFS) Lstat(path string) (os.FileInfo, error) {
	return fs.Stat(path)
}

func (fs *localGitCmdFS) Stat(path string) (os.FileInfo, error) {
	// TODO(sqs): follow symlinks (as Stat is required to do)

	path = filepath.Clean(path)

	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte(fmt.Sprintf("tree %s:%s\n", fs.at, path))) {
		// dir
		return &fileInfo{name: filepath.Base(path), mode: os.ModeDir}, nil
	}

	return &fileInfo{name: filepath.Base(path), size: int64(len(data))}, nil
}

func (fs *localGitCmdFS) ReadDir(path string) ([]os.FileInfo, error) {
	path = filepath.Clean(path)

	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	// in `git show` output for dir, first line is header, 2nd line is blank,
	// and there is a trailing newline.
	lines := bytes.Split(data, []byte{'\n'})
	fis := make([]os.FileInfo, len(lines)-3)
	for i, line := range lines {
		if i < 2 || i == len(lines)-1 {
			continue
		}
		fis[i-2] = &fileInfo{name: string(line)}
	}

	return fis, nil
}

func (fs *localGitCmdFS) String() string {
	return fmt.Sprintf("local git cmd repository %s commit %s", fs.dir, fs.at)
}