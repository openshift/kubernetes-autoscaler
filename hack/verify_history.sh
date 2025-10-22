#!/bin/bash
# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Verifies git commits starting from predefined prefix.

upstreamurl=https://github.com/kubernetes/autoscaler
upstreambranch=cluster-autoscaler-release-1.34
upstream=''
# default to checking most recent commit only if not run by CI pipeline
check_base="${PULL_BASE_SHA:-HEAD^}"
check_sha="${PULL_PULL_SHA:-HEAD}"

get_upstream() {
  if [ -n "$upstream" ]; then
    return 0
  fi
  while read name url ignore; do
    if [ "$url" = "$upstreamurl" ]; then
      upstream=$name
      return 0
     fi
  done < <(git remote -v)
  git remote add upstreamtmp "$upstreamurl" && git fetch upstreamtmp && upstream=upstreamtmp
  return $?
}

upstream_has() {
  get_upstream
  if ! [ -n "$upstream" ]; then
    return 1
  fi
  git merge-base --is-ancestor "$1" "$upstream/$upstreambranch"
  return $?
}

read -d '' help_message << EOF

commit messages should look like one of:
UPSTREAM: <carry>: message  (commits that should be carried indefinitely)
UPSTREAM: <drop>: message   (commits that should be dropped on the next upstream rebase)
UPSTREAM: 1234: message     (commits that should be carried until an upstream rebase includes upstream PR 1234)
EOF

prefix='UPSTREAM: ([0-9]+|<(carry|drop)>): '

echo "examining commits between $check_base and $check_sha"
echo

while read -r hash message; do
  # valid commits either have the prefix or are part of the upstream branch $upstreambranch
  if [[ "$message" =~ ^$prefix ]] || upstream_has "$hash"; then
    echo "$message"
    continue
  fi
  echo "Git history in this PR doesn't conform to set commit message standards. Offending commit message is:"
  echo "$message"
  echo
  echo "$help_message"
  exit 1
done < <(git log "$check_base".."$check_sha" --pretty="%H %s" --no-merges)

echo
echo "All looks good"
