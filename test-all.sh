#!/bin/bash

function init_go_workspace() {
  [ -e "go.work" ] && rm go.work
  go work init ./editor ./extras ./api/go/anvil
}

function deinit_go_workspace() {
  rm -f go.work go.work.sum
}

status=0
function update_status() {
  s=$?
  if [ "$s" != "0" ]
  then
    status=$s
  fi
}

init_go_workspace

echo "testing editor/"
pushd editor > /dev/null 2>&1
go test ./...
update_status
popd > /dev/null 2>&1

echo "testing extras/"
pushd extras > /dev/null 2>&1
go test ./...
update_status
popd > /dev/null 2>&1

deinit_go_workspace

exit $status

