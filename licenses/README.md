# Bundled runtime notices

These are unmodified upstream notices for runtime components used by the standalone executables. The helper's own software and source code are licensed under the MIT License in the package root. These dependency notices do not change that license, and the helper's MIT license does not apply to these third-party components or to users' fonts. Font rights and permitted use remain the user's responsibility. System frameworks are supplied by the operating system.

- macOS: CPython 3.13.15, PyInstaller 6.22.3; CPython's macOS installer recipe uses OpenSSL 3.0.21 and liblzma from XZ 5.2.3.
- Windows: CPython 3.13.16, PyInstaller 6.22.3; its external dependency recipe lists bzip2 1.0.8, libffi 3.4.4, OpenSSL 3.5.9, XZ 5.2.5 and zlib 1.3.1. Microsoft runtime binaries are distributed as part of the official CPython runtime.
- CPython `Doc/license.rst` includes additional incorporated-software notices, including OpenSSL, libffi and zlib. XZ COPYING distinguishes public-domain liblzma from unrelated command-line tools; those tools are not included in these packages.

Original sources:

- https://github.com/python/cpython/blob/v3.13.15/LICENSE
- https://github.com/python/cpython/blob/v3.13.15/Doc/license.rst
- https://github.com/python/cpython/blob/v3.13.16/LICENSE
- https://github.com/python/cpython/blob/v3.13.16/Doc/license.rst
- https://github.com/python/cpython/blob/v3.13.15/Mac/BuildScript/build-installer.py
- https://github.com/python/cpython/blob/v3.13.16/PCbuild/get_externals.bat
- https://github.com/pyinstaller/pyinstaller/blob/v6.22.3/COPYING.txt
- https://github.com/python/cpython-source-deps/blob/bzip2-1.0.8/LICENSE
- https://github.com/libffi/libffi/blob/v3.4.4/LICENSE
- https://github.com/tukaani-project/xz/blob/v5.2.3/COPYING
- https://github.com/tukaani-project/xz/blob/v5.2.5/COPYING

When rebuilding, check the actual interpreter and bundled libraries, and update these notices for any changed dependency versions.
