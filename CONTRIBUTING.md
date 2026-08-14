# Contributing to MASQUE VPN

Thank you for helping test or improve the project. Small reproducible bug reports, documentation corrections, and cross-platform test results are valuable contributions.

## Before opening an issue

- Check existing issues first.
- Use the relevant issue template.
- Remove secrets, private keys, full client profiles, public IP addresses, and personally identifying information.
- Include the client platform, app/build version, server commit or release, and steps to reproduce.

## Development areas

- `server/` — server deployment assets, scripts, and systemd integration
- `windows/` — Windows client and its build/configuration files
- `android/` — Android application and Go integration source

Read the README inside the component you plan to build or modify.

## Pull requests

1. Create a focused branch from `main`.
2. Keep each pull request limited to one purpose.
3. Explain why the change is needed and how you tested it.
4. Do not commit generated credentials, certificates, binaries, local configuration, or other secrets.
5. Update documentation when behaviour, setup, or configuration changes.

For networking changes, describe the test environment without disclosing sensitive endpoints. State which client and server versions were tested and whether traffic, reconnects, and failure cases were checked.

## Code and documentation

- Prefer clear names and small, reviewable changes.
- Keep configuration examples safe to publish.
- Explain non-obvious protocol, routing, and certificate decisions.
- Use English for public documentation and issue reports where possible so more contributors can help.
