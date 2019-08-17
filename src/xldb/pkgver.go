package xldb

import (
	"fmt"
	"strings"
)

type Pkgver string

func JoinPkgver(pkgname, version string) Pkgver {
	return Pkgver(fmt.Sprintf("%s-%s", pkgname, version))
}

func (pkgver Pkgver) Split() (string, string) {
	dash := strings.LastIndex(string(pkgver), "-")
	return string(pkgver)[0:dash], string(pkgver)[dash+1:]
}

func (pkgver Pkgver) Name() string {
	dash := strings.LastIndex(string(pkgver), "-")
	return string(pkgver)[0:dash]
}

func (pkgver Pkgver) Version() string {
	dash := strings.LastIndex(string(pkgver), "-")
	return string(pkgver)[dash+1:]
}
