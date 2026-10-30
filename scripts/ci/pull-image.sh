#!/usr/bin/env bash
# Pulls a Docker Hub library image by its public.ecr.aws name. When ECR Public answers with a quota or
# registry error ("toomanyrequests", "Data limit exceeded"), pulls the same image from mirror.gcr.io at
# once and tags it with the ECR name, so the compose files and docker run calls that name the ECR image
# find it locally. Retrying ECR cannot clear a quota, so only other failures (network errors) are retried.
set -uo pipefail

prefix=public.ecr.aws/docker/library/
image="${1:-}"
if [[ $# -ne 1 || "$image" != "$prefix"?* ]]; then
  echo "pull-image: want one image under $prefix, got: $*" >&2
  exit 2
fi

retry="$(dirname "${BASH_SOURCE[0]}")/retry.sh"
mirror="mirror.gcr.io/library/${image#"$prefix"}"

attempts="${RETRY_ATTEMPTS:-5}"
if out="$(docker pull -q "$image" 2>&1)"; then
  echo "$out"
  exit 0
fi
echo "$out" >&2
case "$out" in
*toomanyrequests* | *"limit exceeded"* | *"Rate exceeded"* | *"error from registry"*) ;;
*)
  if ((attempts > 1)); then
    RETRY_ATTEMPTS=$((attempts - 1)) "$retry" docker pull -q "$image" && exit 0
  fi
  ;;
esac
echo "pull-image: $image failed, pulling $mirror instead" >&2
if mirror_out="$("$retry" docker pull -q "$mirror" 2>&1)"; then
  docker tag "$mirror" "$image" && exit 0
  mirror_out="naming $mirror as $image failed"
fi
echo "$mirror_out" >&2
{
  echo "pull-image: both pulls failed"
  echo "pull-image: $image: $out"
  echo "pull-image: $mirror: $mirror_out"
} >&2
exit 1
