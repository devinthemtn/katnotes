// Package gitsync commits, pulls and pushes the notes folder when it is (or
// is inside) a git repository. Everything is limited to that folder, so a
// notes folder inside a bigger repo only commits its own files.
package gitsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Timeout bounds each git command (network ones included).
var Timeout = 60 * time.Second

// errTimeout marks a git command that ran out of time.
var errTimeout = errors.New("timed out")

// ErrNotRepo means the folder isn't in a git work tree.
var ErrNotRepo = errors.New("notes folder is not a git repository: run git init there to use sync")

// Repo is a notes folder inside a git work tree.
type Repo struct {
	Dir    string
	prefix string // Dir relative to the repo root, with a trailing slash ("" at the root)
}

// Open checks that dir is in a git work tree.
func Open(dir string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errors.New("git is not installed")
	}
	r := &Repo{Dir: dir}
	out, err := r.git("rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(out) != "true" {
		return nil, ErrNotRepo
	}
	if r.prefix, err = r.git("rev-parse", "--show-prefix"); err != nil {
		return nil, err
	}
	r.prefix = strings.TrimSpace(r.prefix)
	return r, nil
}

func (r *Repo) git(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.Dir, "-c", "core.quotepath=off"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git %s: %w after %s", args[0], errTimeout, Timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// Change is one changed file, relative to Dir.
type Change struct {
	Code string `json:"code"` // porcelain XY status, e.g. " M", "??", "D "
	Path string `json:"path"`
}

// Status lists uncommitted changes under Dir.
func (r *Repo) Status() ([]Change, error) {
	// -z: NUL-separated and never quoted, so any file name reads back as is.
	out, err := r.git("status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	var cs []Change
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		l := fields[i]
		if len(l) < 4 {
			continue
		}
		code := l[:2]
		if code[0] == 'R' || code[0] == 'C' {
			i++ // a rename or copy is followed by its old path; keep the new one
		}
		cs = append(cs, Change{Code: code, Path: strings.TrimPrefix(l[3:], r.prefix)})
	}
	return cs, nil
}

// Summary says what Sync did.
type Summary struct {
	Committed int    `json:"committed"` // files in the new commit (0: nothing to commit)
	Upstream  string `json:"upstream"`  // e.g. "origin/main"; "" when the branch has none
	Pulled    bool   `json:"pulled"`    // the pull brought in commits
	Pushed    bool   `json:"pushed"`
}

func (s Summary) String() string {
	var parts []string
	if s.Committed > 0 {
		parts = append(parts, fmt.Sprintf("committed %d file(s)", s.Committed))
	} else {
		parts = append(parts, "nothing to commit")
	}
	if s.Upstream == "" {
		parts = append(parts, "no upstream branch, so nothing pulled or pushed (git push -u <remote> <branch> once)")
	} else {
		if s.Pulled {
			parts = append(parts, "pulled changes from "+s.Upstream)
		} else {
			parts = append(parts, "up to date with "+s.Upstream)
		}
		if s.Pushed {
			parts = append(parts, "pushed")
		}
	}
	return "sync: " + strings.Join(parts, " · ")
}

// Sync commits every change under Dir (message "katnotes sync <time>"),
// then, when the branch has an upstream, pulls with rebase and pushes.
func (r *Repo) Sync(now time.Time) (Summary, error) {
	var s Summary
	cs, err := r.Status()
	if err != nil {
		return s, err
	}
	if len(cs) > 0 {
		if _, err := r.git("add", "-A", "--", "."); err != nil {
			return s, err
		}
		msg := "katnotes sync " + now.Format("2006-01-02 15:04")
		if _, err := r.git("commit", "-q", "-m", msg, "--", "."); err != nil {
			return s, err
		}
		s.Committed = len(cs)
	}
	up, err := r.upstream()
	if err != nil || up == "" {
		return s, err // no upstream: a local-only repo is fine
	}
	s.Upstream = up
	before, _ := r.git("rev-parse", "-q", "--verify", "HEAD") // "" before the first commit
	if _, err := r.git("pull", "-q", "--rebase", "--autostash"); err != nil {
		return s, err
	}
	after, err := r.git("rev-parse", "HEAD")
	if err != nil {
		return s, err
	}
	s.Pulled = before != after
	ahead, err := r.git("rev-list", "--count", "@{u}..HEAD")
	if err == nil && strings.TrimSpace(ahead) != "0" {
		if _, err := r.git("push", "-q"); err != nil {
			return s, err
		}
		s.Pushed = true
	}
	return s, nil
}

// upstream is the current branch's upstream, e.g. "origin/main", or "" when
// it has none, HEAD is detached, or the upstream branch doesn't exist yet
// (a clone of an empty remote). Only a timeout is an error.
func (r *Repo) upstream() (string, error) {
	up, err := r.git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if errors.Is(err, errTimeout) {
		return "", err
	}
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(up), nil
}
