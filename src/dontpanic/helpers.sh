script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"

function hastput {
  command -v tput > /dev/null
}

printSection() {
  termwidth=80
  if hastput; then
    termwidth="$(tput cols)"
  fi

  padding="$(printf '%0.1s' -{1..500})"
  leftmargin=3
  msgwidth="${#1}"
  rightmargin="$(($termwidth-$leftmargin-msgwidth-2))"

  echo -e -n "${blue}"
  printf '%*.*s[%s]%*.*s\n' 0 "$leftmargin" "$padding" "$1" 0 "$rightmargin" "$padding"
  echo -e -n "${white}"
}

function printAndCollect() {
  printSection "$1"
  /bin/sh -c "$2" > >(tee -a "$3") 2> >(tee -a "$3" >&2)
  if [ $? -ne 0 ]; then
    printFailed
  fi
}

function collect() {
  printSection "Collecting $1"
  /bin/sh -c "$2" > "$3" 2> "$3"
  if [ $? -ne 0 ]; then
    printFailed
  fi
}

function archive {
  printSection "Archiving $1"
  tar czf "$3.tgz" -C "$2" "$3"
  if [ $? -ne 0 ]; then
    printFailed
  fi
}

function archiveDir {
  printSection "Archiving $1"
  tar czf "$3" -C "$2" .
  if [ $? -ne 0 ]; then
    printFailed
  fi
}

function printRed() {
  echo -e "${red}$1${white}"
}

function printGreen() {
  echo -e "${green}$1${white}"
}

function printFailed() {
  printRed "Failed"
}

function printBanner {
  cat $logo
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
