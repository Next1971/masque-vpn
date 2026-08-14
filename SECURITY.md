# Security policy

## Project status

MASQUE VPN is experimental software. It has not been independently security-audited. Do not treat it as a guarantee of anonymity, censorship resistance, or protection against every network adversary.

## Reporting a vulnerability

Please do not open a public GitHub issue for a suspected vulnerability, private key exposure, authentication bypass, traffic leak, or other security-sensitive report.

Use GitHub's private vulnerability reporting feature for this repository when available. If it is unavailable, contact the repository owner privately through the GitHub profile and include:

- A concise description of the issue
- Affected component and version/commit
- Reproduction steps or proof of concept
- Expected and observed behaviour
- Security impact and suggested mitigation, if known

Do not include live credentials, private keys, client profiles, or personal data.

## Security practices

- Keep all private keys, certificates, tokens, and production profiles out of Git.
- Verify release checksums when they are published.
- Rotate credentials immediately if they are exposed.
- Apply operating-system and dependency updates promptly.
- Test changes on a non-production server before deployment.
