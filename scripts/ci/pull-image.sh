#!/usr/bin/env bash
# Pulls a Docker Hub library image by its public.ecr.aws name. When ECR Public rate limits the pull
# ("toomanyrequests"), pulls the same image from mirror.gcr.io and tags it with the ECR name, so the
# compose files and docker run calls that name the ECR image find it locally.
set -uo pipefail

prefix=public.ecr.aws/docker/library/
image="${1:-}"
if [[ $# -ne 1 || "$image" != "$prefix"?* ]]; then
  echo "pull-image: want one image under $prefix, got: $*" >&2
  exit 2
fi

retry="$(dirname "${BASH_SOURCE[0]}")/retry.sh"
mirror="mirror.gcr.io/library/${image#"$prefix"}"

"$retry" docker pull -q "$image" && exit 0
echo "pull-image: $image failed, pulling $mirror instead" >&2
if "$retry" docker pull -q "$mirror" && docker tag "$mirror" "$image"; then
  exit 0
fi
echo "pull-image: both pulls failed: $image and $mirror" >&2
exit 1
