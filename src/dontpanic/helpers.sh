script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"

hastput() {
  if command -v tput > /dev/null; then
    # Also check whether tput supports the terminal. The command below would fail if TERM is set to an unsupported value
    tput colors > /dev/null 2>&1
  fi
}

printSection() {
  echo -e -n "${blue}"
  printf '## %s\n' "$1"
  echo -e -n "${white}"
}

printAndCollect() {
  printSection "$1"
  /bin/sh -c "$2" 2>&1 | tee -a "$3" || printFailed
}

collect() {
  printSection "Collecting $1"
  /bin/sh -c "$2" > "$3" 2> "$3" || printFailed
}

massCollect() {
  printSection "Collecting and Archiving $1"
  for p in $(ps -eLo pid | awk 'NR>1'); do
    mkdir -p "process-data/$p"
    ls -lah "/proc/$p/fd" > "process-data/$p/fd" 2>/dev/null
    ls -lah "/proc/$p/ns" > "process-data/$p/ns" 2>/dev/null
    cat "/proc/$p/cgroup" > "process-data/$p/cgroup" 2>/dev/null
    cat "/proc/$p/status" > "process-data/$p/status" 2>/dev/null
    timeout -s SIGSTOP 5 cat "/proc/$p/stack" > "process-data/$p/stack" 2>/dev/null
  done
  (tar czf process-data.tgz process-data && rm -rf process-data) || printFailed
}

archive() {
  printSection "Archiving $1"
  tar czf "$3.tgz" -C "$2" "$3" || printFailed
}

archiveDir() {
  printSection "Archiving $1"
  tar czf "$3" -C "$2" . || printFailed
}

printRed() {
  echo -e "${red}$1${white}"
}

printGreen() {
  echo -e "${green}$1${white}"
}

printFailed() {
  printRed "Failed"
}

printBanner() {
  cat "$logo"
}

# no colours by default
logo="${script_dir}/thisisfine-no-colour"
bold=""
underline=""
standout=""
normal=""
black=""
red=""
green=""
yellow=""
blue=""
magenta=""
cyan=""
white=""
# check if stdout is a terminal...
if test -t 1; then
  if hastput; then

    # see if it supports colors...
    ncolors=$(tput colors)

    if test -n "$ncolors" && test $ncolors -ge 8; then
        logo="${script_dir}/thisisfine"
        bold="$(tput bold)"
        underline="$(tput smul)"
        standout="$(tput smso)"
        normal="$(tput sgr0)"
        black="$(tput setaf 0)"
        red="$(tput setaf 1)"
        green="$(tput setaf 2)"
        yellow="$(tput setaf 3)"
        blue="$(tput setaf 4)"
        magenta="$(tput setaf 5)"
        cyan="$(tput setaf 6)"
        white="$(tput setaf 7)"
    fi
  fi
fi
