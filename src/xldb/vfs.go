/*
 * vfs methods that don't modify the database
 * all of these expect that you've called xldb.RLock() first
 */

package xldb

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
)

func (self *Xldb) VfsCd(vfs *Vfs, name string) *Vfs {
	switch name {
	case ".":
		return vfs
	case "..":
		return self.VfsGetParent(vfs)
	default:
		return (*vfs)[name]
	}
}

func (self *Xldb) vfsckCountTypesTotal(vfs *Vfs, pkgver Pkgver, total *VfsTypes) {
	vtype := self.vfs_owners[vfs][pkgver]
	if !vtype.Ok() {
		fmt.Printf("vfsck_count_types_total: '%s' does not own vfs '%s'!\n",
			pkgver, self.VfsGetPath(vfs))
		return
	}
	switch vtype {
	case XLDB_DIR:
		total.Dir += 1
		for _, cvfs := range *vfs {
			cvtype := self.vfs_owners[cvfs][pkgver]
			if cvtype.Ok() {
				self.vfsckCountTypesTotal(cvfs, pkgver, total)
			}
		}
	case XLDB_FILE:
		total.File += 1
	default:
		total.Link += 1
	}
}

func (self *Xldb) Vfsck(vfs *Vfs) {
	if vfs == nil {
		vfs = &self.vfs_root
		fmt.Println("vfsck: passed nil, defaulting to root (you should only see this once)")
		fmt.Println("vfsck: doing one-time checks")
		// check that every pkgver in self.pkgs owns at least one dir and file/link
		wg := sync.WaitGroup{}
		wg.Add(len(self.pkgs))
		for pkgname, version := range self.pkgs {
			go func(pkgname, version string) {
				types := VfsTypes{}
				pkgver := JoinPkgver(pkgname, version)
				self.vfsckCountTypesTotal(vfs, pkgver, &types)
				if types.File == 0 && types.Link == 0 {
					fmt.Printf("vfsck: '%s' doesn't own any files or links\n",
						pkgver)
				}
				if types.Dir == 0 {
					fmt.Printf("vfsck: '%s' doesn't own any directories\n",
						pkgver)
				}
				wg.Done()
			}(pkgname, version)
		}
		wg.Wait()
		fmt.Println("vfsck: checking vfs tree")
		defer fmt.Println("vfsck: done")
	}
	// check that it has a parent
	if self.vfs_parents[vfs] == nil {
		fmt.Printf("vfsck: vfs '%s' doesn't have a parent!\n",
			self.VfsGetPath(vfs))
	}
	// check that it has owners
	if self.vfs_owners[vfs] == nil || len(self.vfs_owners[vfs]) == 0 {
		fmt.Printf("vfsck: vfs '%s' has no owners\n",
			self.VfsGetPath(vfs))
	}
	// check that only root is its own parent
	if (vfs == self.vfs_parents[vfs]) != (vfs == &self.vfs_root) {
		if vfs == &self.vfs_root {
			fmt.Printf("vfsck: vfs_root is NOT its own parent\n")
		} else {
			fmt.Printf("vfsck: non-root vfs '%s' is its own parent\n",
				self.VfsGetPath(vfs))
		}
	}
	for pkgver, vtype := range self.vfs_owners[vfs] {
		pkgname, version := pkgver.Split()
		// check that the owner is the same version as in self.pkgs
		//
		// - notion-32bit always triggers this because it has two
		//   versions in xlocate (old version in a wrong repo?)
		if version != self.pkgs[pkgname] && pkgname != "notion-32bit" {
			fmt.Printf("vfsck: '%s' is owned by '%s' but self.pkgs has version '%s'\n",
				self.VfsGetPath(vfs), pkgver, self.pkgs[pkgname])
		}
		// check that the parent is a directory in the same package
		if parent := self.vfs_parents[vfs]; parent != nil {
			pvtype := self.vfs_owners[parent][pkgver]
			if !pvtype.IsDir() {
				fmt.Printf("vfsck: parent of '%s' owned by '%s' is not a dir in that package\n",
					self.VfsGetPath(vfs), pkgver)
			}
		}
		// check that a dir has at least one child from the package,
		// and a file/link has none
		hasChild := false
		for _, cvfs := range *vfs {
			if self.vfs_owners[cvfs][pkgver].Ok() {
				hasChild = true
				break
			}
		}
		if hasChild != vtype.IsDir() {
			if vtype.IsDir() {
				fmt.Printf("vfsck: '%s' is a dir in '%s' but it has no children from that package\n",
					self.VfsGetPath(vfs), pkgver)
			} else {
				fmt.Printf("vfsck: '%s' is NOT a dir in '%s' but it at least one child from that package\n",
					self.VfsGetPath(vfs), pkgver)
			}
		}
	}
	wg := sync.WaitGroup{}
	wg.Add(len(*vfs))
	for _, cvfs := range *vfs {
		go func(cvfs *Vfs) {
			self.Vfsck(cvfs)
			wg.Done()
		}(cvfs)
	}
	wg.Wait()
}

func (self *Xldb) VfsDirFollowPath(vfs *Vfs, path string) *Vfs {
	if vfs == nil || strings.HasPrefix(path, "/") {
		vfs = &self.vfs_root
	}
	for _, name := range splitPath(path) {
		vfs = self.VfsCd(vfs, name)
		if vfs == nil {
			break
		}
	}
	return vfs
}

func (self *Xldb) VfsGetDirslash(vfs *Vfs, depth int) string {
	if self.VfsIsDir(vfs, depth) {
		return "/"
	} else {
		return ""
	}
}

func (self *Xldb) VfsGetName(vfs *Vfs) string {
	if vfs == &self.vfs_root {
		return ""
	}
	for name, cvfs := range *self.VfsGetParent(vfs) {
		if cvfs == vfs {
			return name
		}
	}
	return ""
}

func (self *Xldb) VfsGetOwners(vfs *Vfs) map[Pkgver]VfsType {
	return self.vfs_owners[vfs]
}

func (self *Xldb) VfsGetParent(vfs *Vfs) *Vfs {
	return self.vfs_parents[vfs]
}

func (self *Xldb) VfsGetPath(vfs *Vfs) string {
	var path, slash string
	for {
		name := self.VfsGetName(vfs)
		if name == "" {
			break
		}
		path = name + slash + path
		slash = "/"
		vfs = self.VfsGetParent(vfs)
	}
	return "/" + path
}

/*
 * like VfsGetPath but url-encodes the path segments
 */
func (self *Xldb) VfsGetPathUrlencoded(vfs *Vfs) string {
	var path, slash string
	for {
		name := self.VfsGetName(vfs)
		if name == "" {
			break
		}
		path = url.PathEscape(name) + slash + path
		slash = "/"
		vfs = self.VfsGetParent(vfs)
	}
	return "/" + path
}

type VfsTypes struct {
	Dir  int
	File int
	Link int
}

func (self *Xldb) VfsGetTypes(vfs *Vfs) VfsTypes {
	types := VfsTypes{}
	for _, vtype := range self.vfs_owners[vfs] {
		switch vtype {
		case XLDB_DIR:
			types.Dir += 1
		case XLDB_FILE:
			types.File += 1
		default:
			types.Link += 1
		}
	}
	return types
}

func (self *Xldb) VfsIsDir(vfs *Vfs, depth int) bool {
	targets := make([]string, 0)
	for _, vtype := range self.VfsGetOwners(vfs) {
		if vtype.IsDir() {
			return true
		}
		if depth > 0 && vtype.IsLink() {
			targets = append(targets, vtype.GetTarget())
		}
	}
	if depth > 0 && len(targets) != 0 {
		depth -= 1
		for _, target := range targets {
			tgt := self.VfsLinkResolveTarget(vfs, target)
			if tgt != nil && self.VfsIsDir(tgt, depth) {
				return true
			}
		}
	}
	return false
}

func (self *Xldb) VfsLinkResolveTarget(vfs *Vfs, target string) *Vfs {
	return self.VfsDirFollowPath(self.VfsGetParent(vfs), target)
}
