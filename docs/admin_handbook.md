---
lang: en
title: CCA Admin Handbook
---

## Introduction

This handbook guides you in installing, configuring, and managing your CCA Selection System (CCASS) instance.

## Downloading

You may obtain a stable or development version. The stable version is recommended for production.

- To obtain a stable version, go to the [release page](https://git.runxiyu.org/ykps/cca.git/refs/) and download a source tarball of the latest version, or go to the [sr.ht release page](https://git.sr.ht/~runxiyu/cca/refs/) and download a pre-built tarball for your platform.
- To obtain an unstable development version, clone the development repository at [`https://git.runxiyu.org/ykps/cca.git/`](https://git.runxiyu.org/ykps/cca.git/refs/), or download the latest development snapshot's tarball at [`https://git.runxiyu.org/ykps/cca.git/snapshot/cca-master.tar.gz`](https://git.runxiyu.org/ykps/cca.git/snapshot/cca-master.tar.gz).

## External dependencies

You may skip this step if using pre-built tarballs.

You need a [Go](https://go.dev) toolchain, [Pygments](https://pygments.org), [Pandoc](https://pandoc.org), [GNU make](https://www.gnu.org/software/make/), [TeX Live](https://tug.org/texlive/), [minify](https://github.com/tdewolff/minify), and [TypeScript](https://www.typescriptlang.org). Minify must be present in `$PATH` as `minify`. A TypeScript compiler must be present in `$PATH` as `tsc`.

It is possible to build with only the Go toolchain, but the current build system does not support building the program without building the corresponding documentation (including IA documentation which accounts for the huge TeX Live installation). This will be enhanced in the future.

The Go toolchain will fetch a few more dependencies. You may wish to set a custom Go module proxy (such as via `export GOPROXY='https://goproxy.io'`) if it stalls or is too slow. This is likely necessary for users in Mainland China due to firewall restrictions.

## Building

You may skip this step if using pre-built tarballs.

Just type `make` after entering the repository.

The built files will appear in `dist/`. The binary, with all runtime resources other than the configuration file embedded, is located at `dist/cca`. A minified copy of the documentation, including a sample configuration file, is located at `dist/docs/`. IA documentation is located at `dist/iadocs`.

## Configuration

Copy [the example configuration file](./cca.scfg.example) to `cca.scfg` in the working directory where you intend to run CCASS. Then edit it according to the comments, though you may wish to pay attention to the following:

-   CCASS natively supports serving over clear text HTTP or over HTTPS. HTTPS is required for production setups as Microsoft Entra ID does not allow clear-text HTTP redirect URLs for non-`localhost` access.
-   Note that CCASS is designed to be directly exposed to clients due to the lacking performance of standard reverse proxy setups, although there is nothing that otherwise prevents it from being used behind a reverse proxy. Reverse proxies must forward WebSocket connection upgrade headers when the `/ws` endpoint is being accessed.
-   You must [create an app registration on the Azure portal](https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade) and complete the corresponding configuration options, as shown below.
-   You must set up PostgreSQL. See below.

## Microsoft Entra ID setup

A Web redirect URL is needed and must be set to `/auth` from the base of the accessible URL (for example, `https://cca.ykpaoschool.cn/ws` if the site is accessible at `https://cca.ykpaoschool.cn`). &ldquo;ID tokens&rdquo; must be selected. The following optional claims must be configured:
* `email`
* `family_name`
* `given_name`
* `preferred_username`
* `groups` (ID tokens must be configured to receive Group IDs)

The application needs the following delegated permissions:
* `email`
* `offline_access`
* `openid`
* `profile`
* `User.Read`

[An example manifest](./azure.json) is available.

## Database setup

A working PostgreSQL setup is required. It is recommended to set up UNIX socket authentication and set the user running CCASS as the database owner while creating the database.

Before first run, run <code>psql <i>dbname</i> -f sql/schema.sql</code> to create the database tables, where <code><i>dbname</i></code> is the name of the database.

Using the same database for different versions of CCASS is currently unsupported, although it should be trivial to manually migrate the database.

