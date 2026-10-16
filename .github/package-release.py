#!/usr/bin/env python3
"""Package verified CI artifacts; never collect fonts, keys or plugin source."""
import argparse
import hashlib
from pathlib import Path
import shutil
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def package(artifacts, output):
    output.mkdir(parents=True, exist_ok=True)
    for target in ('macos-arm64', 'macos-x86_64', 'windows-x64'):
        name = 'font-helper-' + target
        with tempfile.TemporaryDirectory() as directory:
            staged = Path(directory) / name
            (staged / 'dist').mkdir(parents=True)
            if target.startswith('macos-'):
                with zipfile.ZipFile(artifacts / (name + '-ci') / (name + '.zip')) as archive:
                    binary = archive.read(name + '/dist/' + name)
                (staged / 'dist' / name).write_bytes(binary)
                (staged / 'dist' / name).chmod(0o755)
                shutil.copy2(ROOT / 'start-macos.command', staged)
            else:
                binary = (artifacts / 'font-helper-windows-ci' / 'font-helper-windows.exe').read_bytes()
                (staged / 'dist' / 'font-helper-windows.exe').write_bytes(binary)
                shutil.copy2(ROOT / 'start-windows.cmd', staged)
                shutil.copy2(ROOT / 'helper-windows.ps1', staged)
            shutil.copy2(ROOT / 'README.md', staged)
            shutil.copy2(ROOT / 'LICENSE', staged)
            shutil.copy2(ROOT / 'CHANGELOG.md', staged)
            shutil.copytree(ROOT / 'licenses', staged / 'licenses')
            with zipfile.ZipFile(output / (name + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
                for path in sorted(staged.rglob('*')):
                    if path.is_file():
                        archive.write(path, path.relative_to(staged.parent))
    checksums = []
    for path in sorted(output.glob('font-helper-*.zip')):
        checksums.append(hashlib.sha256(path.read_bytes()).hexdigest() + '  ' + path.name)
    assert len(checksums) == 3
    (output / 'SHA256SUMS.txt').write_text('\n'.join(checksums) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('artifacts', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    package(args.artifacts.resolve(), args.output.resolve())
