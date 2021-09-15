#!/bin/bash
go install github.com/mitchellh/gox@latest
go install github.com/tcnksm/ghr@latest

APPNAME="nats-top"
OSARCH="linux/amd64 darwin/amd64 linux/arm windows/amd64"
DIRS="linux_amd64 darwin_amd64 linux_arm windows_amd64"
OUTDIR="pkg"

gox -osarch="$OSARCH" -output "$OUTDIR/$APPNAME-{{.OS}}_{{.Arch}}/$APPNAME"
for dir in $DIRS; do \
	(cp readme.md $OUTDIR/$APPNAME-$dir/readme.md) ;\
	(cp LICENSE $OUTDIR/$APPNAME-$dir/LICENSE) ;\
	(cd $OUTDIR && zip -q $APPNAME-$dir.zip -r $APPNAME-$dir) ;\
	echo "created '$OUTDIR/$APPNAME-$dir.zip', cleaning up..." ;\
	rm -rf $APPNAME-$dir;\
done
