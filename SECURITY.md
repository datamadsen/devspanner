# Security Policy

## Supported Versions

devspanner is pre-1.0; security fixes land on the latest release. Please make sure
you're on the most recent version before reporting.

## Reporting a Vulnerability

**Please do not open a public issue for security problems.**

Report privately using GitHub's [private vulnerability reporting][gh] (the
"Report a vulnerability" button under the repository's **Security** tab), or email
**tim@datamadsen.dk**.

Please include:

- a description of the issue and its impact,
- steps to reproduce (a minimal `.devspanner/config.yaml` if relevant),
- the version (`devspanner -v`) and OS.

You can expect an initial response within a few days. Once a fix is ready we'll
coordinate a release and credit you in the changelog unless you'd prefer otherwise.

## Scope notes

devspanner runs the commands defined in a repo's `.devspanner/config.yaml` — by
design, it executes whatever `start`, `stop`, `run`, and `logs` commands the config
author specifies. Treat a config file as you would any executable script: only run
devspanner in repositories you trust.

[gh]: https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability
