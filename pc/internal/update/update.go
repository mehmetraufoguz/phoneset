// Package update finds out whether a newer phoneset release exists and installs it.
//
// "Newer" is decided by commit, not by version string: the running build knows the commit it was built
// from (Go embeds it), the latest release's tag resolves to a commit, and GitHub's compare API says whether
// that commit is ahead of, behind or the same as ours.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const DefaultRepo = "mehmetraufoguz/phoneset"

// Build describes the running binary.
type Build struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Modified bool   `json:"modified"`
}

// CurrentBuild names the running binary. stamped is the commit the release workflow wrote in with
// -ldflags; without it the commit Go embeds in a build made from a git checkout is used.
func CurrentBuild(version, stamped string) Build {
	b := Build{Version: version, Commit: stamped}
	if stamped != "" {
		return b
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				b.Commit = s.Value
			case "vcs.modified":
				b.Modified = s.Value == "true"
			}
		}
	}
	return b
}

func (b Build) Short() string {
	if len(b.Commit) > 7 {
		return b.Commit[:7]
	}
	return b.Commit
}

func (b Build) String() string {
	switch {
	case b.Commit == "":
		return b.Version
	case b.Modified:
		return fmt.Sprintf("%s (%s, modified)", b.Version, b.Short())
	default:
		return fmt.Sprintf("%s (%s)", b.Version, b.Short())
	}
}

type Release struct {
	Tag    string           `json:"tag"`
	Commit string           `json:"commit"`
	Assets map[string]Asset `json:"-"`
}

func (r Release) Short() string {
	if len(r.Commit) > 7 {
		return r.Commit[:7]
	}
	return r.Commit
}

type Asset struct {
	Name string
	URL  string // API URL, needs the token for private repositories
	Web  string // browser download URL
}

type Status string

const (
	UpToDate  Status = "up-to-date"
	Available Status = "available"
	Ahead     Status = "ahead" // this build is newer than the latest release (a development build)
	Unknown   Status = "unknown"
)

type Result struct {
	Current   Build     `json:"current"`
	Latest    *Release  `json:"latest,omitempty"`
	Status    Status    `json:"status"`
	BehindBy  int       `json:"behindBy,omitempty"` // commits the latest release has that this build lacks
	AheadBy   int       `json:"aheadBy,omitempty"`
	Note      string    `json:"note,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// ErrNoRelease means the repository has no release, or it can't be seen without signing in.
var ErrNoRelease = errors.New("no release found (the repository has none yet, or it is private: set GITHUB_TOKEN or run `gh auth login`)")

type httpError struct {
	URL    string
	Status string
	Code   int
}

func (e *httpError) Error() string { return e.URL + " answered " + e.Status }

type Client struct {
	Repo string
	API  string
	HTTP *http.Client
}

// NewClient honours PHONESET_UPDATE_REPO and PHONESET_UPDATE_API (forks, and local test servers).
func NewClient() *Client {
	c := &Client{Repo: DefaultRepo, API: "https://api.github.com", HTTP: &http.Client{Timeout: 60 * time.Second}}
	if v := os.Getenv("PHONESET_UPDATE_REPO"); v != "" {
		c.Repo = v
	}
	if v := os.Getenv("PHONESET_UPDATE_API"); v != "" {
		c.API = strings.TrimRight(v, "/")
	}
	return c
}

func token() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// get reads JSON from the API: anonymously, with a token if there is one, and through the GitHub CLI
// as a last resort (a private repository answers 404 to anyone who is not signed in).
func (c *Client) get(ctx context.Context, p string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+p, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "phoneset-updater")
	if t := token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.HTTP.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return json.NewDecoder(resp.Body).Decode(out)
		}
		err = &httpError{URL: c.API + p, Status: resp.Status, Code: resp.StatusCode}
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			return err
		}
	}
	// The GitHub CLI knows who you are.
	if gh, lookErr := exec.LookPath("gh"); lookErr == nil && strings.HasPrefix(c.API, "https://api.github.com") {
		raw, ghErr := exec.CommandContext(ctx, gh, "api", p).Output()
		if ghErr == nil {
			return json.Unmarshal(raw, out)
		}
	}
	return err
}

type apiRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
		Web  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest finds the newest published release and the commit its tag points at.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	var r apiRelease
	if err := c.get(ctx, "/repos/"+c.Repo+"/releases/latest", &r); err != nil {
		var he *httpError
		if errors.As(err, &he) && he.Code == http.StatusNotFound {
			return nil, ErrNoRelease
		}
		return nil, err
	}
	rel := &Release{Tag: r.TagName, Assets: map[string]Asset{}}
	for _, a := range r.Assets {
		rel.Assets[a.Name] = Asset{Name: a.Name, URL: a.URL, Web: a.Web}
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := c.get(ctx, "/repos/"+c.Repo+"/commits/"+url.PathEscape(r.TagName), &commit); err != nil {
		return nil, fmt.Errorf("couldn't resolve %s to a commit: %w", r.TagName, err)
	}
	rel.Commit = commit.SHA
	return rel, nil
}

// Check compares the running build with the latest release.
func (c *Client) Check(ctx context.Context, cur Build) (Result, error) {
	res := Result{Current: cur, Status: Unknown, CheckedAt: time.Now()}
	rel, err := c.Latest(ctx)
	if err != nil {
		return res, err
	}
	res.Latest = rel
	switch {
	case cur.Commit == "":
		res.Note = "this build doesn't know its commit"
		return res, nil
	case cur.Commit == rel.Commit && !cur.Modified:
		res.Status = UpToDate
		return res, nil
	}
	var cmp struct {
		Status   string `json:"status"`
		AheadBy  int    `json:"ahead_by"`
		BehindBy int    `json:"behind_by"`
	}
	// base = this build, head = the release: "ahead" means the release has commits we don't.
	if err := c.get(ctx, "/repos/"+c.Repo+"/compare/"+cur.Commit+"..."+rel.Commit, &cmp); err != nil {
		res.Note = "this build's commit isn't in the repository"
		return res, nil
	}
	switch cmp.Status {
	case "identical":
		res.Status = UpToDate
	case "behind":
		res.Status, res.AheadBy = Ahead, cmp.BehindBy
	default: // "ahead" or "diverged"
		res.Status, res.BehindBy = Available, cmp.AheadBy
	}
	return res, nil
}

// AssetName is the release archive for this system.
func AssetName() (string, error) {
	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		return "phoneset-macos-" + arch + ".tar.gz", nil
	case "windows":
		return "phoneset-windows-" + arch + ".zip", nil
	}
	return "", fmt.Errorf("no release builds for %s", runtime.GOOS)
}

// Install downloads rel, checks it and puts it in place of exe. The running copy keeps working:
// on Windows it is renamed aside, elsewhere the file is replaced atomically.
func (c *Client) Install(ctx context.Context, rel *Release, exe string, log func(string)) error {
	name, err := AssetName()
	if err != nil {
		return err
	}
	if _, ok := rel.Assets[name]; !ok {
		return fmt.Errorf("release %s has no %s", rel.Tag, name)
	}
	tmp, err := os.MkdirTemp("", "phoneset-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	log("downloading " + name)
	archive, err := c.download(ctx, rel, name, tmp)
	if err != nil {
		return err
	}

	if sums, err := c.download(ctx, rel, "SHA256SUMS", tmp); err == nil {
		if err := verify(archive, name, sums); err != nil {
			return err
		}
		log("checksum ok")
	} else {
		log("this release has no SHA256SUMS; skipping the checksum")
	}

	bin := "phoneset"
	if runtime.GOOS == "windows" {
		bin = "phoneset.exe"
	}
	staged := exe + ".new"
	if err := extract(archive, bin, staged); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("can't write to %s; run the installer again or move phoneset somewhere you own", filepath.Dir(exe))
		}
		return err
	}
	defer os.Remove(staged)

	// Run the new binary once before trusting it with the old one's place.
	check, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(check, staged, "version").Output()
	if err != nil || !strings.HasPrefix(string(out), "phoneset") {
		return fmt.Errorf("the downloaded phoneset doesn't run on this machine")
	}
	return replace(exe, staged)
}

func (c *Client) download(ctx context.Context, rel *Release, name, dir string) (string, error) {
	a, ok := rel.Assets[name]
	if !ok {
		return "", fmt.Errorf("no %s in release %s", name, rel.Tag)
	}
	dest := filepath.Join(dir, name)
	try := func(u string, auth bool, accept string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "phoneset-updater")
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if auth && token() != "" {
			req.Header.Set("Authorization", "Bearer "+token())
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s answered %s", u, resp.Status)
		}
		f, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, resp.Body)
		return err
	}

	err := try(a.Web, false, "")
	if err != nil && token() != "" {
		err = try(a.URL, true, "application/octet-stream")
	}
	if err != nil {
		if gh, lookErr := exec.LookPath("gh"); lookErr == nil {
			cmd := exec.CommandContext(ctx, gh, "release", "download", rel.Tag, "--repo", c.Repo, "--pattern", name, "--dir", dir, "--clobber")
			if out, ghErr := cmd.CombinedOutput(); ghErr == nil {
				return dest, nil
			} else {
				err = fmt.Errorf("%v; gh: %s", err, strings.TrimSpace(string(out)))
			}
		}
		return "", err
	}
	return dest, nil
}

func verify(archive, name, sumsFile string) error {
	sums, err := os.ReadFile(sumsFile)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS has no entry for %s", name)
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s: the download is corrupt or has been tampered with", name)
	}
	return nil
}

// extract writes the executable inside archive (a .tar.gz or .zip) to dest.
func extract(archive, bin, dest string) error {
	write := func(r io.Reader) error {
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, zf := range zr.File {
			if path.Base(zf.Name) == bin {
				rc, err := zf.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				return write(rc)
			}
		}
		return fmt.Errorf("the archive doesn't contain %s", bin)
	}
	raw, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("the archive doesn't contain %s", bin)
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == bin {
			return write(tr)
		}
	}
}

func replace(exe, staged string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(staged, exe)
	}
	// Windows lets a running executable be renamed but not overwritten.
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}

// CleanupOld removes the previous executable left behind by a Windows update.
func CleanupOld() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}
