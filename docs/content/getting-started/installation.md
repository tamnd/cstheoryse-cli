---
title: "Installation"
description: "Install cst2 from a release, with go install, or from source."
weight: 20
---

## Prebuilt binaries

Every [release](https://github.com/tamnd/cstheoryse-cli/releases) carries archives for Linux, macOS,
and Windows on amd64 and arm64, plus deb, rpm, and apk packages for Linux.
Download, unpack, put `cst2` on your `PATH`, done. The `checksums.txt`
on each release is signed with keyless [cosign](https://docs.sigstore.dev/) if
you want to verify before running.

## With Go

```bash
go install github.com/tamnd/cstheoryse-cli/cmd/cst2@latest
```

That puts `cst2` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless
you moved it. Make sure that directory is on your `PATH`.

## From source

```bash
git clone https://github.com/tamnd/cstheoryse-cli
cd cstheoryse-cli
make build        # produces ./bin/cst2
./bin/cst2 version
```

## Container image

```bash
docker run --rm ghcr.io/tamnd/cst2:latest --help
```

## Checking the install

```bash
cst2 version
```

prints the version and exits.
