package main

import (
	"regexp"
	"strings"
)

var shellOk *regexp.Regexp

func init() {
	shellOk = regexp.MustCompile("^[-./0-9A-Za-z]+$")
}

func shellquote(s string) string {
	if !shellOk.MatchString(s) {
		s = "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
	}
	return s
}
