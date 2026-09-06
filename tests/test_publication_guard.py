from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class PublicationGuardTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.repo = Path(self.directory.name)
        (self.repo / 'scripts').mkdir()
        shutil.copyfile(ROOT / 'scripts/check_publication.py', self.repo / 'scripts/check_publication.py')
        shutil.copytree(ROOT / 'docs/screenshots', self.repo / 'docs/screenshots')
        self.git('init', '-q')
        self.git('add', '.')

    def git(self, *args):
        subprocess.run(['git', *args], cwd=self.repo, check=True, capture_output=True)

    def check(self):
        return subprocess.run([sys.executable, 'scripts/check_publication.py'], cwd=self.repo,
                              capture_output=True, text=True)

    def test_reviewed_images_pass(self):
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_changed_image_or_appended_metadata_is_rejected(self):
        path = self.repo / 'docs/screenshots/dashboard.jpg'
        path.write_bytes(path.read_bytes() + b'synthetic metadata')
        self.git('add', '.')
        result = self.check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('screenshot bytes changed', result.stderr)

    def test_unreviewed_image_is_rejected_even_with_uppercase_suffix(self):
        shutil.copyfile(self.repo / 'docs/screenshots/dashboard.jpg', self.repo / 'unreviewed.JPG')
        self.git('add', '.')
        result = self.check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('private or generated file category', result.stderr)

    def test_financial_identifier_shape_is_rejected_without_printing_value(self):
        number = '4' + '7' * 15
        (self.repo / 'example.txt').write_text('account=' + number)
        self.git('add', '.')
        result = self.check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('financial-identifier-shaped', result.stderr)
        self.assertNotIn(number, result.stdout + result.stderr)

    def test_go_pseudoversion_timestamp_is_not_a_financial_identifier(self):
        timestamp = '2020' + '0102030405'
        (self.repo / 'go.mod').write_text('example.invalid/module v0.6.0-dev.0.' + timestamp + '-abcdef012345\n')
        self.git('add', '.')
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()
