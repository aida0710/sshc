#!/usr/bin/env python3
"""Check the destructive runner's target selection without contacting a device."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


RUNNER = Path(__file__).with_name("run-vault-lifecycle-test.sh").resolve()
ACTIVITY = "com.github.aida0710.sshc.MainActivity"


class VaultLifecycleRunnerTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="sshc android runner ")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.sdk = self.root / "sdk"
        self.log = self.root / "adb.log"
        self.metadata = self.root / "badging.txt"
        self.apk = self.root / "fixture.apk"
        self.apk.touch()
        self.install_script(self.sdk / "platform-tools/adb", """#!/bin/sh
printf '%s\\n' "$*" >> "$SSHC_TEST_ADB_LOG"
case "$*" in
  'shell getprop ro.build.version.sdk') echo 36 ;;
  'shell getprop ro.kernel.qemu') echo "${SSHC_TEST_EMULATED:-1}" ;;
  shell\\ pidof\\ *) echo 12345 ;;
esac
""")
        self.install_script(self.sdk / "build-tools/36.0.0/aapt", """#!/bin/sh
cat "$SSHC_TEST_BADGING"
""")
        self.install_script(self.root / "bin/node", "#!/bin/sh\nexit 0\n")
        self.install_script(self.root / "bin/sleep", "#!/bin/sh\nexit 0\n")
        self.env = {
            **os.environ,
            "ANDROID_SDK_ROOT": str(self.sdk),
            "SSHC_ANDROID_NODE": str(self.root / "bin/node"),
            "SSHC_ANDROID_ARTIFACTS": str(self.root / "artifacts"),
            "SSHC_TEST_ADB_LOG": str(self.log),
            "SSHC_TEST_BADGING": str(self.metadata),
            "PATH": str(self.root / "bin") + os.pathsep + os.environ.get("PATH", ""),
        }
        self.env.pop("SSHC_ANDROID_AAPT", None)

    @staticmethod
    def install_script(path, source):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source)
        path.chmod(0o755)

    def run_fixture(self, package, *, debuggable=True, activity=ACTIVITY):
        self.metadata.write_text(
            f"package: name='{package}' versionName='fixture'\n"
            + ("application-debuggable\n" if debuggable else "")
            + f"launchable-activity: name='{activity}' label=''\n"
        )
        result = subprocess.run(
            ["sh", str(RUNNER), str(self.apk)],
            env=self.env,
            cwd=RUNNER.parents[2],
            text=True,
            capture_output=True,
            timeout=10,
            check=False,
        )
        calls = self.log.read_text().splitlines() if self.log.exists() else []
        return result, calls

    def assert_successful_target(self, package):
        result, calls = self.run_fixture(package)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"shell pm clear {package}", calls)
        self.assertEqual(calls.count(f"shell am start -W -n {package}/{ACTIVITY}"), 2)
        self.assertIn(f"shell am force-stop {package}", calls)
        return calls

    def test_dev_package_uses_unchanged_activity_namespace(self):
        calls = self.assert_successful_target("com.github.aida0710.sshc.dev")
        self.assertNotIn("shell pm clear com.github.aida0710.sshc", calls)

    def test_legacy_debug_package_is_read_from_the_apk(self):
        self.assert_successful_target("com.github.aida0710.sshc")

    def test_release_apk_is_rejected_before_any_device_access(self):
        result, calls = self.run_fixture("com.github.aida0710.sshc", debuggable=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("debuggable APK is required", result.stderr)
        self.assertEqual(calls, [])

    def test_foreign_package_is_rejected_before_any_device_access(self):
        result, calls = self.run_fixture("example.other.app")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unexpected package", result.stderr)
        self.assertEqual(calls, [])

    def test_wrong_launcher_is_rejected_before_any_device_access(self):
        result, calls = self.run_fixture(
            "com.github.aida0710.sshc.dev", activity="example.other.Activity"
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unexpected launcher", result.stderr)
        self.assertEqual(calls, [])

    def test_physical_device_is_rejected_before_install_or_clear(self):
        self.env["SSHC_TEST_EMULATED"] = "0"
        result, calls = self.run_fixture("com.github.aida0710.sshc.dev")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("physical Android device", result.stderr)
        self.assertFalse(any(call.startswith("install ") or "pm clear" in call for call in calls))


if __name__ == "__main__":
    unittest.main()
