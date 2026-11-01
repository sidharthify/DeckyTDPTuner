#!/bin/sh
# zips up whatever is already built in dist/ and backend/out/
set -e
cd "$(dirname "$0")/.."

version=$(node -p 'require("./package.json").version')
dir=out/DeckyTDPTuner

if [ ! -f dist/index.js ] || [ ! -x backend/out/tdptuner ]; then
	echo "build first (pnpm build and make -C backend)" >&2
	exit 1
fi

rm -rf out
mkdir -p "$dir/bin"
cp -r dist main.py plugin.json package.json LICENSE README.md "$dir/"
cp backend/out/tdptuner "$dir/bin/"

(cd out && zip -qr "DeckyTDPTuner-$version.zip" DeckyTDPTuner)
echo "out/DeckyTDPTuner-$version.zip"
