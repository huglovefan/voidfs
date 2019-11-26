package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	// _ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

import "xldb"

var commit string // set in ldflags
var progname = "voidfs"

func init() {
	if commit != "" {
		progname += fmt.Sprintf(" %s", commit)
	}
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

func print_header(w http.ResponseWriter, xd *xldb.Xldb, vfs *xldb.Vfs, abspath string) {
	fmt.Fprintf(w, `<a href="/">/</a>`)
	pathLen := len(abspath) + len(" is a ")
	components := splitPath(abspath)
	p := ""
	for i, name := range components {
		dirslash_url := "/"
		dirslash_dis := "/"
		if i == len(components)-1 {
			dirslash_url = ""
			dirslash_dis = ""
			if xd.VfsIsDir(vfs, 3) {
				dirslash_url = "/"
				dirslash_dis = "/"
				pathLen += 1
			}
		}
		part_uh := html.EscapeString(url.PathEscape(name)) + dirslash_url
		fmt.Fprintf(w, `<a href="/%s%s">%s%s</a>`,
			p, part_uh,
			html.EscapeString(name), dirslash_dis)
		p += part_uh
	}
	fmt.Fprintf(w, " is a ")

	spaces := ""
	dotype := func(n int, u string) {
		if n == 0 {
			return
		}
		es := "s"
		if n == 1 {
			es = ""
		}
		fmt.Fprintf(w, "%s%s in %d package%s\n",
			spaces,
			u,
			n,
			es)
		if spaces == "" {
			spaces = strings.Repeat(" ", pathLen)
		}
	}
	types := xd.VfsGetTypes(vfs)
	dotype(types.Dir, "dir")
	dotype(types.File, "file")
	dotype(types.Link, "link")
	fmt.Fprintf(w, "\n")
}

func make_typestr(types xldb.VfsTypes) string {
	rv := ""
	if types.Dir > 0 {
		rv += fmt.Sprintf(", dir (%d)", types.Dir)
	}
	if types.File > 0 {
		rv += fmt.Sprintf(", file (%d)", types.File)
	}
	if types.Link > 0 {
		rv += fmt.Sprintf(", link (%d)", types.Link)
	}
	return rv[2:]
}

type child_entry struct {
	name     string
	is_dir   bool
	typestr  string
	name_uh  string
	dirslash string
	name_h   string
	vlen     int
}

func print_children(w http.ResponseWriter, xd *xldb.Xldb, vfs *xldb.Vfs) {
	entries := make([]child_entry, len(*vfs))
	longest_vlen := 0
	i := 0
	for name, cvfs := range *vfs {
		entry := &entries[i]
		types := xd.VfsGetTypes(cvfs)
		entry.name = name
		entry.typestr = make_typestr(types)
		entry.is_dir = types.Dir > 0 || (types.Link > 0 && xd.VfsIsDir(cvfs, 3))
		entry.name_uh = html.EscapeString(url.PathEscape(name))
		entry.name_h = html.EscapeString(name)
		entry.vlen = len(name)
		if entry.is_dir {
			entry.dirslash = "/"
			entry.vlen += 1
		}
		if entry.vlen > longest_vlen {
			longest_vlen = entry.vlen
		}
		i += 1
	}
	sort.Slice(entries, func(i1, i2 int) bool {
		e1, e2 := entries[i1], entries[i2]
		if e1.is_dir == e2.is_dir {
			return e1.name < e2.name
		} else {
			return e1.is_dir
		}
	})
	sp := strings.Repeat(" ", longest_vlen+2)
	for _, entry := range entries {
		fmt.Fprintf(w, `<a href="./%s%s">%s%s</a>%s%s%s`,
			entry.name_uh,
			entry.dirslash,
			entry.name_h,
			entry.dirslash,
			sp[0:(longest_vlen-entry.vlen+2)],
			entry.typestr,
			"\n")
	}
}

type owner_entry struct {
	pkgver  xldb.Pkgver
	typestr string
}

func print_owner_info(w http.ResponseWriter, xd *xldb.Xldb, vfs *xldb.Vfs, real_path string) {
	owners := make([]owner_entry, len(xd.VfsGetOwners(vfs)))
	is_file := false
	longest_owner := 0
	i := 0
	for pkgver, vtype := range xd.VfsGetOwners(vfs) {
		owner := &owners[i]
		owner.pkgver = pkgver
		switch vtype {
		case xldb.XLDB_DIR:
			owner.typestr = "dir"
		case xldb.XLDB_FILE:
			owner.typestr = "file"
			is_file = true
		default:
			if tgt := xd.VfsLinkResolveTarget(vfs, vtype.GetTarget()); tgt != nil {
				urlpath := xd.VfsGetPathUrlencoded(tgt)
				urlpath += xd.VfsGetDirslash(tgt, 3)
				owner.typestr = fmt.Sprintf(`link to <a href="%s">%s</a>`,
					html.EscapeString(urlpath),
					html.EscapeString(vtype.GetTarget()))
			} else {
				owner.typestr = fmt.Sprintf(`link to <span>%s</span>`,
					html.EscapeString(vtype.GetTarget()))
			}
		}
		if len(pkgver) > longest_owner {
			longest_owner = len(pkgver)
		}
		i += 1
	}
	sort.Slice(owners, func(i1, i2 int) bool {
		return owners[i1].pkgver < owners[i2].pkgver
	})
	if is_file {
		path := html.EscapeString(shellquote(real_path))
		for _, entry := range owners {
			if entry.typestr != "file" {
				continue
			}
			fmt.Fprintf(w, "%% xbps-query -R %s --cat=%s\n",
				entry.pkgver,
				path)
		}
		fmt.Fprintf(w, "\n")
	}
	sp := strings.Repeat(" ", longest_owner+2)
	for i, entry := range owners {
		newline := ""
		if i < len(owners)-1 {
			newline = "\n"
		}
		fmt.Fprintf(w, "%s%s%s%s",
			entry.pkgver,
			sp[0:(longest_owner-len(entry.pkgver)+2)],
			entry.typestr,
			newline)
	}
}

func main() {
	xd := xldb.Xldb{}
	xd.Init()
	go func() {
		sig := make(chan os.Signal)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGUSR1)

		if err := xd.Load(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("voidfs: initial load done")

		for {
			switch <-sig {
			case syscall.SIGHUP:
				fmt.Println("voidfs: received SIGHUP, reloading database")
				go func() {
					if err := xd.Load(); err != nil {
						fmt.Fprintf(os.Stderr, "%s\n", err)
					}
					fmt.Println("voidfs: reload done")
				}()
			case syscall.SIGUSR1:
				xd.RLock()
				xd.Vfsck(nil)
				xd.RUnlock()
			}
		}
	}()
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {

		switch req.Method {
		case "GET":
			// ok
		case "HEAD":
			// ok
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		xd.RLock()
		defer xd.RUnlock()

		h := w.Header()
		h.Add("Content-Type", "text/html; charset=utf-8")
		h.Add("Server", progname)
		if xd.LastModified != "" {
			h.Add("Last-Modified", xd.LastModified)
			if req.Header.Get("If-Modified-Since") == xd.LastModified {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		vfs := xd.VfsDirFollowPath(nil, req.URL.Path)
		if vfs == nil {
			w.WriteHeader(http.StatusNotFound)
			if req.Method != "HEAD" {
				fmt.Fprintf(w, `<!doctype html>`)
				fmt.Fprintf(w, `<title>voidfs:error</title>`)
				fmt.Fprintf(w, `<pre style="cursor: default; margin: 0;">`)
				fmt.Fprintf(w, `not found`)
				fmt.Fprintf(w, `</pre>`)
			}
			return
		}

		cwd_is_dir := xd.VfsIsDir(vfs, 3)
		url_is_dir := strings.HasSuffix(req.URL.Path, "/")
		if cwd_is_dir && !url_is_dir {
			h.Add("Location", req.URL.Path+"/")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		} else if url_is_dir && !cwd_is_dir {
			h.Add("Location", strings.TrimRight(req.URL.Path, "/"))
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}

		if req.Method == "HEAD" {
			return
		}

		dirslash := ""
		if cwd_is_dir && xd.VfsGetParent(vfs) != vfs {
			dirslash = "/"
		}

		real_path := xd.VfsGetPath(vfs)

		fmt.Fprintf(w, `<!doctype html>`)
		fmt.Fprintf(w, `<title>voidfs:%s%s</title>`,
			html.EscapeString(real_path),
			dirslash)
		fmt.Fprintf(w, `<pre style="cursor: default; margin: 0;">`)

		print_header(w, &xd, vfs, real_path)

		if len(*vfs) != 0 {
			print_children(w, &xd, vfs)
			fmt.Fprintf(w, "\n")
		}

		print_owner_info(w, &xd, vfs, real_path)

		fmt.Fprintf(w, `</pre>`)
	})
	addr := os.Getenv("VOIDFS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	fmt.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
