package sourcerev

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "JJ_USER=t", "JJ_EMAIL=t@example.invalid")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	write(t, filepath.Join(root, "src", "train.py"), "print(1)\n")
	run(t, root, "git", "add", "-A")
	run(t, root, "git", "commit", "-qm", "first")
	return root
}

// TestReadGit reads HEAD from a subdirectory, and reports tracked changes,
// but not untracked files, as dirty.
func TestReadGit(t *testing.T) {
	root := gitRepo(t)
	head := run(t, root, "git", "rev-parse", "HEAD")
	revision, ok := Read(filepath.Join(root, "src"))
	if !ok || revision.Root != root || revision.VCS != "git" || revision.CommitID != head || revision.Dirty || revision.Error != "" {
		t.Fatalf("clean Read = %+v, %v; want git HEAD %s at %s", revision, ok, head, root)
	}
	write(t, filepath.Join(root, "notes.txt"), "untracked\n")
	if revision, _ := Read(root); revision.Dirty {
		t.Fatalf("an untracked file made the revision dirty: %+v", revision)
	}
	write(t, filepath.Join(root, "src", "train.py"), "print(2)\n")
	if revision, _ := Read(root); !revision.Dirty || revision.CommitID != head {
		t.Fatalf("edited Read = %+v, want dirty at %s", revision, head)
	}
}

// TestReadReportsAFailedRead records why a revision is unknown, here a
// repository with no commit yet.
func TestReadReportsAFailedRead(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	revision, ok := Read(root)
	if !ok || revision.Error == "" || revision.CommitID != "" {
		t.Fatalf("Read of a repository without commits = %+v, %v; want an error", revision, ok)
	}
}

func TestReadOutsideARepository(t *testing.T) {
	if revision, ok := Read(t.TempDir()); ok {
		t.Fatalf("Read outside a repository = %+v, want none", revision)
	}
}

// TestReadJJ checks that a jj repository, colocated with git or not, is read
// as jj, and that an edit is snapshotted into a new commit ID with the same
// change ID.
func TestReadJJ(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	t.Setenv("JJ_CONFIG", filepath.Join(t.TempDir(), "none.toml"))
	for _, colocate := range []bool{false, true} {
		root := t.TempDir()
		args := []string{"git", "init"}
		if colocate {
			args = append(args, "--colocate")
		}
		run(t, root, "jj", args...)
		write(t, filepath.Join(root, "train.py"), "print(1)\n")
		first, ok := Read(root)
		if !ok || first.VCS != "jj" || first.CommitID == "" || first.ChangeID == "" || first.Error != "" {
			t.Fatalf("colocate=%v: Read = %+v, %v; want a jj revision", colocate, first, ok)
		}
		write(t, filepath.Join(root, "train.py"), "print(2)\n")
		second, _ := Read(root)
		if second.CommitID == first.CommitID || second.ChangeID != first.ChangeID {
			t.Fatalf("colocate=%v: after an edit Read = %+v, before %+v; want a new commit ID in the same change", colocate, second, first)
		}
	}
}

// TestReadAllReadsEachRepositoryOnce gives two directories of one repository
// and one outside any; the result has the repository once.
func TestReadAllReadsEachRepositoryOnce(t *testing.T) {
	root := gitRepo(t)
	outside := t.TempDir()
	sources := ReadAll([]string{filepath.Join(root, "src"), outside, root, outside})
	if len(sources.Sources) != 1 || sources.Sources[0].Root != root {
		t.Fatalf("ReadAll = %+v, want only %s", sources, root)
	}
	if source, ok := sources.SourceFor(filepath.Join(root, "src")); !ok || source.Root != root {
		t.Fatalf("SourceFor(src) = %+v, %v", source, ok)
	}
	if _, ok := sources.SourceFor(root + "-other"); ok {
		t.Fatal("SourceFor matched a sibling directory sharing the root's prefix")
	}
}
