#!/usr/bin/env python3
"""Isolated fixture checks: no launchctl or real user Library changes."""
import hashlib
import pathlib
import shutil
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
MOCK = r'''
fixture_launchctl() {
  case "$1" in
    print) [ -f "$fixture_state" ] || return 113; printf ' program = %s\n' "$(cat "$fixture_state")" ;;
    bootout) rm -f "$fixture_state" ;;
    bootstrap)
      if [ -f "$fixture_fail" ]; then
        if [ "$(cat "$fixture_fail")" = rollback ]; then printf rollback > "$fixture_mv_fail"; fi
        rm -f "$fixture_fail"; return 98; fi
      /usr/libexec/PlistBuddy -c 'Print :ProgramArguments:0' "$3" > "$fixture_state" ;;
    *) return 99 ;;
  esac
}
fixture_mv() {
  if [ -f "$fixture_mv_fail" ]; then
    mode=$(cat "$fixture_mv_fail")
    case "$mode:$1" in
      mid:*/.font-stage.*/owner.sha256|rollback:*/previous/figma-local-font-service)
        rm -f "$fixture_mv_fail"; return 97 ;;
    esac
  fi
  /bin/mv "$@"
}
'''

class Lifecycle(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='figmapdf-fixture-')
        self.root = pathlib.Path(self.tmp.name)
        self.home = self.root / 'user 한글 & spaces'
        (self.home / 'Library').mkdir(parents=True)
        self.package = self.root / 'package relocated & 한글'
        shutil.copytree(SOURCE / 'dist/figma-pdf-font-service-macos-universal', self.package)
        # Inject mocked capability references into a fixture copy, never override HOME.
        text = (self.package / 'lifecycle.sh').read_text()
        text = text.replace('$HOME/', '$font_test_home/').replace('launchctl ', 'fixture_launchctl ').replace('mv ', 'fixture_mv ')
        (self.package / 'lifecycle.sh').write_text(text)
        names = ['figma-local-font-service', 'Install.command', 'Uninstall.command', 'lifecycle.sh', 'README.md', 'job.plist', 'LICENSE']
        (self.package / 'SHA256SUMS.txt').write_text(''.join(f'{hashlib.sha256((self.package / n).read_bytes()).hexdigest()}  {n}\n' for n in names))
        self.state = self.root / 'state'
        self.failure = self.root / 'fail'
        self.installed = self.home / 'Library/Application Support/FigmaPDFExporter/FontService'

    def tearDown(self):
        self.tmp.cleanup()

    def run_action(self, action, ok=True):
        command = MOCK + '\nsource "$4/lifecycle.sh"\nfont_' + action
        result = subprocess.run(['/bin/bash', '-c', command, str(self.package / 'Install.command'), str(self.home), str(self.state), str(self.failure), str(self.package)], env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin', 'font_test_home':str(self.home), 'fixture_state':str(self.state), 'fixture_fail':str(self.failure), 'fixture_mv_fail':str(self.root / 'mvfail')}, capture_output=True, text=True)
        self.assertEqual(result.returncode == 0, ok, result.stdout + result.stderr)
        return result

    def test_install_relocate_reinstall_uninstall_preserves_unknown(self):
        self.run_action('install')
        self.assertTrue((self.installed / 'figma-local-font-service').exists())
        self.run_action('install')
        (self.installed / 'keep.txt').write_text('unrelated')
        self.run_action('uninstall')
        self.assertFalse(self.state.exists())
        self.assertEqual((self.installed / 'keep.txt').read_text(), 'unrelated')
        self.assertFalse((self.installed / 'figma-local-font-service').exists())

    def test_bootstrap_failure_rolls_back_previous_files_and_job(self):
        self.run_action('install')
        old = (self.installed / 'figma-local-font-service').read_bytes()
        self.failure.touch()
        self.run_action('install', False)
        self.assertEqual((self.installed / 'figma-local-font-service').read_bytes(), old)
        self.assertTrue(self.state.exists())

    def test_first_bootstrap_failure_removes_only_transaction_files(self):
        self.failure.touch()
        self.run_action('install', False)
        self.assertFalse((self.installed / 'figma-local-font-service').exists())
        self.assertFalse(self.state.exists())
        self.assertFalse(self.installed.exists())
        self.run_action('install')
        self.assertTrue(self.state.exists())

    def test_mid_transaction_move_failure_restores_old_files_and_loaded_job(self):
        self.run_action('install')
        old = (self.installed / 'figma-local-font-service').read_bytes()
        (self.root / 'mvfail').write_text('mid')
        result = self.run_action('install', False)
        self.assertIn('were restored', result.stderr)
        self.assertEqual((self.installed / 'figma-local-font-service').read_bytes(), old)
        self.assertTrue(self.state.exists())

    def test_rollback_move_failure_preserves_backup_and_reports_incomplete(self):
        self.run_action('install')
        old = (self.installed / 'figma-local-font-service').read_bytes()
        self.failure.write_text('rollback')
        result = self.run_action('install', False)
        self.assertIn('rollback was incomplete', result.stderr)
        self.assertNotIn('were restored', result.stderr)
        backups = list(self.installed.parent.glob('.font-stage.*/previous/figma-local-font-service'))
        self.assertEqual(len(backups), 1)
        self.assertEqual(backups[0].read_bytes(), old)
        self.assertFalse(self.state.exists())

    def test_second_installer_lock_refused_and_preserved(self):
        lock = self.installed.parent / '.font-install.lock'
        lock.mkdir(parents=True)
        self.run_action('install', False)
        self.assertTrue(lock.is_dir())
        self.assertFalse(self.state.exists())

    def test_foreign_job_refused(self):
        self.state.write_text('/other/development/service')
        self.run_action('install', False)
        self.assertEqual(self.state.read_text(), '/other/development/service')

    def test_marker_corruption_and_symlink_refused(self):
        self.run_action('install')
        (self.installed / 'owner.sha256').write_text('bad')
        self.run_action('uninstall', False)
        self.assertTrue(self.state.exists())
        (self.installed / 'owner.sha256').unlink()
        (self.installed / 'owner.sha256').symlink_to(self.root / 'elsewhere')
        self.run_action('install', False)

    def test_package_corruption_refused(self):
        (self.package / 'job.plist').write_text('bad')
        self.run_action('install', False)
        self.assertFalse(self.state.exists())

    def test_parent_symlink_refused(self):
        (self.root / 'elsewhere').mkdir()
        (self.home / 'Library/Application Support').symlink_to(self.root / 'elsewhere')
        self.run_action('install', False)
        self.assertEqual(list((self.root / 'elsewhere').iterdir()), [])

if __name__ == '__main__':
    unittest.main()
