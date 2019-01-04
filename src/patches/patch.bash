# Apply all patches in a directory alphabetically.
function apply_patches() {
  patches_dir="$1"

  for patch in $( find "$patches_dir" -type f | sort ); do
    echo "applying patch: ${patch}"

    if patch -p1 -N -R --dry-run --silent < "$patch"; then
      echo "patch not necessary: ${patch}"
    else
      patch -p1 < "$patch"
    fi
  done
}
