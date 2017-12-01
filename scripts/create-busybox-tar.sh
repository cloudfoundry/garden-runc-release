#!/bin/bash
set -euo pipefail

base=$(cd "$(dirname "$0")/.." && pwd)

docker=docker
if [ -n "${BARE_METAL_DOCKER:-}" ]; then
  docker="sudo docker"
fi

if [ "$#" -ne 2 ]; then
  echo "usage: create-busybox-tar.sh <image> <blob dir>"
  exit 1
fi
image=$1
shift
blob_dir=$1
shift

$docker pull "$image"

tmpdir=$base/tmp
mkdir -p "$tmpdir"
trap 'rm -r $tmpdir' EXIT

$docker save "$image" > "$tmpdir"/image.tar

cd "$tmpdir"
mkdir image
tar -xf image.tar -C image

layer_count=$(find image -name layer.tar | wc -l)
if [ "$layer_count" -ne "1" ]; then
  echo "this only works for single-layer images, found $layer_count layers"
  exit 1
fi

gzip image/**/layer.tar
blob_name=${image//:/-}.tar.gz
mv image/**/layer.tar.gz "$blob_name"

cd "$base"
bosh add-blob "$tmpdir/$blob_name" "$blob_dir/$blob_name"

echo "remember to upload blobs, and delete entries for old ones!"
