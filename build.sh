#!/bin/bash
#set -xe
set -e

vers=$(git describe --tags)
now=$(date +'%Y-%m-%d_%T')
ldflags="-X main.buildVersion=$vers -X main.buildTime=$now"
go_build_flags=""
make_go_workspace_only=no

function usage() {
  echo "Usage: "
  echo "  $0 [-a ARCH] [-o OS] [-t]"
  echo "  $0 -w"
  echo
  echo "In the first form, build anvil and extra tools."
  echo "The supported values of ARCH are '386' (for 32-bit x86), 'amd64' (for 64-bit x86_64) or arm64 (for 64-bit ARM)"
  echo "The supported values of OS are 'linux', 'windows', or 'darwin'"
  echo "The -t option trims the paths in the binaries"
  echo
  echo "In the second form, create a Go workspace file (go.work) to make it easier to build locally and exit without building."
  exit 1
}

GOARCHS=""
GOOSS=""

function parse_opts() {
  while getopts "a:o:thw" o
  do
    case "$o" in
      a)
        GOARCHS="$GOARCHS $OPTARG"
        ;;
      o)
        GOOSS="$GOOSS $OPTARG"
        ;;
      t)
        go_build_flags="-trimpath"
        ;;
      w)
        make_go_workspace_only=yes
        ;;
      h)
        usage
        ;;
      *)
        usage
        ;;
    esac
  done
}

function clean() {
  rm -f anvil.exe anvil-con.exe anvil
}

function find_file_in_parent_dirs() {
  target=$1
  local dir=$(pwd)
  path=""

  while true
  do
    path="$dir/$target"
    if [ -e "$path" ]
    then
      return
    fi

    if [ "$dir" = "/" ]
    then
      break
    fi

    dir=$(dirname $dir)
  done
}

function move_if_exists() {
  local src=$1
  local dst=$2

  if [ -f "$src" ]
  then
    mv $src $dst
  fi
}

function save_env_pre_macos_crosscompile() {
  OLDPATH=$PATH
  OLDCC=$CC
  OLDCXX=$CXX
  OLD_CGO_CFLAGS=$CGO_CFLAGS
  OLD_CGO_ENABLED=$CGO_ENABLED
}

function restore_env_post_macos_crosscompile() {
  export PATH=$OLDPATH
  export CC=$OLDCC
  export CXX=$OLDCXX
  export CGO_CFLAGS=$OLD_CGO_CFLAGS
  export CGO_ENABLED=$OLD_CGO_ENABLED
}

function set_env_for_macos_compile() {
  save_env_pre_macos_crosscompile

  if [ "$GOOS" != "darwin" ]
  then
    return
  fi

  # The compiler doesn't handle this function attribute _Nullable_result well.
  # It's used in the Mac SDK, and causes compilation errors. We redefine it to
  # do nothing.
  # Similarly, NS_FORMAT_ARGUMENT which is defined to use the function attribute
  # format_arg(A) also causes problems, so we redefine it as well.
  export CGO_CFLAGS="-D_Nullable_result= -DNS_FORMAT_ARGUMENT(A)= -DTARGET_OS_OSX"
  export CGO_ENABLED=1

  if [ "$DARWIN_CROSS_BIN" = "" ]
  then
    find_file_in_parent_dirs "osxcross"
    if [ "$path" = "" ]
    then
      echo "error: I can't find the directory osxcross in any parent directory. Will not build for macos"
      return
    fi

    DARWIN_CROSS_BIN="$path/target/bin"
  fi

  export PATH=$DARWIN_CROSS_BIN:$PATH

  if [ "$GOARCH" = "amd64" ]
  then
    export CC=x86_64-apple-darwin23.5-cc
    export CXX=x86_64-apple-darwin23.5-c++
  elif [ "$GOARCH" = "arm64" ]
  then
    export CC=arm64-apple-darwin23.5-cc
    export CXX=arm64-apple-darwin23.5-c++
  fi
}

function determine_binary_name() {
  dir=$1

  binary_name=$(basename $dir)
  special_binary_name=""
  if [ "$dir" = "../extras/cmd/autodump" ]
  then
    special_binary_name=aad
    if [ "$GOOS" = "windows" ]
    then
      special_binary_name=aad.exe
    fi
    binary_name=$special_binary_name
  fi
}

function build_one_dir() {
  dir=$1

  if [ "$GOOS" = "darwin" ]
  then
    set_env_for_macos_compile
  fi

  determine_binary_name $dir

  if [ "$GOOS" = "windows" -a "$dir" = "../editor/cmd/anvil" ]
  then
    if [ "$dir" = "../editor/cmd/anvil" ]
    then
      gogio -ldflags "$ldflags" -icon ../editor/misc/icon/anvil32b.png -buildmode exe -target windows $dir
      cp $dir/anvil.exe .
      go build -o anvil-con.exe -ldflags "$ldflags" $go_build_flags $dir
    fi
  else
    if [ "$special_binary_name" != "" ]
    then
      go build -o $special_binary_name -ldflags "$ldflags" $go_build_flags $dir
      echo "  note: building $dir as $binary_name"
    else
      go build -ldflags "$ldflags" $go_build_flags $dir
    fi
  fi

  if [ "$GOOS" = "darwin" ]
  then
    rcodesign_errmsg="rcodesign not available, so will not codesign binary.\n"
    rcodesign_errmsg="$rcodesign_errmsg if you are compiling natively on darwin and not cross compiling, this is fine."
    rcodesign sign $binary_name || echo $rcodesign_errmsg
  fi

  if [ "$GOOS" = "darwin" ]
  then
    restore_env_post_macos_crosscompile
  fi
}

function build() {
  rm -rf build
  mkdir build
  pushd build > /dev/null

  dirs_to_build="../editor/cmd/anvil ../extras/cmd/*"

  # Remove anvsshd if we are building for windows
  if [ "$GOOS" = "windows" ]
  then
    dirs_to_build=$(echo $dirs_to_build | sed -e 's/\.\..extras.cmd.anvsshd *//')
  fi

  echo "Building with cwd $(pwd)"
  for dir in $dirs_to_build
  do
    echo "Building $dir"
    build_one_dir $dir
  done

  popd > /dev/null
}

function build_all() {
  local msg=$1

  if [ "$msg" = "" ]
  then
    msg="native os and arch"
  fi

  echo "Building for $msg"
  build
}

function build_all_arch() {
  local msg=$1

  if [ "$GOARCHS" = "" ]
  then
    build_all "$msg"
    return
  fi

  for x in $GOARCHS
  do
    if [ "$msg" = "" ]
    then
      msg="arch: $x"
    else
      msg="$msg, arch: $x"
      echo $msg
    fi

    export GOARCH=$x
    build_all "$msg"
  done
}

function build_all_os_and_arch() {
  if [ "$GOOSS" = "" ]
  then
    build_all_arch
    return
  fi

  for x in $GOOSS
  do
    export GOOS=$x
    build_all_arch "os: $x"
  done
}

function init_go_workspace() {
  [ -e "go.work" ] && rm go.work
  go work init ./editor ./extras ./api/go/anvil
}

function deinit_go_workspace() {
  rm -f go.work
}

parse_opts $@

init_go_workspace
if [ "$make_go_workspace_only" = "yes" ]
then
  exit 0
fi

clean

echo "Invoke as '$0 -h' for help."
echo "Building version $vers"
build_all_os_and_arch

deinit_go_workspace
