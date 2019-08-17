XLOCATE_GIT=/srv/voidfs/xlocate.git
XLOCATE_REPO=https://alpha.de.repo.voidlinux.org/xlocate/xlocate.git

set -efu

[ "$(whoami)" = voidfs ] || exit

set -- $(pidof voidfs)

[ $# -gt 0 ] || exit

[ -d "$XLOCATE_GIT" ] || mkdir -p "$XLOCATE_GIT" || exit

if [ -f "$XLOCATE_GIT/config" ]; then
	git -C "$XLOCATE_GIT" fetch -f -u "$XLOCATE_REPO" master:master
else
	git clone --bare "$XLOCATE_REPO" "$XLOCATE_GIT"
fi

kill -HUP "$@"
