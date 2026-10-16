"""Test CPU dispatch independently of the CPU of the test machine."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


@unittest.skipUnless(os.name == 'posix', 'macOS shell launcher tests')
class MacPackagingTest(unittest.TestCase):
    def launch(self, host, binaries, arm_hardware='0', legacy_compatible=False):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            shutil.copy(Path(__file__).with_name('start-macos.command'), root)
            (root / 'dist').mkdir()
            (root / 'commands').mkdir()
            scripts = {'uname': 'echo ' + host,
                       'sysctl': 'echo ' + arm_hardware,
                       'lipo': 'exit ' + ('0' if legacy_compatible else '1')}
            for name, body in scripts.items():
                path = root / 'commands' / name
                path.write_text('#!/bin/sh\n' + body + '\n')
                path.chmod(0o755)
            for binary in binaries:
                path = root / 'dist' / binary
                path.write_text('#!/bin/sh\necho selected-' + binary + '\necho "arg:$1"\n')
                path.chmod(0o755)
            env = dict(os.environ, PATH=str(root / 'commands') + os.pathsep + os.environ['PATH'])
            return subprocess.run(['/bin/sh', str(root / 'start-macos.command'), '--help'],
                                  env=env, capture_output=True, text=True, stdin=subprocess.DEVNULL)

    def test_intel_selects_x86_binary(self):
        result = self.launch('x86_64', ['font-helper-macos-x86_64', 'font-helper-macos-arm64'])
        self.assertEqual(result.returncode, 0)
        self.assertIn('selected-font-helper-macos-x86_64', result.stdout)
        self.assertIn('arg:--help', result.stdout)

    def test_apple_silicon_selects_arm_binary_under_rosetta(self):
        result = self.launch('x86_64', ['font-helper-macos-x86_64', 'font-helper-macos-arm64'], '1')
        self.assertIn('selected-font-helper-macos-arm64', result.stdout)
        self.assertEqual(result.returncode, 0)

    def test_wrong_arch_has_clear_failure(self):
        result = self.launch('x86_64', ['font-helper-macos-arm64'])
        self.assertEqual(result.returncode, 1)
        self.assertIn('macOS-x86_64', result.stderr)

    def test_compatible_legacy_binary(self):
        result = self.launch('x86_64', ['font-helper-macos'], legacy_compatible=True)
        self.assertEqual(result.returncode, 0)
        self.assertIn('selected-font-helper-macos', result.stdout)

    def test_incompatible_legacy_binary_rejected(self):
        result = self.launch('x86_64', ['font-helper-macos'])
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('selected-', result.stdout)
