#!/usr/bin/env sh
set -e
version="${VERSION}"
latest="${LATEST:-0}"
push="${PUSH:-0}"
image="docker.io/benfiola/external-dns-mikrotik-webhook"

if [ "${version}" = "" ]; then
    1>&2 echo "VERSION unset"
    exit 1
fi

confirm() {
    value="n"
    while [ ! "$value" = "y" ]; do
        printf "confirm [y/n]:"
        read value
        if [ "$value" = "n" ]; then
            1>&2 echo "user aborted operation"
            exit 1
        fi
    done
}

arg_latest=""
if [ "${latest}" = "1" ]; then
    latest_image="${image}:latest"
    arg_latest="--tag ${latest_image}"
fi
arg_push=""
if [ "${push}" = "1" ]; then
    arg_push="--push"
fi
command="docker buildx build --platform linux/arm64 --platform linux/amd64 --load --tag ${image}:${version} ${arg_latest} ${arg_push} ."

echo "version: ${version}"
echo "latest: ${latest}"
echo "push: ${push}"
echo "command: ${command}"

confirm

$command