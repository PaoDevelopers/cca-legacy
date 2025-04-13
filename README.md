# YKPS CCA Selection System

This is my internal assessment for the IB Diploma Programme&rsquo;s Computer
Science (Higher Level) course. The `iadocs` directory contains documentation
required by the IB, while the `docs` directory contains the documentation that
I actually plan to distribute for the production system.

## Build

You need a [Go](https://go.dev) toolchain, [Pygments](https://pygments.org),
[Pandoc](https://pandoc.org), [GNU make](https://www.gnu.org/software/make/),
[TeX Live](https://tug.org/texlive/),
[minify](https://github.com/tdewolff/minify), and
[TypeScript](https://www.typescriptlang.org). Minify must be present in `$PATH`
as `gominify`. A TypeScript compiler must be present in `$PATH` as `tsc`.

Then just run `make`.

## Repository mirrors

* [Upstream repo](https://git.runxiyu.org/cca.git)
* [SourceHut mirror](https://git.sr.ht/~runxiyu/cca)
* [Codeberg mirror](https://codeberg.org/runxiyu/cca)
* [GitHub mirror](https://github.com/runxiyu/cca)

## Misc links

* [Issue tracker](https://todo.sr.ht/~runxiyu/cca)

## Documentation

The following link to the raw HTML source of the documentation as served by
cgit. However, a demo instance would provide better documentation, but I'm
not currently hosting one.

* [Admin handbook](https://git.runxiyu.org/cca.git/plain/docs/admin_handbook.html)
* [IA cover page](https://git.runxiyu.org/cca.git/plain/iadocs/cover_page.htm)

## Notice on cryptographic software

This distribution includes cryptographic software. The country in which you
currently reside may have restrictions on the import, possession, use, and/or
re-export to another country, of encryption software. BEFORE using any
encryption software, please check your country's laws, regulations and policies
concerning the import, possession, or use, and re-export of encryption
software, to see if this is permitted. See http://www.wassenaar.org/ for more
information.

The U.S. Government Department of Commerce, Bureau of Industry and Security
(BIS), has classified this software as Export Commodity Control Number (ECCN)
5D002.C.1, which includes information security software using or performing
cryptographic functions with asymmetric algorithms. The form and manner of this
distribution makes it eligible for export under the License Exception ENC
Technology Software Unrestricted (TSU) exception (see the BIS Export
Administration Regulations, Section 740.13) for both object code and source
code.
