# MASQUE VPN MVP

> **Experimental cross-platform VPN based on MASQUE:** QUIC + HTTP/3 CONNECT-IP with mutual TLS.
>
> Status: **v1.0 released**. The project is suitable for technical testing, but it has not received an independent security audit.

MASQUE VPN MVP includes a Linux server, a Windows client, and an Android application. It is intended for people who want to deploy and test a self-managed MASQUE-based VPN.

## Quick start

| Goal | Start here |
|---|---|
| Download the latest build | [Latest release](../../releases/latest) |
| Deploy a Linux server | [Server deployment guide](server/README.md) |
| Connect from Android | [Android guide](android/README-Android.md) |
| Connect from Windows | [Windows guide](windows/README.md) |
| Report a bug or request a feature | [Open an issue](../../issues/new/choose) |
| Report a security issue privately | [Security policy](SECURITY.md) |

## What it provides

- QUIC transport with HTTP/3 CONNECT-IP
- Mutual TLS authentication between clients and server
- A self-hosted server deployment path
- Windows client source and configuration example
- Android application source

## Architecture

```text
Android client / Windows client
             |
             | QUIC + HTTP/3 CONNECT-IP + mTLS
             v
       MASQUE VPN server
             |
             v
          Internet
```

## Repository layout

| Directory | Contents |
|---|---|
| [`server/`](server/) | Deployment assets, scripts, and systemd integration |
| [`windows/`](windows/) | Windows client, configuration example, and build files |
| [`android/`](android/) | Android application, Go sources, and Android build files |

## Security and limitations

- This is **experimental software** and has not been independently security-audited.
- Operate your own server and protect certificates, private keys, tokens, and client profiles.
- Do not commit real credentials, endpoint addresses, or production configuration to this repository.
- A VPN does not provide universal anonymity or protect against every threat model.

Read [SECURITY.md](SECURITY.md) before deploying the project for real users.

## Documentation

- [Server deployment overview](server/README.md)
- [Android setup and build guide](android/README-Android.md)
- [Windows setup and build guide](windows/README.md)
- [Contributing guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## Contributing

Testing feedback is welcome, especially for Android, Windows, Linux server deployment, reconnection behaviour, and network compatibility. Please use the provided issue templates and remove all secrets from reports.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## License

A license file has not yet been added. Please contact the repository owner before reusing the code outside evaluation and contribution to this project.
