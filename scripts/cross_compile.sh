#!/bin/bash
go get github.com/mitchellh/gox
go get github.com/tcnksm/ghr

export APPNAME="nats-top"
export OSARCH="linux/amd64 darwin/amd64 linux/arm windows/amd64"
export DIRS="linux_amd64 darwin_amd64 linux_arm windows_amd64"
export OUTDIR="pkg"

rm $HOME/gopath/src/github.com/nats-io/gnatsd/server/pse_*.go
cat <<HERE > $HOME/gopath/src/github.com/nats-io/gnatsd/server/pse.go
// Copyright 2016 Apcera Inc. All rights reserved.

package server

// This is a placeholder for now.
func procUsage(pcpu *float64, rss, vss *int64) error {
	*pcpu = 0.0
	*rss = 0
	*vss = 0

	return nil
}
HERE

gox -osarch="$OSARCH" -output "$OUTDIR/$APPNAME-{{.OS}}_{{.Arch}}/$APPNAME"
for dir in $DIRS; do \
	(cp readme.md $OUTDIR/$APPNAME-$dir/readme.md) ;\
	(cp LICENSE $OUTDIR/$APPNAME-$dir/LICENSE) ;\
	(cd $OUTDIR && zip -q $APPNAME-$dir.zip -r $APPNAME-$dir) ;\
	echo "created '$OUTDIR/$APPNAME-$dir.zip', cleaning up..." ;\
	rm -rf $APPNAME-$dir;\
done

pwd
ls -laR .
