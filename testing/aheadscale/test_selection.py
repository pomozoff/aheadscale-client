"""Контракт выбора CI: сборки только явно, проверки не теряются при изменениях."""
import unittest

from selection import select_checks


class SelectionTests(unittest.TestCase):
    def test_push_and_documentation_do_not_start_product_jobs(self):
        self.assertFalse(any(select_checks("push", "", "", ["client/internal/dns/server.go"]).values()))
        self.assertFalse(any(select_checks("pull_request", "", "", ["docs/aheadscale/test.md"]).values()))

    def test_auth_change_selects_only_auth(self):
        self.assertEqual(select_checks("pull_request", "", "", ["management/server/registration/handler.go"]),
                         dict(unit=False, packaging=False, auth=True, windows=False))

    def test_packaging_change_does_not_start_core_or_windows(self):
        self.assertEqual(select_checks("pull_request", "", "", ["packaging/aheadscale/package.py"]),
                         dict(unit=False, packaging=True, auth=False, windows=False))

    def test_common_and_unknown_code_are_conservative(self):
        for path in ("go.mod", "shared/management/client.go", "new-component/main.go"):
            with self.subTest(path=path):
                self.assertTrue(all(select_checks("pull_request", "", "", [path]).values()))

    def test_client_changes_include_windows_identity(self):
        result = select_checks("pull_request", "", "", ["client/iface/device/device_windows.go"])
        self.assertTrue(result["unit"])
        self.assertTrue(result["windows"])
        self.assertFalse(result["auth"])

    def test_manual_suite_does_not_expand_to_all_tests(self):
        for suite in ("unit", "packaging", "auth", "windows"):
            with self.subTest(suite=suite):
                result = select_checks("workflow_dispatch", suite, "all", [])
                self.assertEqual([key for key, selected in result.items() if selected], [suite])
        for suite in ("server", "ci-contract"):
            self.assertFalse(any(select_checks("workflow_dispatch", suite, "all", []).values()))

    def test_build_needs_platform_checks_without_auth(self):
        self.assertEqual(select_checks("workflow_dispatch", "build", "linux-amd64", []),
                         dict(unit=True, packaging=True, auth=False, windows=False))
        self.assertTrue(select_checks("workflow_dispatch", "build", "windows-arm64", [])["windows"])
        self.assertTrue(all(select_checks("workflow_dispatch", "full", "all", []).values()))

    def test_email_selects_only_registration_job(self):
        self.assertEqual(select_checks("workflow_dispatch", "email", "all", []),
                         dict(unit=False, packaging=False, auth=True, windows=False))

    def test_invalid_manual_input_fails_closed(self):
        with self.assertRaises(ValueError):
            select_checks("workflow_dispatch", "typo", "all", [])


if __name__ == "__main__":
    unittest.main()
