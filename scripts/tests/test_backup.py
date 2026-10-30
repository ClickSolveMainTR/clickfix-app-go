"""Exercise backup completion ordering without database or network access."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'backup-postgres.sh'


class BackupTest(unittest.TestCase):
    def run_backup(self, fail_upload=False):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            tools = {
                'pg_dump': '#!/bin/sh\nfor a in "$@"; do case "$a" in --file=*) printf archive > "${a#--file=}";; esac; done\n',
                'pg_restore': '#!/bin/sh\nexit 0\n',
                'aws': '#!/bin/sh\nprintf "%s\\n" "$*" >> "$BACKUP_TEST_LOG"\nif [ "$FAIL_UPLOAD" = 1 ]; then exit 1; fi\n',
            }
            for name, content in tools.items():
                executable = root / name
                executable.write_text(content)
                executable.chmod(0o700)
            log = root / 'calls'
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                       ENV_FILE=str(root / 'missing.env'), DB_NAME='test', DB_USER='test',
                       S3_ENDPOINT='https://store.example.test', S3_ACCESS_KEY='test',
                       S3_SECRET_KEY='test', BACKUP_BUCKET='backups', BACKUP_TEST_LOG=str(log),
                       FAIL_UPLOAD='1' if fail_upload else '0')
            result = subprocess.run(['bash', str(SCRIPT)], env=env, capture_output=True, text=True)
            return result, log.read_text().splitlines()

    def test_completion_marker_is_uploaded_last(self):
        result, calls = self.run_backup()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 3)
        self.assertIn('postgres.dump', calls[0])
        self.assertIn('SHA256SUMS', calls[1])
        self.assertIn('complete.json', calls[2])

    def test_upload_failure_never_publishes_completion(self):
        result, calls = self.run_backup(fail_upload=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(calls), 1)
        self.assertNotIn('complete.json', '\n'.join(calls))


if __name__ == '__main__':
    unittest.main()
