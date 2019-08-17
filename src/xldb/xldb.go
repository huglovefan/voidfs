/*
 * "xlocate database"
 */

package xldb

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type Vfs map[string]*Vfs

type Xldb struct {
	LastModified string // last-modified header
	Repo         string // path to git repo

	vfs_owners  map[*Vfs]map[Pkgver]VfsType
	vfs_parents map[*Vfs]*Vfs
	vfs_root    Vfs
	loading     int32
	mutex       sync.RWMutex
	pkgs        map[string]string
}

func getDefaultRepo() string {
	repo := os.Getenv("VOIDFS_REPO")
	if repo == "" {
		// same default as xlocate
		xlGit := os.Getenv("XLOCATE_GIT")
		if xlGit == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				cache = "."
			}
			xlGit = cache + "/xlocate.git"
		}
		repo = xlGit
	}
	return repo
}

func (self *Xldb) Init() {
	self.vfs_root = make(Vfs)
	self.vfs_owners = make(map[*Vfs]map[Pkgver]VfsType)
	self.vfs_parents = make(map[*Vfs]*Vfs)
	self.vfs_parents[&self.vfs_root] = &self.vfs_root
	self.vfs_owners[&self.vfs_root] = make(map[Pkgver]VfsType)
	self.pkgs = make(map[string]string)
	self.Repo = getDefaultRepo()
}

func (self *Xldb) RLock() {
	self.mutex.RLock()
}

func (self *Xldb) RUnlock() {
	self.mutex.RUnlock()
}

const thecomma = ","
const thearrow = " -> "

func splitLine(line string) (pkgver Pkgver, path string, target string) {
	comma := strings.Index(line, thecomma)
	pkgver = Pkgver(line[0:comma])
	path = line[comma+len(thecomma):]
	if arrow := strings.Index(path, thearrow); arrow != -1 {
		target = path[arrow+len(thearrow):]
		path = path[0:arrow]
	}
	return
}

func splitPath(path string) []string {
	names := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := 0; i < len(names); i++ {
		if names[i] == "" {
			names = append(names[:i], names[i+1:]...)
			i--
		}
	}
	return names
}

func (self *Xldb) getLastModified() (string, error) {
	cmd := exec.Command("/bin/sh", "-c", `
	set -e
	s=$(git -C "$VOIDFS_REPO" log -1 --format=%at)
	LC_ALL=C TZ=GMT date -d "@$s" +'%a, %d %b %Y %H:%M:%S %Z'
	`)
	cmd.Env = append(os.Environ(), fmt.Sprintf("VOIDFS_REPO=%s", self.Repo))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func (self *Xldb) Load() error {

	defer atomic.AddInt32(&self.loading, -1)
	if atomic.AddInt32(&self.loading, 1) > 1 {
		fmt.Println("xldb: already loading")
		return nil
	}

	lastModified, err := self.getLastModified()
	if err != nil {
		return fmt.Errorf("failed to read date from xlocate repo: %s", err)
	}

	updating := self.LastModified != ""

	if updating {
		if lastModified == self.LastModified {
			fmt.Println("xldb: already up-to-date")
			return nil
		}
		fmt.Printf("xldb: %s -> %s\n", self.LastModified, lastModified)
	}

	cmd := exec.Command("/bin/sh", "-c", `
	# -z: use null byte instead of colon for the delimiter
	#     the version string of "telepathy-mission-control" contains a colon
	# tr: (could this be removed? lua didn't like null bytes in strings)
	git -C "$VOIDFS_REPO" grep -z '' @ | tr '\0' ',' | cut -b3- | uniq
	`)
	cmd.Env = append(os.Environ(), fmt.Sprintf("VOIDFS_REPO=%s", self.Repo))
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to read file list: %s", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to read file list: %s", err)
	}

	pkgs := make(map[string]string)

	var ppkgver Pkgver
	skip := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		pkgver, path, target := splitLine(line)

		if pkgver == ppkgver {
			if skip {
				continue
			}
			pkgver = ppkgver
		} else {
			pkgver = Pkgver([]byte(pkgver))
		}

		self.mutex.Lock()

		if pkgver != ppkgver {
			pkgname, version := pkgver.Split()
			pkgs[pkgname] = version
			ppkgver = pkgver
			if updating {
				if version == self.pkgs[pkgname] {
					// unchanged
					skip = true
				} else if self.pkgs[pkgname] != "" {
					// updated
					skip = false
					fmt.Printf("%s: %s -> %s\n", pkgname, self.pkgs[pkgname], version)
					self.vfsEradicatePkgver(&self.vfs_root,
						JoinPkgver(pkgname, self.pkgs[pkgname]))
				} else {
					// added
					skip = false
					fmt.Printf("%s: new package\n", pkgname)
				}
				if skip {
					self.mutex.Unlock()
					continue
				}
				// removed packages are checked later
			}
			self.vfs_owners[&self.vfs_root][pkgver] = XLDB_DIR
		}

		vfs := &self.vfs_root
		components := splitPath(path)
		for i, name := range components {
			var cvtype VfsType
			if i < len(components)-1 {
				cvtype = XLDB_DIR
			} else if target == "" {
				cvtype = XLDB_FILE
			} else {
				cvtype = VfsType([]byte(target))
			}
			cvfs := self.vfsGetOrCreate(vfs, name)
			self.vfs_owners[cvfs][pkgver] = cvtype
			vfs = cvfs
		}

		self.mutex.Unlock()
	}

	if updating {
		// check for removed packages
		for pkgname, version := range self.pkgs {
			if pkgs[pkgname] == "" {
				fmt.Printf("%s: removed package\n", pkgname)
				self.mutex.Lock()
				self.vfsEradicatePkgver(&self.vfs_root,
					JoinPkgver(pkgname, version))
				self.mutex.Unlock()
			}
		}
	}
	self.pkgs = pkgs

	// only update this after we're done so browsers don't cache inconsistent results
	self.LastModified = lastModified

	// don't return errors on these since we already updated the database
	if err := scanner.Err(); err != nil {
		fmt.Printf("xldb: %s\n", err)
	}
	if err := cmd.Wait(); err != nil {
		fmt.Printf("xldb: %s\n", err)
	}

	return nil
}

func (self *Xldb) vfsEradicatePkgver(vfs *Vfs, pkgver Pkgver) {
	vtype := self.vfs_owners[vfs][pkgver]
	if !vtype.Ok() {
		return
	}
	if vtype.IsDir() {
		for _, cvfs := range *vfs {
			self.vfsEradicatePkgver(cvfs, pkgver)
		}
	}
	delete(self.vfs_owners[vfs], pkgver) // a
	if len(self.vfs_owners[vfs]) == 0 && vfs != &self.vfs_root {
		parent := self.VfsGetParent(vfs)
		name := self.VfsGetName(vfs)
		delete(*parent, name)
		delete(self.vfs_owners, vfs) // b
		delete(self.vfs_parents, vfs)
	}
}

func (self *Xldb) vfsGetOrCreate(vfs *Vfs, name string) *Vfs {
	cvfs := (*vfs)[name]
	if cvfs == nil {
		cvfs = &Vfs{}
		(*vfs)[string([]byte(name))] = cvfs
		self.vfs_owners[cvfs] = make(map[Pkgver]VfsType)
		self.vfs_parents[cvfs] = vfs
	}
	return cvfs
}
