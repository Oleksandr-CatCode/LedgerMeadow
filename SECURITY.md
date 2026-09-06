# Reporting security issues

Use [GitHub private vulnerability reporting](https://github.com/Oleksandr-CatCode/LedgerMeadow/security/advisories/new). Do not include credentials, financial records, private endpoint addresses, or real account identifiers in public issues, discussions, or pull requests.

Provide a minimal reproduction with synthetic data. Describe the affected route, expected access boundary, and observed behavior without revealing another person's data. Do not test against a deployment you do not control.

Read [security status](docs/SECURITY_STATUS.md) before evaluating production use. Development controls and CI do not constitute a complete security assessment.

If credentials are actually published, revoke or rotate them and review all affected histories, logs, artifacts, and service access. Removing the current file does not remove historical exposure.
